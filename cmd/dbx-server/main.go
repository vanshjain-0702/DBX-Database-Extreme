package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/server"
)

var version = "1.0.0"

func main() {
	cfgPath := flag.String("config", "configs/local.yaml", "config file path")
	printVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Printf("DBX version %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	inst, err := server.NewInstance(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create server instance: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := inst.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start server instance: %v\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		fmt.Printf("Received signal %v, shutting down...\n", sig)
	case err := <-inst.ErrorChannel():
		if err != nil {
			fmt.Printf("Server error: %v\n", err)
		}
	}

	cancel()
	inst.Stop()
}
