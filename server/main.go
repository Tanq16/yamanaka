package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"time"

	"github.com/tanq16/yamanaka/server/api"
	"github.com/tanq16/yamanaka/server/state"
	"github.com/tanq16/yamanaka/server/vault"
)

const (
	dataDir              = "./data"
	serverAddr           = ":8080"
	gitCommitInterval    = 6 * time.Hour
	periodicCommitUserID = "server_periodic_commit"
)

// goroutine to periodically commit changes in the vault
func startPeriodicGitCommits(vaultPath string) {
	slog.Info("git-goroutine: started", "interval", gitCommitInterval)
	ticker := time.NewTicker(gitCommitInterval)
	go func() {
		for range ticker.C {
			commitMsg := "Yamanaka git sync"
			newHash, err := vault.CommitChanges(vaultPath, commitMsg)
			if err != nil {
				slog.Error("git-goroutine: failed to commit changes", "error", err)
				continue
			}
			slog.Info("git-goroutine: changes committed", "hash", newHash)
		}
	}()
}

func main() {
	vaultPath, _ := filepath.Abs(dataDir)
	if _, err := os.Stat(vaultPath); os.IsNotExist(err) {
		slog.Info("data directory not found, creating", "vault path", vaultPath)
		if err := os.MkdirAll(vaultPath, 0755); err != nil {
			slog.Error("could not create data directory", "error", err)
			os.Exit(1)
		}
	}
	if err := vault.InitRepo(vaultPath); err != nil {
		slog.Error("could not initialize git", "error", err)
	}
	slog.Info("vault ready")

	stateManager := state.NewManager(vaultPath)
	slog.Info("state manager initialized")
	apiHandler := api.NewApiHandler(stateManager, vaultPath)
	startPeriodicGitCommits(vaultPath)

	// http routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", apiHandler.CheckHandler)
	mux.HandleFunc("/api/sync/initial", apiHandler.InitialSyncHandler)
	mux.HandleFunc("/api/sync/push", apiHandler.PushHandler)
	mux.HandleFunc("/api/sync/pull", apiHandler.PullHandler)
	mux.HandleFunc("/api/events", apiHandler.EventsHandler)
	// simple root handler for health checks
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Yamanaka Sync Server is running."))
	})

	slog.Info("starting server", "address", serverAddr)
	corsMux := corsMiddleware(mux)
	if err := http.ListenAndServe(serverAddr, corsMux); err != nil {
		slog.Error("could not start server", "error", err)
		os.Exit(1)
	}
}

// wraps an http.Handler with CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "app://obsidian.md")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Requested-With, Origin, Accept, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
