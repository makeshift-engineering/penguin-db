// Package main is the entrypoint for starting the Penguin-DB storage node daemon.
// It handles CLI parameter parsing, configuration initialization, LSM engine
// bootstrap, and gRPC network server execution.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/makeshift-engineering/penguin-db/internal/config"
	"github.com/makeshift-engineering/penguin-db/internal/rpc/storage_server"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

type flags struct {
	configPath string
	port       int
	dataDir    string
}

// parseFlags parses the command-line parameters.
func parseFlags() flags {
	var f flags
	flag.StringVar(&f.configPath, "config", "", "Path to configuration JSON file")
	flag.IntVar(&f.port, "port", 0, "Port to listen on for gRPC requests (0 = use configuration value)")
	flag.StringVar(&f.dataDir, "dir", "", "Directory path for LSM storage database files (empty = use configuration value)")
	flag.Parse()
	return f
}

// loadAndMergeConfig retrieves settings from config file and merges CLI flag overrides.
func loadAndMergeConfig(f flags) (*config.Config, error) {
	var cfg *config.Config
	var err error
	if f.configPath != "" {
		cfg, err = config.LoadConfig(f.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file at %s: %w", f.configPath, err)
		}
		slog.Info("Loaded configuration from file", "path", f.configPath)
	} else {
		cfg = config.DefaultConfig()
		slog.Info("Using default configuration settings")
	}

	if f.port != 0 {
		cfg.Server.Port = f.port
	}
	if f.dataDir != "" {
		cfg.Server.Dir = f.dataDir
	}
	return cfg, nil
}

// initStorageEngine boots the directories and opens the LSM storage engine.
func initStorageEngine(cfg *config.Config) (storage.Engine, error) {
	if err := os.MkdirAll(cfg.Server.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	opts := cfg.Storage.ToStorageOptions()
	engine, err := storage.NewEngine(cfg.Server.Dir, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage engine: %w", err)
	}
	return engine, nil
}

// startRPCServer handles network binding, server initialization, graceful stop orchestration, and cleanup.
func startRPCServer(cfg *config.Config, engine storage.Engine) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		_ = engine.Close()
		return fmt.Errorf("failed to listen on port %d: %w", cfg.Server.Port, err)
	}

	grpcServer := grpc.NewServer()
	storageServer := storage_server.NewStorageServer(engine)
	storageServer.Register(grpcServer)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("Shutdown signal received, shutting down gracefully...", "signal", sig)
		// We call Stop() first to interrupt any active stream scans instantly.
		storageServer.Stop()

		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
			slog.Info("gRPC server stopped gracefully")
		case <-time.After(5 * time.Second):
			slog.Warn("Graceful shutdown timed out, stopping server forcefully")
			grpcServer.Stop()
		}
	}()

	slog.Info("gRPC Storage Server is listening", "address", lis.Addr().String())
	var serveErr error
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("gRPC server run failed", "error", err)
		serveErr = err
	}

	// Clean up snapshots and close the storage engine
	storageServer.Stop()
	if err := engine.Close(); err != nil {
		slog.Error("Error closing storage engine", "error", err)
		if serveErr == nil {
			serveErr = err
		}
	} else {
		slog.Info("Storage engine closed successfully")
	}

	return serveErr
}

// main is the single-point entry function.
func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	f := parseFlags()
	cfg, err := loadAndMergeConfig(f)
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	engine, err := initStorageEngine(cfg)
	if err != nil {
		slog.Error("Storage initialization error", "error", err)
		os.Exit(1)
	}

	if err := startRPCServer(cfg, engine); err != nil {
		os.Exit(1)
	}
}
