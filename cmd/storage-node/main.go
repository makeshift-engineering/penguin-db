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

	"google.golang.org/grpc"

	"github.com/makeshift-engineering/penguin-db/internal/config"
	"github.com/makeshift-engineering/penguin-db/internal/rpc/storage_server"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

// main bootstraps and runs the storage node daemon.
func main() {
	configPath := flag.String("config", "", "Path to configuration JSON file")
	port := flag.Int("port", 0, "Port to listen on for gRPC requests (0 = use configuration value)")
	dataDir := flag.String("dir", "", "Directory path for LSM storage database files (empty = use configuration value)")
	flag.Parse()

	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load configuration hierarchy
	var cfg *config.Config
	var err error
	if *configPath != "" {
		cfg, err = config.LoadConfig(*configPath)
		if err != nil {
			slog.Error("Failed to load config file", "path", *configPath, "error", err)
			os.Exit(1)
		}
		slog.Info("Loaded configuration from file", "path", *configPath)
	} else {
		cfg = config.DefaultConfig()
		slog.Info("Using default configuration settings")
	}

	// CLI Flag overrides
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *dataDir != "" {
		cfg.Server.Dir = *dataDir
	}

	slog.Info("Starting Penguin-DB Storage Node...", "port", cfg.Server.Port, "dir", cfg.Server.Dir)

	// Create storage directory
	if err := os.MkdirAll(cfg.Server.Dir, 0o755); err != nil {
		slog.Error("Failed to create storage directory", "dir", cfg.Server.Dir, "error", err)
		os.Exit(1)
	}

	// Map config options to storage options
	opts := cfg.Storage.ToStorageOptions()

	// Open the engine
	engine, err := storage.NewEngine(cfg.Server.Dir, opts)
	if err != nil {
		slog.Error("Failed to initialize storage engine", "error", err)
		os.Exit(1)
	}

	// Setup listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		slog.Error("Failed to listen on port", "port", cfg.Server.Port, "error", err)
		_ = engine.Close()
		os.Exit(1)
	}

	// Initialize server
	grpcServer := grpc.NewServer()
	storageServer := storage_server.NewStorageServer(engine)
	storageServer.Register(grpcServer)

	// Intercept shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		slog.Info("Shutdown signal received, shutting down gracefully...", "signal", sig)
		grpcServer.GracefulStop()
	}()

	slog.Info("gRPC Storage Server is listening", "address", lis.Addr().String())
	var serveErr error
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("gRPC server run failed", "error", err)
		serveErr = err
	}

	storageServer.ReleaseAllSnapshots()
	if err := engine.Close(); err != nil {
		slog.Error("Error closing storage engine", "error", err)
		os.Exit(1)
	}
	slog.Info("Storage engine closed successfully")

	if serveErr != nil {
		os.Exit(1)
	}
}
