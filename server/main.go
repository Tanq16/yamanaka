package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"yamanaka/api"
	"yamanaka/state"
	"yamanaka/vault"
)

const (
	dataDir    = "./data"
	serverAddr = ":8080"
)

func main() {
	log.Println("Starting Yamanaka Sync Server...")

	// --- 1. Setup Vault Directory ---
	// The vault directory is where all the notes and the git repository are stored.
	vaultPath, err := filepath.Abs(dataDir)
	if err != nil {
		log.Fatalf("FATAL: Could not determine absolute path for data directory: %v", err)
	}

	// Create the data directory if it doesn't exist.
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		log.Printf("Data directory not found. Creating at %s", vaultPath)
		if err := os.MkdirAll(vaultPath, 0755); err != nil {
			log.Fatalf("FATAL: Could not create data directory: %v", err)
		}
	}

	// Initialize the git repository if it's not already there.
	if err := vault.InitRepo(vaultPath); err != nil {
		log.Fatalf("FATAL: Could not initialize git repository: %v", err)
	}
	log.Printf("Vault is ready at %s", vaultPath)

	// --- 2. Initialize State Manager ---
	// The state manager keeps track of all connected clients for real-time updates (SSE).
	stateManager := state.NewManager()
	log.Println("State manager initialized.")

	// --- 3. Setup API Handlers ---
	// We create an ApiHandler struct to provide dependencies (like the state manager and vault path)
	// to our HTTP handler functions. This is a form of dependency injection.
	apiHandler := api.NewApiHandler(stateManager, vaultPath)

	// --- 4. Define HTTP Routes ---
	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", apiHandler.CheckHandler)
	mux.HandleFunc("/api/sync/initial", apiHandler.InitialSyncHandler)
	mux.HandleFunc("/api/sync/push", apiHandler.PushHandler)
	mux.HandleFunc("/api/sync/pull", apiHandler.PullHandler)
	mux.HandleFunc("/api/events", apiHandler.EventsHandler)

	// Simple root handler for health checks
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Yamanaka Sync Server is running."))
	})

	// --- 5. Start Server ---
	log.Printf("Server starting on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("FATAL: Could not start server: %v", err)
	}
}
