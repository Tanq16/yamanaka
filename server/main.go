package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"yamanaka/api"
	"time" // Added for ticker
	"yamanaka/state"
	"yamanaka/vault"
)

const (
	dataDir              = "./data"
	serverAddr           = ":8080"
	gitCommitInterval    = 6 * time.Hour
	periodicCommitUserID = "server_periodic_commit"
)

// startPeriodicGitCommits starts a goroutine that periodically commits changes in the vault.
func startPeriodicGitCommits(vaultPath string, sm *state.Manager) {
	log.Printf("Starting periodic Git committer. Interval: %v", gitCommitInterval)
	ticker := time.NewTicker(gitCommitInterval)
	go func() {
		for range ticker.C {
			log.Println("Periodic Git committer: Attempting to commit changes...")
			commitMsg := "Automatic periodic server commit"
			newHash, err := vault.CommitChanges(vaultPath, commitMsg)
			if err != nil {
				log.Printf("ERROR: Periodic Git committer: Failed to commit changes: %v", err)
				continue // Continue to next tick
			}

			// Check if the hash actually changed (i.e., if there were any new changes)
			// We need a way to get the hash *before* this commit attempt to compare.
			// For simplicity, CommitChanges now returns the current hash even if nothing was committed.
			// So, we broadcast only if the process of committing (even if it results in the same hash) is considered an event.
			// Or, more robustly, CommitChanges could return a boolean indicating if a *new* commit was actually made.
			// For now, we'll assume any call to CommitChanges that doesn't error should result in a broadcast,
			// as it represents a "settling" of the state.

			// It's important that CommitChanges itself is wrapped in the FileSystemMutex.
			// And GetCurrentHash is also wrapped if it reads from disk in a way that could race.
			// GetCurrentHash currently uses git rev-parse HEAD, which should be safe.

			log.Printf("Periodic Git committer: Changes committed. New hash: %s", newHash)
			// sm.Broadcast(periodicCommitUserID, newHash) // DO NOT broadcast git hash to clients anymore. This is purely a backend operation.
			// If we need to notify admins or a different system, use a separate mechanism.
		}
	}()
}

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

	// --- 3a. Start Periodic Git Commits ---
	startPeriodicGitCommits(vaultPath, stateManager)

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
