package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dataDir := flag.String("data-dir", "", "legacy tenant data directory to archive")
	confirm := flag.Bool("confirm-reset", false, "confirm creation of a fresh v1 data directory")
	flag.Parse()
	if *dataDir == "" || !*confirm {
		fmt.Fprintln(os.Stderr, "usage: dbx-v1-reset -data-dir <tenant-dir> -confirm-reset")
		os.Exit(2)
	}
	absolute, err := filepath.Abs(*dataDir)
	if err != nil {
		fail(err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		fail(fmt.Errorf("data directory is unavailable: %w", err))
	}
	if !info.IsDir() {
		fail(fmt.Errorf("data path is not a directory"))
	}
	archive := absolute + ".pre-v1-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(absolute, archive); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		_ = os.Rename(archive, absolute)
		fail(err)
	}
	fmt.Printf("Archived legacy data at %s\nFresh v1 directory created at %s\n", archive, absolute)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dbx-v1-reset:", err)
	os.Exit(1)
}
