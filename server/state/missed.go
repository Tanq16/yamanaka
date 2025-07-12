package state

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const missedEventsDir = "missed_events"

// StoreMissedEvent saves an event for a specific client who is not currently connected.
func StoreMissedEvent(dataDir string, clientID string, eventData interface{}) {
	clientDir := filepath.Join(dataDir, missedEventsDir, clientID)
	if err := os.MkdirAll(clientDir, 0755); err != nil {
		log.Printf("ERROR: Could not create directory for missed events for client %s: %v", clientID, err)
		return
	}

	// Filename based on timestamp to ensure order
	timestamp := time.Now().UnixNano()
	fileName := fmt.Sprintf("%d.json", timestamp)
	filePath := filepath.Join(clientDir, fileName)

	data, err := json.Marshal(eventData)
	if err != nil {
		log.Printf("ERROR: Could not marshal missed event for client %s: %v", clientID, err)
		return
	}

	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		log.Printf("ERROR: Could not write missed event to file for client %s: %v", clientID, err)
	}
}

// RetrieveAndClearMissedEvents gets all stored events for a client and then clears them.
func RetrieveAndClearMissedEvents(dataDir string, clientID string) []interface{} {
	clientDir := filepath.Join(dataDir, missedEventsDir, clientID)
	if _, err := os.Stat(clientDir); os.IsNotExist(err) {
		return nil // No missed events
	}

	files, err := ioutil.ReadDir(clientDir)
	if err != nil {
		log.Printf("ERROR: Could not read missed events directory for client %s: %v", clientID, err)
		return nil
	}

	// Sort files by timestamp in the filename to ensure chronological order
	sort.Slice(files, func(i, j int) bool {
		ts1, _ := strconv.ParseInt(strings.TrimSuffix(files[i].Name(), ".json"), 10, 64)
		ts2, _ := strconv.ParseInt(strings.TrimSuffix(files[j].Name(), ".json"), 10, 64)
		return ts1 < ts2
	})

	var events []interface{}
	for _, file := range files {
		filePath := filepath.Join(clientDir, file.Name())
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("ERROR: Could not read missed event file %s for client %s: %v", file.Name(), clientID, err)
			continue
		}

		var eventData interface{}
		if err := json.Unmarshal(data, &eventData); err != nil {
			log.Printf("ERROR: Could not unmarshal missed event file %s for client %s: %v", file.Name(), clientID, err)
			continue
		}
		events = append(events, eventData)
	}

	// Clear the directory after retrieving events
	if err := os.RemoveAll(clientDir); err != nil {
		log.Printf("ERROR: Could not clear missed events directory for client %s: %v", clientID, err)
	}

	return events
}

// IsClientActive checks if a client has an active SSE connection.
func (m *Manager) IsClientActive(clientID string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, ok := m.clients[clientID]
	return ok
}

// GetAllTrackedClients returns a list of all client IDs that have ever connected.
func (m *Manager) GetAllTrackedClients() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	ids := make([]string, 0, len(m.trackedClients))
	for id := range m.trackedClients {
		ids = append(ids, id)
	}
	return ids
}

// In this file, we're adding the functions to handle missed events.
// The next step is to modify the Broadcast function in manager.go to use these.
// We also add IsClientActive and GetAllTrackedClients to the Manager.
var _ = &sync.Mutex{} // Dummy use of sync to avoid import error if FileSystemMutex is removed
