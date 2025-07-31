package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nullable-eth/syncarr/internal/config"
	"github.com/nullable-eth/syncarr/internal/logger"
	"github.com/nullable-eth/syncarr/internal/orchestrator"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Command line flags
	var (
		showVersion   = flag.Bool("version", false, "Show version information")
		validateOnly  = flag.Bool("validate", false, "Validate configuration and exit")
		oneShot       = flag.Bool("oneshot", false, "Run sync once and exit (don't run continuously)")
		forceFullSync = flag.Bool("force-full-sync", false, "Force a complete synchronization, bypassing incremental checks")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("SyncArr %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override force full sync if specified via command line
	if *forceFullSync {
		cfg.ForceFullSync = true
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	if *validateOnly {
		fmt.Println("Configuration is valid")
		os.Exit(0)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)

	log.WithFields(map[string]interface{}{
		"version":          version,
		"commit":           commit,
		"build_date":       date,
		"source_host":      cfg.Source.Host,
		"destination_host": cfg.Destination.Host,
		"sync_label":       cfg.SyncLabel,
		"force_full_sync":  cfg.ForceFullSync,
		"dry_run":          cfg.DryRun,
	}).Info("SyncArr starting up")

	// Create sync orchestrator
	sync, err := orchestrator.NewSyncOrchestrator(cfg, log)
	if err != nil {
		log.WithError(err).Fatal("Failed to create sync orchestrator")
	}
	defer func() {
		if err := sync.Close(); err != nil {
			log.WithError(err).Error("Failed to close sync orchestrator")
		}
	}()

	// Handle force full sync
	if err := sync.HandleForceFullSync(); err != nil {
		log.WithError(err).Fatal("Failed to handle force full sync")
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run sync
	if *oneShot {
		log.Info("Running single synchronization cycle")
		if err := sync.RunSyncCycle(); err != nil {
			log.WithError(err).Fatal("Sync failed")
		}
		log.Info("Single sync completed successfully")
	} else {
		// Run continuous sync in a goroutine
		go func() {
			if err := sync.RunContinuous(); err != nil {
				log.WithError(err).Error("Continuous sync failed")
			}
		}()

		// Wait for shutdown signal
		sig := <-sigChan
		log.WithField("signal", sig.String()).Info("Received shutdown signal, stopping...")
	}

	log.Info("SyncArr shutdown complete")
}
