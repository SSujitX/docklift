package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"docklift/internal/certs"
	"docklift/internal/config"
	"docklift/internal/db"
	"docklift/internal/nginx"
	"docklift/internal/panelaccess"
	"docklift/internal/store"
)

const reopenCommand = "docker exec docklift-backend docklift panel-access open"

// panelAccess is `docklift panel-access status|open|close [domain]`: the
// server-side switch of "Only allow access through the domain", and the
// way back in when the panel domain no longer works. It writes the same
// settings row as the dashboard; a running server re-reads it every
// panelaccess.RefreshInterval.
func panelAccess(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 || len(args) > 2 || (len(args) == 2 && args[0] != "close") {
		panelAccessUsage()
		return exitCode(2)
	}
	switch args[0] {
	case "status", "open", "close":
	default:
		panelAccessUsage()
		return exitCode(2)
	}
	cfg, err := loadConfig(log)
	if err != nil {
		return err
	}
	return runPanelAccess(ctx, cfg, args)
}

func panelAccessUsage() {
	fmt.Print(`usage: docklift panel-access status|open|close [domain]

  status          show whether the dashboard answers only through its panel domain
  open            allow http://SERVER_IP:8080 again (turns the restriction off)
  close [domain]  answer only through the HTTPS panel domain (name it when there are several)
`)
}

func runPanelAccess(ctx context.Context, cfg *config.Config, args []string) (err error) {
	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	st := store.New(database)
	panels := nginx.New(nginx.Options{ConfDir: cfg.NginxConfPath, BackendPort: cfg.Port}, nil, st)
	g := panelaccess.New(st, panels, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := g.Load(ctx); err != nil {
		fmt.Printf("❌ Could not read the setting: %v\n", err)
		return exitCode(1)
	}
	apply := "A running server applies this within " + panelaccess.RefreshInterval.Round(time.Second).String() + "."

	// open is the way back in: it must not depend on the nginx conf directory.
	if args[0] == "open" {
		was, err := g.Disable(ctx)
		if err != nil {
			fmt.Printf("❌ Could not change the setting: %v\n", err)
			return exitCode(1)
		}
		if was == "" {
			fmt.Print("🔓 Dashboard access was already open. Nothing changed.\n")
			return nil
		}
		fmt.Printf("✅ Dashboard access by IP is allowed again (it was restricted to https://%s).\n", was)
		fmt.Printf("   %s Then http://SERVER_IP:8080 works again; no restart needed.\n", apply)
		return nil
	}

	usable, err := usablePanelDomains(panels)
	if err != nil {
		fmt.Printf("❌ Could not read the panel domains in %s: %v\n", cfg.NginxConfPath, err)
		return exitCode(1)
	}
	if args[0] == "status" {
		if domain := g.Domain(); domain != "" {
			fmt.Printf("🔒 Dashboard access: only through https://%s\n", domain)
			fmt.Print("   http://SERVER_IP:8080 sends browsers to the domain and refuses API and terminal requests.\n")
			fmt.Printf("   To allow it again: %s\n", reopenCommand)
			return nil
		}
		fmt.Print("🔓 Dashboard access: open. http://SERVER_IP:8080 and every panel domain work.\n")
		if len(usable) == 0 {
			fmt.Print("   No panel domain with HTTPS is configured (Settings → Domain), so it cannot be restricted yet.\n")
		} else {
			fmt.Printf("   Can be restricted to: %s (docklift panel-access close)\n", strings.Join(usable, ", "))
		}
		return nil
	}

	// close
	if len(cfg.TrustedProxies.Trusted) == 0 {
		fmt.Print("❌ TRUSTED_PROXIES trusts no proxy, so requests through the domain cannot be told apart from direct ones. Nothing changed.\n")
		return exitCode(1)
	}
	var domain string
	switch {
	case len(args) == 2:
		domain = certs.NormalizeHost(args[1])
		if !certs.IsValidHostname(domain) || !slices.Contains(usable, domain) {
			fmt.Printf("❌ %s is not a panel domain of the dashboard with HTTPS. Nothing changed.\n", args[1])
			printUsable(usable)
			return exitCode(1)
		}
	case len(usable) == 0:
		fmt.Print("❌ No panel domain with HTTPS is configured. Add one in Settings → Domain and wait until HTTPS is active. Nothing changed.\n")
		return exitCode(1)
	case len(usable) > 1:
		fmt.Print("❌ Several panel domains have HTTPS. Name the one to use: docklift panel-access close <domain>\n")
		printUsable(usable)
		return exitCode(1)
	default:
		domain = usable[0]
	}
	if err := g.Enable(ctx, domain); err != nil {
		fmt.Printf("❌ Could not change the setting: %v\n", err)
		return exitCode(1)
	}
	fmt.Printf("🔒 Dashboard access is now restricted to https://%s\n\n", domain)
	fmt.Printf("⚠️  %s From then on http://SERVER_IP:8080 (also from this server and through\n", apply)
	fmt.Print("   SSH tunnels) sends browsers to the domain and refuses API and terminal requests.\n")
	fmt.Printf("   To allow it again: %s\n", reopenCommand)
	return nil
}

// usablePanelDomains lists the panel domains access can be restricted to.
func usablePanelDomains(panels *nginx.Service) ([]string, error) {
	list, err := panels.ListPanelDomains()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range list {
		if !certs.IsValidHostname(p.Domain) {
			continue
		}
		st, err := panels.PanelState(p.Domain)
		if err != nil {
			return nil, err
		}
		if panelaccess.Usable(st) {
			out = append(out, p.Domain)
		}
	}
	return out, nil
}

func printUsable(usable []string) {
	if len(usable) == 0 {
		fmt.Print("   No panel domain with HTTPS is configured.\n")
		return
	}
	fmt.Printf("   Panel domains with HTTPS: %s\n", strings.Join(usable, ", "))
}
