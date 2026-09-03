package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/dbx/dbx/internal/config"
	"github.com/dbx/dbx/internal/isolation"
	"github.com/dbx/dbx/internal/server"
)

var version = "1.0.0"

func main() {
	cfgPath := flag.String("config", "configs/local.yaml", "config file path")
	printVersion := flag.Bool("version", false, "print version and exit")
	isolate := flag.Bool("isolate", false, "apply Isolation Kernel (Landlock) after bind")
	dekStdin := flag.Bool("dek-stdin", false, "read the 32-byte tenant DEK from stdin")
	tenantID := flag.String("tenant-id", "", "tenant id for control endpoints")
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
	if pid := os.Getenv("DBX_ORCHESTRATOR_PID"); pid != "" {
		if n, err := strconv.Atoi(pid); err == nil && n > 0 {
			cfg.Server.PeerPIDs = []int{n}
		}
	}

	inst, err := server.NewInstance(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create server instance: %v\n", err)
		os.Exit(1)
	}
	inst.SkipBuiltinUser()
	if *tenantID != "" {
		inst.SetTenantID(*tenantID)
	}
	if *dekStdin {
		dek := make([]byte, 32)
		if _, err := io.ReadFull(os.Stdin, dek); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read tenant DEK: %v\n", err)
			os.Exit(1)
		}
		enc, err := isolation.NewEncryptor(dek)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid tenant DEK: %v\n", err)
			os.Exit(1)
		}
		inst.SetAtRest(enc)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := inst.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start server instance: %v\n", err)
		os.Exit(1)
	}
	if *isolate {
		if err := isolation.LockDown(cfg.Persistence.DataDir); err != nil {
			fmt.Fprintf(os.Stderr, "isolation lockdown failed: %v\n", err)
			inst.Stop()
			os.Exit(1)
		}
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
