//go:build waterfallbench

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	benchmarkSentinel = "__WATERFALL_BENCHMARK__"
	defaultListen     = "127.0.0.1:8001"
	serveUsage        = "usage: waterfallbench serve [--listen address]"
)

type commandOptions struct {
	serve  bool
	listen string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Restore the default signal behavior so a second interrupt can force an
		// exit if graceful shutdown is waiting on an in-flight request.
		stop()
	}()
	err := run(ctx, os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "waterfallbench: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	options, err := parseCommand(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = fmt.Fprintln(stdout, serveUsage)
		return err
	}
	if err != nil {
		return err
	}
	if !options.serve {
		_, err := fmt.Fprintln(stdout, benchmarkSentinel)
		return err
	}
	return serve(ctx, options.listen, stdout)
}

func parseCommand(args []string) (commandOptions, error) {
	if len(args) == 0 {
		return commandOptions{}, nil
	}
	if args[0] != "serve" {
		return commandOptions{}, fmt.Errorf("unknown command %q; expected \"serve\"", args[0])
	}

	options := commandOptions{serve: true, listen: defaultListen}
	flags := flag.NewFlagSet("waterfallbench serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.listen, "listen", defaultListen, "HTTP listen address")
	if err := flags.Parse(args[1:]); err != nil {
		return commandOptions{}, fmt.Errorf("serve: %w", err)
	}
	if flags.NArg() != 0 {
		return commandOptions{}, fmt.Errorf("serve: unexpected arguments %q", flags.Args())
	}
	return options, nil
}
