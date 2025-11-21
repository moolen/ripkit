package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/moritz/mcp-toolkit/internal/watch/api"
	"github.com/moritz/mcp-toolkit/internal/watch/config"
	"github.com/moritz/mcp-toolkit/internal/watch/storage"
	"github.com/moritz/mcp-toolkit/internal/watch/watchers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	// Setup logger
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	log := ctrl.Log.WithName("watch-server")

	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config/resources.yaml"
	}

	cfg, err := loadConfig(configPath, log)
	if err != nil {
		log.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	log.Info("Configuration loaded",
		"storagePath", cfg.StoragePath,
		"retentionDays", cfg.RetentionDays,
		"serverPort", cfg.ServerPort,
		"maxQueryLimit", cfg.MaxQueryLimit,
		"resourceCount", len(cfg.Resources),
		"discoverCRDs", cfg.DiscoverCRDs)

	// Initialize BadgerDB storage
	store, err := storage.NewStore(cfg.StoragePath, cfg.RetentionDays)
	if err != nil {
		log.Error(err, "Failed to initialize storage")
		os.Exit(1)
	}
	defer store.Close()
	log.Info("Storage initialized", "path", cfg.StoragePath)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start garbage collection routine
	go store.StartGCRoutine(ctx)
	log.Info("Started background GC routine")

	// Create controller-runtime manager
	kubeConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Cache: cache.Options{
			// Watch all namespaces
			DefaultNamespaces: map[string]cache.Config{},
		},
		// Disable metrics server
	})
	if err != nil {
		log.Error(err, "Failed to create controller manager")
		os.Exit(1)
	}
	log.Info("Controller-runtime manager created")

	// Initialize watcher manager
	watcherMgr := watchers.NewManager(mgr, store, cfg)
	if err := watcherMgr.Start(ctx); err != nil {
		log.Error(err, "Failed to start watchers")
		os.Exit(1)
	}
	log.Info("Watchers initialized")

	// Start the controller-runtime manager
	go func() {
		log.Info("Starting controller-runtime manager")
		if err := mgr.Start(ctx); err != nil {
			log.Error(err, "Manager stopped with error")
			os.Exit(1)
		}
	}()

	// Wait for cache to sync
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		log.Error(fmt.Errorf("cache sync failed"), "Failed to sync cache")
		os.Exit(1)
	}
	log.Info("Cache synced successfully")

	// Create and start HTTP server
	apiServer := api.NewServer(store, cfg.MaxQueryLimit)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      apiServer,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Info("Starting HTTP server", "port", cfg.ServerPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "HTTP server error")
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "HTTP server shutdown error")
	}

	// Cancel context to stop watchers and GC
	cancel()

	log.Info("Shutdown complete")
}

// loadConfig loads configuration from file or returns default
func loadConfig(path string, log logr.Logger) (*config.Config, error) {
	// Try to load from file
	if _, err := os.Stat(path); err == nil {
		log.Info("Loading configuration from file", "path", path)
		return config.LoadConfig(path)
	}

	// Use default configuration
	log.Info("Using default configuration")
	cfg := config.DefaultConfig()

	// Override with environment variables if set
	if storagePath := os.Getenv("BADGER_PATH"); storagePath != "" {
		cfg.StoragePath = storagePath
	}
	if serverPort := os.Getenv("SERVER_PORT"); serverPort != "" {
		var port int
		if _, err := fmt.Sscanf(serverPort, "%d", &port); err == nil {
			cfg.ServerPort = port
		}
	}

	return cfg, nil
}
