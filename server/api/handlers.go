package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/tanq16/yamanaka/server/events"
	"github.com/tanq16/yamanaka/server/state"
	"github.com/tanq16/yamanaka/server/vault"
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
	Status string `json:"status"`
	// LatestHash string `json:"latest_hash,omitempty"` // Git hash is no longer the primary sync mechanism from client's perspective
}

type SuccessResponse struct {
	Status string `json:"status"`
	// NewHash string `json:"new_hash"` // Git hash is no longer immediately relevant to the client operation
}

type PullResponse struct {
	// Hash  string      `json:"hash"` // Git hash is no longer the primary sync mechanism
	Files []vault.File `json:"files"`
}

type PushRequest struct {
	FilesToUpdate []vault.File `json:"files_to_update"`
	FilesToDelete []string     `json:"files_to_delete"`
}

// --- Handlers ---

// CheckHandler compares the client's hash with the server's latest git hash.
func (h *ApiHandler) CheckHandler(w http.ResponseWriter, r *http.Request) {
	// clientHash := r.URL.Query().Get("current_hash") // Client hash comparison is removed

	// This handler's utility is significantly reduced with Git-decoupled sync.
	// For now, it just confirms the server is alive.
	// Clients will rely on SSE for real-time updates and `/api/sync/pull` for full state.
	// _, err := vault.GetCurrentHash(h.VaultPath) // No longer get server hash for this
	// if err != nil {
	// 	http.Error(w, "Could not get server status", http.StatusInternalServerError)
	// 	return
	// }
	// Always return "ok", client decides if it needs to pull or rely on SSE.
	json.NewEncoder(w).Encode(CheckResponse{Status: "ok"})
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

	// 3. File operations successful. Git commit is handled by a periodic background job (and is irrelevant to this handler now).
	// Notify other clients that a full sync might be required for them, as the entire vault state was replaced.
	fullSyncMessage := fmt.Sprintf("Vault was reset and populated by an initial sync from device %s. Other clients should perform a full pull if they need the latest state.", deviceID)
	h.StateManager.Broadcast(deviceID, events.FullSyncEventData{
		Message: fullSyncMessage,
		// SenderDeviceID is implicitly handled by the Broadcast method logic to not send to self
	})

	// Respond to the initiating client.
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{Status: "success, initial sync processed. Other clients notified."})
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
			log.Printf("WARN: PushHandler: Could not delete file %s: %v. Skipping SSE broadcast for this file.", path, err)
			// Optionally, you could send an error event to the originating client, but not broadcast a delete.
			continue
		}
		// Broadcast delete event
		log.Printf("PushHandler: File %s deleted by %s. Broadcasting.", path, deviceID)
		h.StateManager.Broadcast(deviceID, events.FileEventData{
			Path: path,
			// Content is empty for delete
			// SenderDeviceID is handled by Broadcast
		})
	}

	// 2. Process files to update/create
	for _, file := range req.FilesToUpdate {
		// Note: file.Content is already base64 encoded from the client request
		contentBytes, err := base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			log.Printf("WARN: PushHandler: Could not decode file content for %s from device %s: %v. Skipping.", file.Path, deviceID, err)
			continue
		}
		if err := vault.WriteFile(h.VaultPath, file.Path, contentBytes); err != nil {
			log.Printf("WARN: PushHandler: Could not write file %s from device %s: %v. Skipping SSE broadcast for this file.", file.Path, deviceID, err)
			continue
		}
		// Broadcast update/create event
		log.Printf("PushHandler: File %s updated/created by %s. Broadcasting.", file.Path, deviceID)
		h.StateManager.Broadcast(deviceID, events.FileEventData{
			Path:    file.Path,
			Content: file.Content, // Send the base64 content as received
			// SenderDeviceID is handled by Broadcast
		})
	}

	// 3. Respond to the client
	// Commit changes to Git after processing all files and before responding to the client.
	// This makes the backend changes persistent immediately.
	commitMsg := fmt.Sprintf("Client push from device %s", deviceID)
	_, err := vault.CommitChanges(h.VaultPath, commitMsg)
	if err != nil {
		// Log the error, but don't fail the entire push operation,
		// as files are written and SSE events are broadcasted.
		// The periodic commit will eventually pick up these changes if this one fails.
		log.Printf("ERROR: PushHandler: Failed to commit changes for device %s: %v", deviceID, err)
	} else {
		log.Printf("PushHandler: Changes committed to Git for device %s.", deviceID)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{Status: "success, push processed and changes broadcasted"})
}

// PullHandler sends the entire current state of the vault to the client.
func (h *ApiHandler) PullHandler(w http.ResponseWriter, r *http.Request) {
	// currentHash, err := vault.GetCurrentHash(h.VaultPath) // Git hash is no longer sent
	// if err != nil {
	// 	http.Error(w, "Could not get server hash", http.StatusInternalServerError)
	// 	return
	// }

	files, err := vault.GetAllFiles(h.VaultPath) // This function reads directly from the filesystem
	if err != nil {
		http.Error(w, "Could not read vault files", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PullResponse{
		// Hash:  currentHash, // Removed
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

	// Create a channel for this client that can send various event types
	eventChan := make(chan interface{})
	h.StateManager.AddClient(deviceID, eventChan)
	defer h.StateManager.RemoveClient(deviceID)

	log.Printf("Client %s connected for events", deviceID)

	// Listen for context cancellation (client disconnects)
	ctx := r.Context()
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Retrieve and send missed events
	missedEvents := state.RetrieveAndClearMissedEvents(h.VaultPath, deviceID)
	if len(missedEvents) > 10 { // Threshold for full sync
		log.Printf("Client %s has %d missed events, requiring a full sync.", deviceID, len(missedEvents))
		fullSyncEvent := events.FullSyncEventData{
			Message: fmt.Sprintf("You have %d missed updates. A full sync is required.", len(missedEvents)),
		}
		jsonData, err := json.Marshal(fullSyncEvent)
		if err != nil {
			log.Printf("Error marshalling full sync event for %s: %v", deviceID, err)
		} else {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", events.SSEEventFullSyncRequired, string(jsonData))
			flusher.Flush()
		}
	} else if len(missedEvents) > 0 {
		log.Printf("Sending %d missed events to client %s", len(missedEvents), deviceID)
		for _, eventMsg := range missedEvents {
			var eventName string
			var jsonData []byte
			var err error

			switch specificEvent := eventMsg.(type) {
			case map[string]interface{}: // Unmarshalled from JSON
				_, pathOk := specificEvent["path"].(string)
				content, contentOk := specificEvent["content"].(string)

				if pathOk {
					if contentOk && content != "" {
						eventName = events.SSEEventFileUpdated
					} else {
						eventName = events.SSEEventFileDeleted
					}
				} else if _, msgOk := specificEvent["message"]; msgOk {
					eventName = events.SSEEventFullSyncRequired
				}

				jsonData, err = json.Marshal(specificEvent)

			default:
				log.Printf("Unknown type in missed events: %T", eventMsg)
				continue
			}

			if err != nil {
				log.Printf("Error marshalling missed event for %s: %v", deviceID, err)
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(jsonData))
		}
		flusher.Flush()
	}

	// Heartbeat ticker
	heartbeatTicker := time.NewTicker(2 * time.Minute)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-heartbeatTicker.C:
			// Send a comment as a heartbeat
			// SSE comments start with a colon and are ignored by EventSource implementations
			// but they keep the connection alive.
			fmt.Fprintf(w, ":heartbeat\n\n")
			flusher.Flush()
			log.Printf("Sent heartbeat to client %s", deviceID)
		case eventMsg := <-eventChan:
			var eventName string
			var jsonData []byte
			var err error

			switch specificEvent := eventMsg.(type) {
			case events.FileEventData:
				if specificEvent.Content == "" { // Assume delete if content is empty, path is present
					eventName = events.SSEEventFileDeleted
				} else { // Assume create or update
					// Could differentiate C/U based on prior existence, but client can usually infer or upsert.
					// For now, let's use a generic "updated" for simplicity if content is present.
					// Or, rely on client to know if file exists locally.
					// To be more precise, PushHandler should ideally tell us if it was a create or update.
					// For now, we'll use SSEEventFileUpdated if content is present.
					// A better approach would be for PushHandler to send different event types.
					// Let's assume PushHandler sends FileEventData for C/U/D and we determine type here.
					// No, PushHandler will send FileEventData, and we determine eventName here based on content.
					// Let's refine: PushHandler will set a temporary type field or we infer.
					// For now: if content is present, it's an update/create. If not, it's a delete.
					// The events.go defines SSEEventFileCreated, SSEEventFileUpdated, SSEEventFileDeleted.
					// Let's assume the broadcaster (PushHandler) will send the correct specific event type
					// or we enhance FileEventData to include an Action (Create, Update, Delete).

					// Simpler: StateManager's Broadcast will receive specific event structs.
					// Here, we just need to marshal what we receive.
					// The event NAME (file_created, file_updated) needs to be determined.

					// Let's assume for FileEventData, if Content is empty, it's a delete. Otherwise, update/create.
					// This is a simplification. A more robust way is for the sender (PushHandler)
					// to create specific event types (e.g. events.FileCreatedEventData, events.FileUpdatedEventData),
					// but that requires more event types in events.go.
					// Given current events.FileEventData, we infer.
					if specificEvent.Content != "" {
						eventName = events.SSEEventFileUpdated // Or created. Client plugin can upsert.
					} else {
						eventName = events.SSEEventFileDeleted
					}
					jsonData, err = json.Marshal(specificEvent)
				}
				// How PushHandler signals create vs update for FileEventData:
				// Option 1: PushHandler sends different types (e.g. `events.FileCreatedData`, `events.FileUpdatedData`). Manager broadcasts `interface{}`. EventsHandler type switches.
				// Option 2: FileEventData gets an `Action` field: "create", "update", "delete".
				// For now, `PushHandler` creates `events.FileEventData`. If `Content` is present, it's `file_updated`. If `Content` is absent, it's `file_deleted`.
				// This means "create" is also signaled as "file_updated". Client plugin handles this by creating if not exist, updating if exists.

			case events.FullSyncEventData:
				eventName = events.SSEEventFullSyncRequired
				jsonData, err = json.Marshal(specificEvent)
			default:
				log.Printf("EventsHandler: Unknown event type received for device %s: %T", deviceID, eventMsg)
				continue // Skip unknown event types
			}

			if err != nil {
				log.Printf("EventsHandler: Error marshalling event data for device %s: %v", deviceID, err)
				continue
			}

			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(jsonData))
			flusher.Flush()

		case <-ctx.Done():
			// Client has disconnected
			log.Printf("Client %s disconnected from event stream", deviceID)
			return
		}
	}
}
