package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const FixedConfigPath = "configs/config.json"

// LoadFixedConfig reads JSON configuration from configs/config.json if available.
func LoadFixedConfig() (string, int) {
	defaultHost := "127.0.0.1"
	defaultPort := 5433

	file, err := os.Open(FixedConfigPath)
	if err != nil {
		return defaultHost, defaultPort
	}
	defer file.Close()

	var doc struct {
		Gateway *struct {
			PGWirePort  int    `json:"pgwire_port"`
			StorageAddr string `json:"storage_addr"`
		} `json:"gateway"`
	}
	if err := json.NewDecoder(file).Decode(&doc); err != nil {
		return defaultHost, defaultPort
	}

	if doc.Gateway != nil && doc.Gateway.PGWirePort != 0 {
		defaultPort = doc.Gateway.PGWirePort
	}

	return defaultHost, defaultPort
}

func main() {
	cfgHost, cfgPort := LoadFixedConfig()

	hostFlag := flag.String("host", cfgHost, "PenguinDB Gateway host address")
	hFlag := flag.String("h", "", "PenguinDB Gateway host address (short)")
	portFlag := flag.Int("port", cfgPort, "PenguinDB Gateway pgwire TCP port")
	pFlag := flag.Int("p", 0, "PenguinDB Gateway pgwire TCP port (short)")
	userFlag := flag.String("user", "penguin", "Database user name")
	uFlag := flag.String("u", "", "Database user name (short)")
	dbFlag := flag.String("db", "testdb", "Target database name")
	dFlag := flag.String("d", "", "Target database name (short)")
	flag.Parse()

	host := *hostFlag
	if *hFlag != "" {
		host = *hFlag
	}

	port := *portFlag
	if *pFlag != 0 {
		port = *pFlag
	}

	user := *userFlag
	if *uFlag != "" {
		user = *uFlag
	}

	database := *dbFlag
	if *dFlag != "" {
		database = *dFlag
	}

	// Connect custom PGWire client over TCP (Trust authentication, no password required)
	client := NewPGClient(host, port, user, database)
	if err := client.Connect(); err != nil {
		fmt.Printf("\x1b[31mError connecting to PenguinDB Gateway at %s:%d\x1b[0m\n", host, port)
		fmt.Printf("Details: %v\n\n", err)
		fmt.Println("Please ensure the PenguinDB SQL Gateway daemon is running:")
		fmt.Println("  go run ./cmd/gateway/main.go")
		os.Exit(1)
	}

	// Initialize Bubbletea TUI Program in standard inline terminal mode
	m := NewModel(client, host, port, database)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		log.Fatalf("CLI TUI Error: %v", err)
	}
}
