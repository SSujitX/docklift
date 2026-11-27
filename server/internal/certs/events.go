package certs

import (
	"sort"
	"sync"
	"time"
)

// EventLevel is the severity of an SSL activity line.
type EventLevel string

const (
	LevelInfo    EventLevel = "info"
	LevelSuccess EventLevel = "success"
	LevelWarn    EventLevel = "warn"
	LevelError   EventLevel = "error"
)

// Event is one SSL activity line; JSON {at, level, message} as in certs.ts.
type Event struct {
	At      string     `json:"at"`
	Level   EventLevel `json:"level"`
	Message string     `json:"message"`
}

// MaxEventsPerDomain bounds the activity kept per hostname and returned per
// query.
const MaxEventsPerDomain = 40

// EventLog is the recent issuance activity per hostname, so the UI can show
// what certbot is doing. It is in memory on purpose: it is progress
// narration, not state (the PEMs remain the source of truth). Safe for
// concurrent use.
type EventLog struct {
	mu     sync.Mutex
	byHost map[string][]Event
	now    func() time.Time
}

// NewEventLog returns an empty log.
func NewEventLog() *EventLog {
	return &EventLog{byHost: map[string][]Event{}, now: time.Now}
}

// Append records one event (stamped now) for every non-empty hostname.
func (l *EventLog) Append(hostnames []string, level EventLevel, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ev := Event{At: isoMillis(l.now()), Level: level, Message: message}
	for _, h := range hostnames {
		h = NormalizeHost(h)
		if h == "" {
			continue
		}
		events := append(l.byHost[h], ev)
		if len(events) > MaxEventsPerDomain {
			events = append([]Event(nil), events[len(events)-MaxEventsPerDomain:]...)
		}
		l.byHost[h] = events
	}
}

// Get returns the activity of the hostnames merged, de-duplicated by
// (at, message), oldest first, at most MaxEventsPerDomain. Never nil.
func (l *EventLog) Get(hostnames []string) []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[string]bool{}
	merged := []Event{}
	for _, h := range hostnames {
		for _, ev := range l.byHost[NormalizeHost(h)] {
			key := ev.At + "|" + ev.Message
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, ev)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].At < merged[j].At })
	if len(merged) > MaxEventsPerDomain {
		merged = merged[len(merged)-MaxEventsPerDomain:]
	}
	return merged
}

// Clear drops the activity of the hostnames.
func (l *EventLog) Clear(hostnames []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, h := range hostnames {
		delete(l.byHost, NormalizeHost(h))
	}
}
