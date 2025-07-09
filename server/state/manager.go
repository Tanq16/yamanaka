package state

import (
	"log"
	"sync"
)

// Manager holds the state of all connected clients for SSE.
type Manager struct {
	clients map[string]chan string
	mutex   sync.RWMutex
}

// NewManager creates a new state manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]chan string),
	}
}

// AddClient registers a new client with its message channel.
func (m *Manager) AddClient(deviceID string, ch chan string) {
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

// Broadcast sends a message (the new git hash) to all clients except the sender.
func (m *Manager) Broadcast(senderDeviceID, message string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	log.Printf("Broadcasting new hash '%s' from sender '%s'", message, senderDeviceID)
	for id, ch := range m.clients {
		if id != senderDeviceID {
			// Use a non-blocking send to prevent a slow client from blocking the broadcast.
			select {
			case ch <- message:
			default:
				log.Printf("WARN: Channel for client %s is full. Skipping broadcast.", id)
			}
		}
	}
}
