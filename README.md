# Penguin DB

Penguin DB is a fast, transactional LSM-tree-backed key-value store database node. It exposes its operations over gRPC and provides support for prefix-range scans, write batching, and point-in-time read snapshots.

---

## 1. gRPC Code Generation

The gRPC schemas are defined using Protocol Buffers version 3. We use [Buf](https://buf.build/) to manage, lint, and compile our `.proto` schemas.

### Prerequisites
Make sure you have `buf` installed and available in your shell environment:
```bash
# If installed via Go:
go install github.com/bufbuild/buf/cmd/buf@latest
```

Ensure the plugins `protoc-gen-go` and `protoc-gen-go-grpc` are also installed and present on your environment PATH:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Steps to Auto-Generate Stubs
Navigate to the `proto/` directory and execute `buf generate`:
```bash
cd proto
buf generate
```
This reads `buf.yaml` and `buf.gen.yaml` inside the `proto/` folder, compiling the schemas and writing the resulting Go files to `gen/go/storage/v1/`.

---

## 2. Running the Storage Server

The database server daemon is located under `cmd/storage-node/main.go`.

### Run with Default Settings
By default, the server starts on port `50051` and writes its database engine files to `./data`:
```bash
go run cmd/storage-node/main.go
```

### Run with a Configuration File
You can supply a JSON configuration file using the `-config` flag:
```bash
go run cmd/storage-node/main.go -config configs/config.json
```

A sample `configs/config.json` looks like this:
```json
{
  "server": {
    "port": 50051,
    "dir": "./data"
  },
  "storage": {
    "memtable": {
      "max_size_bytes": 4194304,
      "max_level": 12,
      "max_imm": 2
    },
    "wal": {
      "segment_size_bytes": 33554432,
      "batch_size_bytes": 4194304,
      "ingest_channel_capacity": 10000
    },
    "flush": {
      "estimated_keys": 10000
    },
    "compaction": {
      "threshold": 4,
      "read_buffer_size": 1048576,
      "estimated_keys": 100000,
      "max_sstable_size": 2097152
    }
  }
}
```

### Run with Flag Overrides
You can override specific parameters via CLI flags at startup. Overrides take precedence over values loaded from the configuration file:
```bash
go run cmd/storage-node/main.go -port 8080 -dir ./db-data
```

### Supported CLI Flags
- `-config`: string - Path to the JSON configuration file (e.g., `configs/config.json`).
- `-port`: int - TCP port to bind the gRPC server onto (defaults to `50051`).
- `-dir`: string - Directory path to locate database storage files (defaults to `./data`).

---

## 3. Graceful Shutdown
To stop the server without risking data corruption, send a `SIGINT` (Ctrl+C) or `SIGTERM` signal. The storage node daemon will intercept it and perform a graceful sequence:
1. Stop accepting new incoming client requests.
2. Complete in-progress read-amplifications and prefix-scans.
3. Dispose of all active snapshots.
4. Flush the current active Memtable skip list to a Level 0 SSTable.
5. Gracefully close open segment log writers and shutdown.
