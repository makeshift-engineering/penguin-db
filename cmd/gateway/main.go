package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/executor"
	"github.com/makeshift-engineering/penguin-db/internal/wire"
)

// FixedConfigPath defines the mandatory fixed path location for reading JSON configuration.
const FixedConfigPath = "configs/config.json"

// GatewayConfig holds configuration settings for the SQL Wire Protocol Gateway daemon.
type GatewayConfig struct {
	PGWirePort  int    `json:"pgwire_port"`
	StorageAddr string `json:"storage_addr"`
}

// DefaultGatewayConfig provides default fallback values when configuration file is absent.
func DefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		PGWirePort:  5433,
		StorageAddr: "127.0.0.1:50051",
	}
}

// LoadFixedConfig reads JSON configuration from configs/config.json if present.
func LoadFixedConfig() *GatewayConfig {
	cfg := DefaultGatewayConfig()
	file, err := os.Open(FixedConfigPath)
	if err != nil {
		return cfg
	}
	defer file.Close()

	var doc struct {
		Gateway *GatewayConfig `json:"gateway"`
		Server  *struct {
			Port int `json:"port"`
		} `json:"server"`
	}
	if err := json.NewDecoder(file).Decode(&doc); err != nil {
		log.Printf("Warning: Error parsing %s (%v), using default settings", FixedConfigPath, err)
		return cfg
	}

	if doc.Gateway != nil {
		if doc.Gateway.PGWirePort != 0 {
			cfg.PGWirePort = doc.Gateway.PGWirePort
		}
		if doc.Gateway.StorageAddr != "" {
			cfg.StorageAddr = doc.Gateway.StorageAddr
		}
	} else if doc.Server != nil && doc.Server.Port != 0 {
		cfg.StorageAddr = fmt.Sprintf("127.0.0.1:%d", doc.Server.Port)
	}

	return cfg
}

func main() {
	portFlag := flag.Int("port", 0, "PenguinDB pgwire TCP server port (overrides config.json)")
	pFlag := flag.Int("p", 0, "PenguinDB pgwire TCP server port short (overrides config.json)")
	storageAddrFlag := flag.String("storage-addr", "", "Remote storage node gRPC address (overrides config.json)")
	flag.Parse()

	cfg := LoadFixedConfig()

	if *portFlag != 0 {
		cfg.PGWirePort = *portFlag
	} else if *pFlag != 0 {
		cfg.PGWirePort = *pFlag
	}

	if *storageAddrFlag != "" {
		cfg.StorageAddr = *storageAddrFlag
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Connecting Gateway to storage gRPC server at %s...", cfg.StorageAddr)
	conn, err := grpc.NewClient(cfg.StorageAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to storage node at %s: %v", cfg.StorageAddr, err)
	}
	defer conn.Close()

	grpcClient := storagepb.NewStorageServiceClient(conn)
	store := kv.NewRemoteKV(grpcClient)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down PenguinDB SQL Gateway...")
		cancel()
	}()

	cat, err := catalog.NewCatalog(ctx, store)
	if err != nil {
		cat = catalog.NewEmptyCatalog()
	}
	exec := executor.New(store, cat)
	addr := fmt.Sprintf(":%d", cfg.PGWirePort)
	server := wire.NewServer(addr, exec, cat)

	log.Printf("PenguinDB SQL Gateway Server listening on pgwire port %d (connected to storage node at %s)...", cfg.PGWirePort, cfg.StorageAddr)
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Gateway server error: %v", err)
	}
}
