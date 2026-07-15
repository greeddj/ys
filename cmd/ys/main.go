// Package cmd wires CLI configuration and subcommands.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/greeddj/ys/cmd/ys/helpers"

	"github.com/urfave/cli/v3"
)

//nolint:gochecknoglobals
var (
	Version string
	Commit  string
	Date    string
	BuiltBy string
)

// main is the entry point for the ys application.
func main() {
	os.Exit(run())
}

// run configures and executes the ys CLI application.
func run() int {
	// Customize the version printer to show only the version.
	cli.VersionPrinter = func(c *cli.Command) {
		_, _ = fmt.Fprintln(c.Writer, Version)
	}

	app := &cli.Command{
		Name:                   "ys",
		Usage:                  "Simple YAML search across files",
		HideHelpCommand:        true,
		UseShortOptionHandling: true,
		Version:                helpers.Version(Version, Commit, Date, BuiltBy),
		Commands:               []*cli.Command{
			// place holder
		},
	}

	// user invokes Ctrl+\ when the program appears hung; the dump shows which
	// goroutine is blocked and where.
	sigquit := make(chan os.Signal, 1)
	signal.Notify(sigquit, syscall.SIGQUIT)
	go func() {
		for range sigquit {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			os.Stderr.Write(buf[:n]) //nolint:errcheck
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args); err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return 130
		default:
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}
	return 0
}
