package state

import (
	"log"
	"sync"

	"github.com/tanq16/yamanaka/server/events"
)

// Manager holds the state of all connected clients for SSE.
type Manager struct {
	clients        map[string]chan interface{} // Channel now sends interface{} to accommodate different event types
	trackedClients map[string]bool             // Persistently tracked client IDs
	mutex          sync.RWMutex                // Mutex for client map operations
	dataDir        string
}

// FileSystemMutex protects file system operations in the vault.
var FileSystemMutex = &sync.RWMutex{}

// NewManager creates a new state manager.
func NewManager(dataDir string) *Manager {
	m := &Manager{
		clients:        make(map[string]chan interface{}),
		trackedClients: make(map[string]bool),
		dataDir:        dataDir,
	}
	m.trackedClients = LoadTrackedClients(m.dataDir, &m.mutex)
	return m
}

// AddClient registers a new client with its message channel.
func (m *Manager) AddClient(deviceID string, ch chan interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[deviceID] = ch

	if !m.trackedClients[deviceID] {
		m.trackedClients[deviceID] = true
		// Use a copy of trackedClients for saving to avoid holding lock in goroutine
		clientsToSave := make(map[string]bool)
		for k, v := range m.trackedClients {
			clientsToSave[k] = v
		}
		go SaveTrackedClients(m.dataDir, clientsToSave, &sync.RWMutex{}) // Pass a new mutex to avoid lock contention
	}
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
	case events.FullSyncEventData:
		eventType = "FullSyncEventData"
	default:
		eventType = "UnknownEvent"
	}

	log.Printf("Broadcasting %s (Path: %s) from sender '%s' to all clients.", eventType, targetPath, senderDeviceID)

	allClients := m.GetAllTrackedClients()
	for _, clientID := range allClients {
		if clientID == senderDeviceID {
			continue
		}

		if m.IsClientActive(clientID) {
			// Use a non-blocking send to prevent a slow client from blocking the broadcast.
			select {
			case m.clients[clientID] <- eventData:
			default:
				log.Printf("WARN: Channel for client %s is full. Skipping broadcast of %s.", clientID, eventType)
				// Also store as a missed event because the client might be overwhelmed
				StoreMissedEvent(m.dataDir, clientID, eventData)
			}
		} else {
			// Client is not active, store the event
			StoreMissedEvent(m.dataDir, clientID, eventData)
		}
	}
}
