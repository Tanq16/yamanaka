package state

import (
	"log"
	"sync"
	"yamanaka/events" // Import for event structures
)

// Manager holds the state of all connected clients for SSE.
type Manager struct {
	clients map[string]chan interface{} // Channel now sends interface{} to accommodate different event types
	mutex   sync.RWMutex                // Mutex for client map operations
}

// FileSystemMutex protects file system operations in the vault.
var FileSystemMutex = &sync.Mutex{}

// NewManager creates a new state manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]chan interface{}),
	}
}

// AddClient registers a new client with its message channel.
func (m *Manager) AddClient(deviceID string, ch chan interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[deviceID] = ch
}

// RemoveClient unregisters a client.
func (m *Manager) RemoveClient(deviceID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if ch, ok := m.clients[deviceID]; ok {
		close(ch)
		delete(m.clients, deviceID)
	}
}

// Broadcast sends an event to all clients except the sender.
// The event is expected to be one of the event types defined in the events package
// (e.g., events.FileEventData, events.FullSyncEventData).
func (m *Manager) Broadcast(senderDeviceID string, eventData interface{}) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Log the type of event being broadcast for clarity
	var eventType string
	var targetPath string // For file events

	switch data := eventData.(type) {
	case events.FileEventData:
		eventType = "FileEventData"
		targetPath = data.Path
		// Ensure SenderDeviceID is set in the event if it's a FileEventData for internal use,
		// though it won't be marshalled to JSON for the client.
		// This is more of a conceptual note; the comparison is on senderDeviceID passed to Broadcast.
	case events.FullSyncEventData:
		eventType = "FullSyncEventData"
	default:
		eventType = "UnknownEvent"
	}

	log.Printf("Broadcasting %s (Path: %s) from sender '%s' to other clients.", eventType, targetPath, senderDeviceID)

	for id, ch := range m.clients {
		if id != senderDeviceID {
			// Use a non-blocking send to prevent a slow client from blocking the broadcast.
			select {
			case ch <- eventData:
			default:
				log.Printf("WARN: Channel for client %s is full. Skipping broadcast of %s.", id, eventType)
			}
		}
	}
}
