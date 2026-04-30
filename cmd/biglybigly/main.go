package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hamed0406/biglybigly/internal/core/api"
	"github.com/hamed0406/biglybigly/internal/core/config"
	"github.com/hamed0406/biglybigly/internal/core/storage"
	"github.com/hamed0406/biglybigly/internal/platform"
	"github.com/hamed0406/biglybigly/internal/tools/netmon"
	"github.com/hamed0406/biglybigly/internal/tools/urlcheck"
)

// Set via ldflags at build time
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Parse flags
	mode := flag.String("mode", "server", "server or agent")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("biglybigly %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	// Override with env var
	if envMode := os.Getenv("BIGLYBIGLY_MODE"); envMode != "" {
		*mode = envMode
	}

	// Load config
	cfg := config.Load()

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Open database
	db, err := storage.OpenDB(cfg)
	if err != nil {
		logger.Error("Failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Run platform migrations
	if err := storage.RunMigrations(db); err != nil {
		logger.Error("Failed to run platform migrations", "err", err)
		os.Exit(1)
	}

	// Register modules
	modules := []platform.Module{
		urlcheck.New(),
		netmon.New(),
		// Add more modules here
	}

	// Create registry
	registry := platform.NewRegistry(db, logger)
	for _, mod := range modules {
		if err := registry.Register(mod); err != nil {
			logger.Error("Failed to register module", "module", mod.ID(), "err", err)
			os.Exit(1)
		}
	}

	// Create platform
	mux := http.NewServeMux()
	plat := platform.NewPlatform(db, mux, logger)

	// Create cancellable context for module lifecycle
	moduleCtx, moduleCancel := context.WithCancel(context.Background())
	defer moduleCancel()

	// Start modules
	if err := registry.Start(moduleCtx, plat); err != nil {
		logger.Error("Failed to start modules", "err", err)
		os.Exit(1)
	}

	// Create HTTP server
	apiHandler := api.NewServer(plat, registry)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiHandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	logger.Info("Starting Biglybigly", "addr", cfg.HTTPAddr, "mode", *mode)

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "err", err)
		}
	}()

	// Wait for interrupt signal
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
	<-sigch

	logger.Info("Shutting down...")
	moduleCancel()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("Shutdown error", "err", err)
	}
}
