package httpx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBodyBudgetAcquireRelease(t *testing.T) {
	b := NewBodyBudget(100)
	if !b.Acquire(t.Context(), 60, 0) || !b.Acquire(t.Context(), 40, 0) {
		t.Fatal("bytes within capacity must be reserved")
	}
	if b.Acquire(t.Context(), 1, 10*time.Millisecond) {
		t.Fatal("a full budget must refuse")
	}
	done := make(chan bool)
	go func() { done <- b.Acquire(t.Context(), 50, 2*time.Second) }()
	time.Sleep(20 * time.Millisecond)
	b.Release(60)
	if !<-done {
		t.Fatal("a waiter must get bytes released while it waits")
	}
	// A request larger than the whole budget fits an idle budget alone.
	b.Release(40)
	b.Release(50)
	if !b.Acquire(t.Context(), 1000, 0) || b.InUse() != 100 {
		t.Fatalf("oversized reservation: in use %d", b.InUse())
	}
	b.Release(1000)
	if b.InUse() != 0 {
		t.Fatalf("in use after release = %d", b.InUse())
	}
}

// TestParseBodyBudgetSaturated holds the whole budget with one slow handler
// and checks that a second body is refused with 503 + Retry-After instead
// of being buffered.
func TestParseBodyBudgetSaturated(t *testing.T) {
	old := bodyBudgetWait
	bodyBudgetWait = 50 * time.Millisecond
	t.Cleanup(func() { bodyBudgetWait = old })
	budget := NewBodyBudget(1024)
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once, unblockOnce sync.Once
	release := func() { unblockOnce.Do(func() { close(unblock) }) }
	srv := httptest.NewServer(ParseBodyWith(BodyOptions{Limit: 1024, Budget: budget})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-unblock
		JSON(w, http.StatusOK, map[string]any{"ok": true})
	})))
	t.Cleanup(srv.Close)
	t.Cleanup(release)

	full := `{"a":"` + strings.Repeat("x", 1000) + `"}`
	first := make(chan int)
	go func() {
		resp, err := http.Post(srv.URL, "application/json", strings.NewReader(full))
		if err != nil {
			first <- 0
			return
		}
		resp.Body.Close()
		first <- resp.StatusCode
	}()
	<-entered
	if budget.InUse() != int64(len(full)) {
		t.Fatalf("reserved %d, want the declared length %d", budget.InUse(), len(full))
	}
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"b":"`+strings.Repeat("y", 100)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 503 || resp.Header.Get("Retry-After") != "1" || !strings.Contains(string(body), MsgServerBusy) {
		t.Fatalf("saturated budget: %d Retry-After=%q %s", resp.StatusCode, resp.Header.Get("Retry-After"), body)
	}
	release()
	if code := <-first; code != 200 {
		t.Fatalf("first request: %d", code)
	}
	if budget.InUse() != 0 {
		t.Fatalf("budget not returned after the handler: %d", budget.InUse())
	}
}

func TestParseBodyWithLimit(t *testing.T) {
	budget := NewBodyBudget(MaxBodyBytes)
	srv := httptest.NewServer(ParseBodyWith(BodyOptions{Limit: 64, Budget: budget})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]any{"defined": BodyFrom(r).Defined})
	})))
	t.Cleanup(srv.Close)
	if e := post(t, srv, "application/json", strings.NewReader(`{"a":1}`), nil); e.status != 200 || !e.Defined {
		t.Errorf("small body: %+v", e)
	}
	big := `{"a":"` + strings.Repeat("x", 100) + `"}`
	if e := post(t, srv, "application/json", strings.NewReader(big), nil); e.status != 413 || e.Error != MsgBodyTooLarge {
		t.Errorf("declared length over the limit: %+v", e)
	}
	chunked := func(r *http.Request) { r.ContentLength = -1 }
	if e := post(t, srv, "application/json", io.MultiReader(strings.NewReader(big)), chunked); e.status != 413 || e.Error != MsgBodyTooLarge {
		t.Errorf("chunked body over the limit: %+v", e)
	}
	if budget.InUse() != 0 {
		t.Errorf("budget leaked: %d", budget.InUse())
	}
}

func TestParseBodyValueCap(t *testing.T) {
	srv := bodyServer(t)
	many := "[" + strings.Repeat("0,", MaxJSONValues) + "0]"
	if e := post(t, srv, "application/json", strings.NewReader(many), nil); e.status != 413 || e.Error != MsgBodyTooLarge {
		t.Errorf("%d values: %d %q", MaxJSONValues+1, e.status, e.Error)
	}
	// Separators inside strings are not values.
	str := `{"a":"` + strings.Repeat(",:[{", MaxJSONValues) + `"}`
	if e := post(t, srv, "application/json", strings.NewReader(str), nil); e.status != 200 {
		t.Errorf("punctuation inside a string: %d %q", e.status, e.Error)
	}
	if n := countJSONValues([]byte(`{"a":[1,2,{"b":"x\",y"}]}`)); n < 6 || n > 8 {
		t.Errorf("countJSONValues = %d", n)
	}
}

func TestReadJSONBodyThenDecode(t *testing.T) {
	budget := NewBodyBudget(1 << 20)
	o := BodyOptions{Limit: 1 << 10, Budget: budget}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, release, ok := ReadJSONBody(w, r, o)
		if !ok {
			return
		}
		defer release()
		if raw == nil {
			JSON(w, http.StatusOK, map[string]any{"defined": false})
			return
		}
		b, err := DecodeJSONBody(raw)
		if err != nil {
			WriteBodyError(w, r, err)
			return
		}
		JSON(w, http.StatusOK, map[string]any{"defined": true, "raw": string(raw), "value": b.Value().Raw()})
	}))
	t.Cleanup(srv.Close)
	if e := post(t, srv, "application/json", strings.NewReader(`{"b":2,  "a":1}`), nil); e.status != 200 || e.Raw != `{"b":2,  "a":1}` {
		t.Errorf("exact bytes: %+v", e)
	}
	if e := post(t, srv, "application/json", strings.NewReader(`{"a":`), nil); e.status != 400 || e.Error != MsgInvalidJSON {
		t.Errorf("malformed: %+v", e)
	}
	if e := post(t, srv, "text/plain", strings.NewReader(`{}`), nil); e.status != 200 || e.Defined {
		t.Errorf("other content type is not read: %+v", e)
	}
	if e := post(t, srv, "application/json", bytes.NewReader(make([]byte, 2<<10)), nil); e.status != 413 {
		t.Errorf("over the limit: %+v", e)
	}
	if budget.InUse() != 0 {
		t.Errorf("budget leaked: %d", budget.InUse())
	}
}
