// Command biglybigly is the single-binary entry point for the platform. It
// runs in one of two modes selected via -mode or the BIGLYBIGLY_MODE env var:
//
//   - server: serves the web UI, owns the database, and accepts WebSocket
//     connections from remote agents. This is the default.
//   - agent:  no UI; collects local data (network flows, system metrics, DNS
//     queries) and ships it to a server over the agent protocol. Also runs a
//     local DNS filtering proxy on 127.0.0.1:53.
//
// Modules are registered explicitly here — adding a new tool is a one-line
// change to the modules slice below. Logs are written to both stderr and a
// file via io.MultiWriter so operators can tail the file in production while
// developers still see output on the terminal.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
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
	"github.com/hamed0406/biglybigly/internal/tools/dnsfilter"
	"github.com/hamed0406/biglybigly/internal/tools/netmon"
	"github.com/hamed0406/biglybigly/internal/tools/sysmon"
	"github.com/hamed0406/biglybigly/internal/tools/urlcheck"
)

// Build metadata, populated via -ldflags "-X main.version=... -X main.commit=...".
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

	// Setup file logging — write to data dir if DB path is set, else current dir
	logPath := "biglybigly.log"
	if dbPath := os.Getenv("BIGLYBIGLY_DB_PATH"); dbPath != "" {
		// Use same directory as the database
		dir := dbPath[:max(0, len(dbPath)-len("biglybigly.db"))]
		if dir != "" {
			logPath = dir + "biglybigly.log"
		}
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: could not open log file: %v\n", err)
		logFile = nil
	}
	if logFile != nil {
		defer logFile.Close()
	}

	// Multi-writer: log to both stderr and file
	var logWriter io.Writer
	if logFile != nil {
		logWriter = io.MultiWriter(os.Stderr, logFile)
	} else {
		logWriter = os.Stderr
	}

	// Setup logger
	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	logger.Info("Log file opened", "path", "biglybigly.log")

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
		sysmon.New(),
		dnsfilter.New(),
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

// runAgent is the agent-mode entry point. It runs preflight checks, opens a
// connection to the configured server, then launches a fan of background
// goroutines: a DNS filtering proxy, periodic blocklist/rule sync, and
// shippers for network flows, system metrics, and DNS query logs.
//
// Two ordering details matter here:
//   - VPN/proxy detection runs at startup so operators are warned early when
//     a VPN is intercepting DNS and would defeat local filtering.
//   - The DNS proxy is started BEFORE blocklists are downloaded. If a previous
//     run already pointed system DNS at 127.0.0.1, the blocklist HTTP fetch
//     itself depends on the proxy being live to resolve its own URL.
//
// Server-pushed rule changes are reconciled every 5 minutes; full blocklist
// re-downloads happen every 6 hours.
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

	// Check for VPN/proxy that may interfere with DNS filtering
	agent.DetectVPN(logger)

	// Start collector
	collector := netmon.NewCollectorWithLogger(logger)
	logger.Info("Agent started — collecting network flows every 30s")

	// Start sysmon collector
	sysCollector := sysmon.NewCollector(logger)
	logger.Info("Agent started — collecting system metrics every 30s")

	// Start DNS filter proxy
	dnsBlocklist := dnsfilter.NewBlocklistManager(logger)

	// Seed default blocklist in agent DB if none exist
	var blCount int
	db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_blocklists`).Scan(&blCount)
	if blCount == 0 {
		now := time.Now().Unix()
		db.Exec(`INSERT OR IGNORE INTO dnsfilter_blocklists (url, name, enabled, created_at) VALUES (?, ?, 1, ?)`,
			"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts", "Steven Black Unified", now)
		logger.Info("DNS Filter: seeded default blocklist on agent")
	}

	// Start proxy BEFORE loading blocklists — the blocklist download needs DNS,
	// and if system DNS is already set to 127.0.0.1 from a previous run, the
	// proxy must be running to resolve the download URL via upstream.
	dnsProxy := dnsfilter.NewProxy("127.0.0.1:53", []string{"8.8.8.8:53", "1.1.1.1:53"}, dnsBlocklist, logger)

	go func() {
		logger.Info("Starting DNS proxy on 127.0.0.1:53 (requires admin/root)")
		if err := dnsProxy.Start(ctx); err != nil {
			logger.Error("DNS proxy failed to start", "err", err,
				"hint", "On Windows: run as Administrator and stop 'DNS Client' service (net stop dnscache)")
		}
	}()

	// Auto-configure system DNS to use our proxy
	dnsConfig := agent.NewDNSConfigurator(logger)
	// Give proxy a moment to bind
	time.Sleep(500 * time.Millisecond)
	if dnsConfig.Configure() {
		defer dnsConfig.Restore()
	}

	// Now load blocklists — proxy is running so DNS works for the download
	go func() {
		// Sync rules from server first (updates local DB)
		dnsBlocklist.SyncRulesFromServer(client, db, logger)

		// Then load everything from DB into memory (blocklists + rules)
		if err := dnsBlocklist.LoadFromDB(db); err != nil {
			logger.Warn("Failed to load DNS blocklists", "err", err)
		}
	}()

	// Periodically refresh blocklists (every 6 hours) and sync rules (every 5 min)
	go func() {
		ruleTicker := time.NewTicker(5 * time.Minute)
		listTicker := time.NewTicker(6 * time.Hour)
		defer ruleTicker.Stop()
		defer listTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ruleTicker.C:
				// Sync rules from server (updates local DB), then rebuild memory
				dnsBlocklist.SyncRulesFromServer(client, db, logger)
				if err := dnsBlocklist.LoadFromDB(db); err != nil {
					logger.Warn("Blocklist reload failed", "err", err)
				}
			case <-listTicker.C:
				// Full blocklist re-download
				if err := dnsBlocklist.LoadFromDB(db); err != nil {
					logger.Warn("Blocklist refresh failed", "err", err)
				}
			}
		}
	}()

	// Ship DNS query logs to server every 30s
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logs := dnsProxy.FlushLogs()
				if len(logs) == 0 {
					continue
				}
				logger.Info("Sending DNS query logs", "count", len(logs))
				if err := client.SendDNSLogs(ctx, logs); err != nil {
					logger.Warn("Failed to send DNS logs", "err", err)
				}
			}
		}
	}()

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

	// Sysmon collection goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap, err := sysCollector.Collect()
				if err != nil {
					logger.Warn("Sysmon collect failed", "err", err)
					continue
				}
				if err := client.SendSysmon(ctx, snap); err != nil {
					logger.Warn("Failed to send sysmon data", "err", err)
				} else {
					logger.Debug("Sent sysmon snapshot", "cpu", snap.CPUPercent, "mem_used", snap.MemUsed)
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
