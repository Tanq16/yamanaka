package events

// SSEEvent Types
const (
	SSEEventFileCreated      = "file_created"
	SSEEventFileUpdated      = "file_updated"
	SSEEventFileDeleted      = "file_deleted"
	SSEEventFullSyncRequired = "full_sync_required" // Sent when a client does an initial sync
)

// FileEventData is the payload for file-specific SSE events.
// It's used as the `data` field in an SSE message.
type FileEventData struct {
	Path           string `json:"path"`
	Content        string `json:"content,omitempty"`         // base64 encoded, empty for delete or if content not needed
	SenderDeviceID string `json:"-"`                       // Used internally to prevent echo, not marshalled
}

// FullSyncEventData is the payload for a full_sync_required SSE event.
type FullSyncEventData struct {
	Message        string `json:"message"`
	SenderDeviceID string `json:"-"` // Used internally to prevent echo, not marshalled
}
