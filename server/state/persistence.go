package state

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Ensure data directory exists
func ensureDataDir(dataDir string) {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("Failed to create data directory: %v", err)
		}
	}
}

// SaveTrackedClients saves the list of tracked client IDs to a file.
func SaveTrackedClients(dataDir string, clientIDs map[string]bool, mutex *sync.RWMutex) {
	mutex.RLock()
	defer mutex.RUnlock()

	ensureDataDir(dataDir)
	path := filepath.Join(dataDir, "clients.json")
	data, err := json.MarshalIndent(clientIDs, "", "  ")
	if err != nil {
		log.Printf("Error marshalling tracked clients: %v", err)
		return
	}
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		log.Printf("Error saving tracked clients: %v", err)
	}
}

// LoadTrackedClients loads the list of tracked client IDs from a file.
func LoadTrackedClients(dataDir string, mutex *sync.RWMutex) map[string]bool {
	mutex.Lock()
	defer mutex.Unlock()

	path := filepath.Join(dataDir, "clients.json")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("clients.json not found, starting with an empty set of tracked clients.")
			return make(map[string]bool)
		}
		log.Printf("Error reading tracked clients: %v", err)
		return make(map[string]bool)
	}

	var clientIDs map[string]bool
	if err := json.Unmarshal(data, &clientIDs); err != nil {
		log.Printf("Error unmarshalling tracked clients: %v", err)
		return make(map[string]bool)
	}
	return clientIDs
}
