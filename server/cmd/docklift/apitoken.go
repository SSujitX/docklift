package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"docklift/internal/api/tokens"
	"docklift/internal/auth"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/store"
)

// apiToken is `docklift api-token create|list|revoke`: manage the bearer
// tokens scripts and AI agents use (the same rows as Settings → API & MCP).
// It runs on the host, so it needs no password: whoever can run it already
// has the database.
func apiToken(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		apiTokenUsage()
		return exitCode(2)
	}
	switch args[0] {
	case "create", "list", "revoke":
	default:
		apiTokenUsage()
		return exitCode(2)
	}
	cfg, err := loadConfig(log)
	if err != nil {
		return err
	}
	return runAPIToken(ctx, cfg, args)
}

func apiTokenUsage() {
	fmt.Print(`usage: docklift api-token create <name> [--read] [--allow-dangerous] [--expires 90d]
       docklift api-token list
       docklift api-token revoke <id|prefix>

  create   make a token for the first admin and print it once.
           Scope is full unless --read (GET requests only). --allow-dangerous lets it
           run actions that ask for the password in the dashboard (rollback, restore,
           reboot, purge, shell commands). --expires takes days (90d) or never (default).
  list     show every token (never the token itself)
  revoke   delete a token by id or by the start of its prefix (dl_...)
`)
}

// apiTokenFlags is the parsed `create` command line.
type apiTokenFlags struct {
	in tokens.CreateInput
}

func parseAPITokenCreate(args []string) (apiTokenFlags, error) {
	f := apiTokenFlags{in: tokens.CreateInput{Scope: auth.ScopeFull}}
	var names []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--read":
			f.in.Scope = auth.ScopeRead
		case a == "--allow-dangerous":
			f.in.AllowDangerous = true
		case a == "--expires" || strings.HasPrefix(a, "--expires="):
			v, ok := strings.CutPrefix(a, "--expires=")
			if !ok {
				if i+1 >= len(args) {
					return f, errors.New("--expires needs a value such as 90d")
				}
				i++
				v = args[i]
			}
			days, err := parseDays(v)
			if err != nil {
				return f, err
			}
			f.in.ExpiresInDays = days
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("unknown flag %s", a)
		default:
			names = append(names, a)
		}
	}
	f.in.Name = strings.TrimSpace(strings.Join(names, " "))
	if f.in.Name == "" || len([]rune(f.in.Name)) > tokens.MaxNameLength {
		return f, errors.New("a token name is required (max 100 characters)")
	}
	if f.in.AllowDangerous && f.in.Scope != auth.ScopeFull {
		return f, errors.New("--allow-dangerous cannot be combined with --read")
	}
	return f, nil
}

// parseDays reads "90d", "90" or "never".
func parseDays(v string) (int, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "never" || v == "0" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
	if err != nil || n < 1 || n > tokens.MaxExpiresInDays {
		return 0, fmt.Errorf("--expires must be a number of days from 1 to %d (e.g. 90d) or never", tokens.MaxExpiresInDays)
	}
	return n, nil
}

func runAPIToken(ctx context.Context, cfg *config.Config, args []string) (err error) {
	var create apiTokenFlags
	switch args[0] {
	case "create":
		if create, err = parseAPITokenCreate(args[1:]); err != nil {
			fmt.Printf("❌ %v\n\n", err)
			apiTokenUsage()
			return exitCode(2)
		}
	case "list":
		if len(args) != 1 {
			apiTokenUsage()
			return exitCode(2)
		}
	case "revoke":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			apiTokenUsage()
			return exitCode(2)
		}
	}
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	st := store.New(database)
	fail := func(what string, err error) error {
		if errors.Is(err, store.ErrMissingTable) {
			fmt.Print("❌ The database has no api_tokens table yet. Start the server once (or run docklift migrate) first.\n")
		} else {
			fmt.Printf("❌ Could not %s: %v\n", what, err)
		}
		return exitCode(1)
	}

	switch args[0] {
	case "create":
		owner, err := st.FirstUserByRole(ctx, "admin")
		if errors.Is(err, store.ErrNotFound) {
			fmt.Print("❌ No admin user found. Finish the setup in the dashboard first.\n")
			return exitCode(1)
		}
		if err != nil {
			return fail("read the admin user", err)
		}
		row, plain, err := tokens.Issue(ctx, st, owner.ID, create.in, time.Now())
		if err != nil {
			return fail("create the token", err)
		}
		fmt.Printf("✅ API token %q created for %s\n\n", row.Name, owner.Email)
		fmt.Printf("   %s\n\n", plain)
		fmt.Printf("   ID:      %s\n   Scope:   %s\n   Danger:  %s\n   Expires: %s\n\n", row.ID, row.Scope, yesNo(row.AllowDangerous), when(row.ExpiresAt, "never"))
		fmt.Print("⚠️  This is the only time the token is shown. Use it as: Authorization: Bearer <token>\n")
		fmt.Print("   Changing the account password revokes it.\n")
		return nil
	case "list":
		rows, err := st.ListAPITokens(ctx)
		if err != nil {
			return fail("list tokens", err)
		}
		if len(rows) == 0 {
			fmt.Print("No API tokens. Create one with: docklift api-token create <name>\n")
			return nil
		}
		owners := map[string]*store.User{}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tPREFIX\tSCOPE\tDANGEROUS\tLAST USED\tEXPIRES\tSTATUS")
		now := time.Now()
		for _, t := range rows {
			owner, seen := owners[t.UserID]
			if !seen {
				owner, _ = st.GetUser(ctx, t.UserID)
				owners[t.UserID] = owner
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Prefix, t.Scope, yesNo(t.AllowDangerous),
				when(t.LastUsedAt, "never"), when(t.ExpiresAt, "never"), tokens.ItemOf(t, owner, now).Status)
		}
		return tw.Flush()
	default: // revoke
		ref := strings.TrimSpace(args[1])
		target, err := st.GetAPIToken(ctx, ref)
		if errors.Is(err, store.ErrNotFound) {
			matches, ferr := st.FindAPITokensByPrefix(ctx, ref)
			if ferr != nil {
				return fail("find the token", ferr)
			}
			switch {
			case len(matches) == 0 || len(ref) < 4:
				fmt.Printf("❌ No API token with id or prefix %q.\n", ref)
				return exitCode(1)
			case len(matches) > 1:
				fmt.Printf("❌ %d tokens start with %q. Use the id (docklift api-token list).\n", len(matches), ref)
				return exitCode(1)
			}
			target, err = matches[0], nil
		}
		if err != nil {
			return fail("find the token", err)
		}
		if err := st.DeleteAPIToken(ctx, target.ID); err != nil {
			return fail("revoke the token", err)
		}
		fmt.Printf("✅ API token %q (%s) revoked. It stops working at once.\n", target.Name, target.Prefix)
		return nil
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func when(t *db.Time, none string) string {
	if t == nil {
		return none
	}
	return t.Time().Local().Format("2006-01-02 15:04")
}
