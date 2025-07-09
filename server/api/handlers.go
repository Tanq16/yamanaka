package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"yamanaka/state"
	"yamanaka/vault"
)

// ApiHandler holds dependencies for our handlers.
type ApiHandler struct {
	StateManager *state.Manager
	VaultPath    string
}

// NewApiHandler creates a new ApiHandler with its dependencies.
func NewApiHandler(sm *state.Manager, vaultPath string) *ApiHandler {
	return &ApiHandler{
		StateManager: sm,
		VaultPath:    vaultPath,
	}
}

// --- Response Structs ---

type CheckResponse struct {
	Status     string `json:"status"`
	LatestHash string `json:"latest_hash,omitempty"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	NewHash string `json:"new_hash"`
}

type PullResponse struct {
	Hash  string      `json:"hash"`
	Files []vault.File `json:"files"`
}

type PushRequest struct {
	FilesToUpdate []vault.File `json:"files_to_update"`
	FilesToDelete []string     `json:"files_to_delete"`
}

// --- Handlers ---

// CheckHandler compares the client's hash with the server's latest git hash.
func (h *ApiHandler) CheckHandler(w http.ResponseWriter, r *http.Request) {
	clientHash := r.URL.Query().Get("current_hash")
	
	serverHash, err := vault.GetCurrentHash(h.VaultPath)
	if err != nil {
		http.Error(w, "Could not get server hash", http.StatusInternalServerError)
		return
	}

	if clientHash == serverHash {
		json.NewEncoder(w).Encode(CheckResponse{Status: "uptodate"})
	} else {
		json.NewEncoder(w).Encode(CheckResponse{Status: "new_changes", LatestHash: serverHash})
	}
}

// InitialSyncHandler handles the first-time sync from a client, replacing the server's vault.
func (h *ApiHandler) InitialSyncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deviceID := r.URL.Query().Get("device_id")

	// 1. Clean the vault (delete all files except .git)
	if err := vault.CleanDir(h.VaultPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to clean vault: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Extract the uploaded tar.gz archive
	if err := vault.ExtractTarGz(r.Body, h.VaultPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to extract archive: %v", err), http.StatusInternalServerError)
		return
	}

	// 3. Commit changes to git
	commitMsg := fmt.Sprintf("Initial sync from device %s", deviceID)
	newHash, err := vault.CommitChanges(h.VaultPath, commitMsg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to commit changes: %v", err), http.StatusInternalServerError)
		return
	}

	// 4. Notify other clients
	h.StateManager.Broadcast(deviceID, newHash)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{Status: "success", NewHash: newHash})
}

// PushHandler applies incremental changes from a client.
func (h *ApiHandler) PushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deviceID := r.URL.Query().Get("device_id")

	var req PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// 1. Process files to delete
	for _, path := range req.FilesToDelete {
		if err := vault.DeleteFile(h.VaultPath, path); err != nil {
			log.Printf("WARN: Could not delete file %s: %v", path, err)
		}
	}

	// 2. Process files to update/create
	for _, file := range req.FilesToUpdate {
		content, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			log.Printf("WARN: Could not decode file content for %s: %v", file.Path, err)
			continue
		}
		if err := vault.WriteFile(h.VaultPath, file.Path, content); err != nil {
			log.Printf("WARN: Could not write file %s: %v", file.Path, err)
		}
	}

	// 3. Commit changes to git
	commitMsg := fmt.Sprintf("Sync from device %s", deviceID)
	newHash, err := vault.CommitChanges(h.VaultPath, commitMsg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to commit changes: %v", err), http.StatusInternalServerError)
		return
	}

	// 4. Notify other clients
	h.StateManager.Broadcast(deviceID, newHash)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{Status: "success", NewHash: newHash})
}


// PullHandler sends the entire current state of the vault to the client.
func (h *ApiHandler) PullHandler(w http.ResponseWriter, r *http.Request) {
	currentHash, err := vault.GetCurrentHash(h.VaultPath)
	if err != nil {
		http.Error(w, "Could not get server hash", http.StatusInternalServerError)
		return
	}

	files, err := vault.GetAllFiles(h.VaultPath)
	if err != nil {
		http.Error(w, "Could not read vault files", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PullResponse{
		Hash:  currentHash,
		Files: files,
	})
}


// EventsHandler manages Server-Sent Events (SSE) for real-time updates.
func (h *ApiHandler) EventsHandler(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create a channel for this client
	messageChan := make(chan string)
	h.StateManager.AddClient(deviceID, messageChan)
	defer h.StateManager.RemoveClient(deviceID)

	log.Printf("Client %s connected for events", deviceID)

	// Listen for context cancellation (client disconnects)
	ctx := r.Context()

	for {
		select {
		case newHash := <-messageChan:
			// A new hash has been broadcasted, send it to the client
			eventData := fmt.Sprintf(`{"latest_hash": "%s"}`, newHash)
			fmt.Fprintf(w, "event: new_changes\ndata: %s\n\n", eventData)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-ctx.Done():
			// Client has disconnected
			log.Printf("Client %s disconnected", deviceID)
			return
		}
	}
}

