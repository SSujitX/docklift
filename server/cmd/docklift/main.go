// Command docklift is the Docklift backend: the HTTP API, the WebSocket
// terminal and the embedded dashboard.
//
// Usage:
//
//	docklift [serve]        run the server (default)
//	docklift migrate        apply database migrations and exit
//	docklift reset-password generate a new password for the first admin
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "docklift %s takes no arguments\n", cmd)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "serve":
		err = serve(ctx, log)
	case "migrate":
		err = migrate(ctx, log)
	case "reset-password":
		err = resetPassword(ctx, log)
	case "help", "-h", "--help":
		usage()
		return 0
	default:
		usage()
		return 2
	}
	var exit exitCode
	if errors.As(err, &exit) {
		return int(exit)
	}
	if err != nil {
		log.Error("docklift "+cmd+" failed", "error", err)
		return 1
	}
	return 0
}

// exitCode ends the process with a status after the command already reported
// why on stdout.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func usage() {
	fmt.Fprint(os.Stderr, `usage: docklift [command]

commands:
  serve           run the API server (default)
  migrate         apply database migrations and exit
  reset-password  set a random password for the first admin user and print it
`)
}
