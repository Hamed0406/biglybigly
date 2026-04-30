package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hamed0406/biglybigly/internal/core/agent"
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

	// Check if setup is complete; if so, use DB mode (env var overrides)
	setupComplete := storage.IsSetupComplete(db)
	if setupComplete && *mode == "server" {
		if dbMode, _ := storage.GetSetting(db, "mode"); dbMode != "" {
			*mode = dbMode
		}
	}

	// Generate bootstrap token for first-run setup security
	bootstrapToken := ""
	if !setupComplete {
		bootstrapToken = api.GenerateBootstrapToken()
		logger.Info("══════════════════════════════════════════════════")
		logger.Info("  FIRST RUN — Setup required")
		logger.Info("  Open your browser and complete setup at:")
		logger.Info(fmt.Sprintf("  http://localhost%s", cfg.HTTPAddr))
		logger.Info("")
		logger.Info(fmt.Sprintf("  Bootstrap token: %s", bootstrapToken))
		logger.Info("  (required to complete setup)")
		logger.Info("══════════════════════════════════════════════════")
	}

	// Register modules
	modules := []platform.Module{
		urlcheck.New(),
		netmon.New(),
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

	// Branch based on mode
	if *mode == "agent" {
		runAgent(moduleCtx, moduleCancel, cfg, db, logger)
		return
	}

	// --- Server Mode ---

	// Only start modules if setup is complete
	if setupComplete {
		if err := registry.Start(moduleCtx, plat); err != nil {
			logger.Error("Failed to start modules", "err", err)
			os.Exit(1)
		}
	}

	// Create HTTP server
	apiHandler := api.NewServer(plat, registry, bootstrapToken)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiHandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	logger.Info("Starting Biglybigly", "addr", cfg.HTTPAddr, "mode", *mode, "setup_complete", setupComplete)

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "err", err)
		}
	}()

	// Log registered module routes
	for _, mod := range modules {
		logger.Info("Module loaded", "id", mod.ID(), "version", mod.Version(), "routes_active", setupComplete)
	}

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

// runAgent runs the agent mode — collects network data and sends to server
func runAgent(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, db *sql.DB, logger *slog.Logger) {
	// Run preflight checks
	if !agent.RunPreflight(logger) {
		logger.Error("Required preflight checks failed — cannot start agent")
		os.Exit(1)
	}

	serverURL, _ := storage.GetSetting(db, "server_url")
	agentName, _ := storage.GetSetting(db, "instance_name")

	// Env vars override DB settings
	if env := os.Getenv("BIGLYBIGLY_SERVER_URL"); env != "" {
		serverURL = env
	}
	if env := os.Getenv("BIGLYBIGLY_AGENT_NAME"); env != "" {
		agentName = env
	}
	agentToken := os.Getenv("BIGLYBIGLY_AGENT_TOKEN")

	if serverURL == "" {
		logger.Error("Agent mode requires BIGLYBIGLY_SERVER_URL or setup via web UI")
		os.Exit(1)
	}
	if agentName == "" {
		hostname, _ := os.Hostname()
		agentName = hostname
	}

	client := agent.NewClient(serverURL, agentName, agentToken, logger)

	// Test connectivity
	logger.Info("Agent mode — connecting to server", "server", serverURL, "agent_name", agentName)
	if err := client.Ping(ctx); err != nil {
		logger.Error("Cannot reach server", "err", err)
		logger.Info("Will retry in the background...")
	} else {
		logger.Info("Server connection OK")
	}

	// Start collector
	collector := netmon.NewCollectorWithLogger(logger)
	logger.Info("Agent started — collecting network flows every 30s")

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Exponential backoff for retries
		backoff := 1 * time.Second
		maxBackoff := 60 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				flows := collector.CollectFiltered()
				if len(flows) == 0 {
					continue
				}
				logger.Info("Collected flows", "count", len(flows))

				if err := client.SendFlows(ctx, flows); err != nil {
					logger.Warn("Failed to send flows", "err", err, "retry_in", backoff)
					backoff = min(backoff*2, maxBackoff)
				} else {
					backoff = 1 * time.Second
				}
			}
		}
	}()

	// Periodic status display
	go func() {
		statusTicker := time.NewTicker(60 * time.Second)
		defer statusTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-statusTicker.C:
				status := client.Status()
				totalSent, totalErrors, flowsSent, lastSend, lastErr := client.Stats()

				statusIcon := "●"
				switch status {
				case agent.StatusConnected:
					statusIcon = "🟢"
				case agent.StatusDisconnected:
					statusIcon = "🔴"
				case agent.StatusConnecting:
					statusIcon = "🟡"
				}

				sinceLastSend := "never"
				if !lastSend.IsZero() {
					sinceLastSend = time.Since(lastSend).Round(time.Second).String() + " ago"
				}

				logger.Info("──── Agent Status ────────────────────────")
				logger.Info(fmt.Sprintf("  %s Connection: %s", statusIcon, status))
				logger.Info(fmt.Sprintf("  Server:     %s", serverURL))
				logger.Info(fmt.Sprintf("  Batches:    %d sent, %d errors", totalSent, totalErrors))
				logger.Info(fmt.Sprintf("  Flows:      %d total sent", flowsSent))
				logger.Info(fmt.Sprintf("  Last send:  %s", sinceLastSend))
				if lastErr != "" {
					logger.Info(fmt.Sprintf("  Last error: %s", lastErr))
				}
				logger.Info("──────────────────────────────────────────")
			}
		}
	}()

	// Wait for interrupt
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt, syscall.SIGTERM)
	<-sigch

	logger.Info("Agent shutting down...")
	cancel()
}
