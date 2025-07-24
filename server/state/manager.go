package state

import (
	"log/slog"
	"maps"
	"sync"

	"github.com/tanq16/yamanaka/server/events"
)

// holds the state of all connected clients for SSE
type Manager struct {
	clients        map[string]chan any // any accommodates different event types
	trackedClients map[string]bool
	mutex          sync.RWMutex
	dataDir        string
}

var FileSystemMutex = &sync.RWMutex{}

// creates a new state manager
func NewManager(dataDir string) *Manager {
	m := &Manager{
		clients:        make(map[string]chan any),
		trackedClients: make(map[string]bool),
		dataDir:        dataDir,
	}
	m.trackedClients = LoadTrackedClients(m.dataDir, &m.mutex)
	return m
}

// registers a new client with its message channel
func (m *Manager) AddClient(deviceID string, ch chan any) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[deviceID] = ch
	if !m.trackedClients[deviceID] {
		m.trackedClients[deviceID] = true
		clientsToSave := make(map[string]bool)
		maps.Copy(clientsToSave, m.trackedClients) // use copy to avoid holding lock in goroutine
		go SaveTrackedClients(m.dataDir, clientsToSave, &sync.RWMutex{})
	}
}

// unregisters a client
func (m *Manager) RemoveClient(deviceID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if ch, ok := m.clients[deviceID]; ok {
		close(ch)
		delete(m.clients, deviceID)
	}
}

// sends an event to all clients except the sender.
func (m *Manager) Broadcast(senderDeviceID string, eventData any) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	var eventType string
	var targetPath string
	switch data := eventData.(type) {
	case events.FileEventData:
		eventType = "FileEventData"
		targetPath = data.Path
	case events.FullSyncEventData:
		eventType = "FullSyncEventData"
	default:
		eventType = "UnknownEvent"
	}
	slog.Info("broadcast", "event", eventType, "path", targetPath, "sender", senderDeviceID)

	allClients := m.GetAllTrackedClients()
	for _, clientID := range allClients {
		if clientID == senderDeviceID {
			continue
		}
		if m.IsClientActive(clientID) {
			select {
			case m.clients[clientID] <- eventData:
			default:
				slog.Warn("channel is full, skipping broadcast", "client", clientID, "event", eventType)
				StoreMissedEvent(m.dataDir, clientID, eventData)
			}
		} else {
			StoreMissedEvent(m.dataDir, clientID, eventData)
		}
	}
}
