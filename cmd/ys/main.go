// Package main configures and runs the ys CLI.
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
	"github.com/greeddj/ys/internal/app"

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

	cmd := newCommand()

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

	if err := cmd.Run(ctx, os.Args); err != nil {
		var exitErr cli.ExitCoder
		switch {
		case errors.Is(err, context.Canceled):
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return 130
		case errors.As(err, &exitErr):
			if msg := exitErr.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, msg)
			}
			return exitErr.ExitCode()
		default:
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}
	return 0
}

// newCommand builds the ys CLI command with its flags and action.
func newCommand() *cli.Command {
	return &cli.Command{
		Name:      "ys",
		Usage:     "Simple YAML search across files",
		UsageText: "ys [-r|-p] KEY PATH...",
		Description: "Search the YAML files under each PATH for KEY and group identical\n" +
			"results, printing each unique \"path: value\" block once with the list\n" +
			"of files it was found in.\n\n" +
			"By default KEY is matched exactly against the last path segment.\n\n" +
			"In regexp mode, sibling paths of equal depth that differ in a single\n" +
			"segment and carry an identical value are merged into one block whose\n" +
			"path lists the alternatives, e.g. services.(api|worker).resources.limits.",
		HideHelpCommand:        true,
		UseShortOptionHandling: true,
		Version:                helpers.Version(Version, Commit, Date, BuiltBy),
		// main owns the exit codes: suppress the framework's default os.Exit so
		// every error flows back through run() below.
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "path",
				Aliases: []string{"p"},
				Usage:   "match KEY as a path suffix, e.g. http.timeout",
			},
			&cli.BoolFlag{
				Name:    "regexp",
				Aliases: []string{"r"},
				Usage:   "match KEY as a regexp over the full dotted path",
			},
			&cli.BoolFlag{
				Name:    "color",
				Aliases: []string{"c"},
				Usage:   "force colorized output (default: only on a terminal)",
			},
			&cli.BoolFlag{
				Name:    "no-color",
				Aliases: []string{"n"},
				Usage:   "disable colorized output",
			},
		},
		Action: action,
	}
}

// action parses the positional KEY and PATH arguments and runs the search.
func action(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() < 2 {
		_ = cli.ShowRootCommandHelp(cmd)
		return cli.Exit("", 2)
	}

	opts := app.Options{
		Key:       args.First(),
		Roots:     args.Tail(),
		RegexMode: cmd.Bool("regexp"),
		PathMode:  cmd.Bool("path"),
		Color:     helpers.WantColor(cmd.Bool("color"), cmd.Bool("no-color")),
	}

	if err := app.Run(ctx, cmd.Writer, cmd.ErrWriter, opts); err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return cli.Exit(fmt.Sprintf("ys: %v", err), 2)
	}
	return nil
}
