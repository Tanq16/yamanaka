# yamanaka
Obsidian Sync Plugin


------

# Plan of Implementation

### High-Level Architecture

The system consists of two main parts: the **Obsidian Plugin (Client)** and the **Go Backend (Server)**. They communicate exclusively over an HTTP API.

1. **Client (Obsidian Plugin):**
    - Lives inside each user's Obsidian vault.
    - Each instance generates and stores a unique `device_id`.
    - It watches the local file system for changes.
    - It communicates with the Server to push local changes and pull remote changes.
2. **Server (Go Backend):**
    - A single, self-hosted application (run via Docker).
    - It stores the "master" copy of the Obsidian vault on its file system.
    - It initializes this master copy as a Git repository to track all history.
    - It exposes an HTTP API for clients to interact with.
    - It keeps track of the latest version (via Git commit hash) and notifies clients of new updates.

### Part 1: The Go Backend Server

The server's primary job is to be the central source of truth and to orchestrate the synchronization.

#### **1. Project Structure (Go)**

```
/obsidian-sync-server
|-- go.mod
|-- go.sum
|-- main.go         // Entry point, HTTP server setup
|-- api/            // API handlers (sync, check, etc.)
|   |-- handlers.go
|-- vault/          // Logic for interacting with the vault files
|   |-- git.go      // Functions to run git commands (commit, hash, etc.)
|   |-- files.go    // Functions for file I/O
|-- state/          // Logic for managing device state
|   |-- manager.go  // In-memory map of device states and SSE channels
|-- Dockerfile      // To containerize the server
|-- data/           // IMPORTANT: This is where the vault will be stored
    |-- .git/
    |-- Your Note.md
```

#### **2. Core Logic**

- **State Management:** The server needs to know about connected clients. An in-memory map is sufficient for a simple start.
    - `map[string]chan string` could hold SSE channels for each `device_id`. When a change is committed, the server iterates through this map and sends a "new_changes" message to every _other_ device.
- **Git as the Source of Truth:**
    - On startup, the server checks if `data/.git` exists. If not, it runs `git init` in the `data/` directory.
    - Every successful `Initial Sync` or `Push` operation will result in a new Git commit.
        - `git add .`
        - `git commit -m "Sync from device [device_id]"`
    - The latest Git commit hash (`git rev-parse HEAD`) becomes the identifier for the current version of the vault. This is simple and foolproof.

#### **3. API Endpoints (HTTP)**

The server will expose the following endpoints:

- **`GET /api/check`**
    - **Query Params:** `device_id={uuid}`, `current_hash={git_commit_hash}`
    - **Logic:** The server compares the client's `current_hash` with its own latest commit hash.
    - **Response:**
        - If they match: `{"status": "uptodate"}`
        - If they differ: `{"status": "new_changes", "latest_hash": "..."}`
- **`POST /api/sync/initial`**
    - **Query Params:** `device_id={uuid}`
    - **Body:** A `tar.gz` archive of the entire vault.
    - **Logic:**
        1. Delete all files in the server's `data/` directory (except `.git`).
        2. Extract the archive into `data/`.
        3. Run the Git commit cycle.
        4. Notify all other connected devices via SSE that there are new changes.
    - **Response:** `{"status": "success", "new_hash": "..."}`
- **`POST /api/sync/push`**
    - **Query Params:** `device_id={uuid}`
    - **Body:** JSON array of changed/deleted files.
        ```
        {
          "files_to_update": [
            { "path": "folder/My Note.md", "content": "base64-encoded-content" },
            { "path": "Another Note.md", "content": "..." }
          ],
          "files_to_delete": [
            "path/to/Old Note.md"
          ]
        }
        ```
    - **Logic:**
        1. Iterate through `files_to_update`, decode the content, and write each file to the `data/` directory, creating subdirectories as needed.
        2. Iterate through `files_to_delete` and remove each file.
        3. Run the Git commit cycle.
        4. Notify other devices via SSE.
    - **Response:** `{"status": "success", "new_hash": "..."}`
- **`GET /api/sync/pull`**
    - **Query Params:** `device_id={uuid}`
    - **Logic:** Walk the entire `data/` directory (ignoring `.git`) and read every file.
    - **Response:** JSON object containing all files.
        ```
        {
          "hash": "latest_commit_hash",
          "files": [
            { "path": "folder/My Note.md", "content": "base64-encoded-content" },
            { "path": "Another Note.md", "content": "..." }
          ]
        }
        ```
- **`GET /api/events` (For Server-Sent Events)**
    - **Query Params:** `device_id={uuid}`
    - **Logic:** This is a long-lived connection. The handler adds the connection's response writer to the central state manager. When a change occurs (from another device's push), the server sends an event down this connection.
    - **Event Format:** `event: new_changes\ndata: {"latest_hash": "..."}\n\n`

#### **4. Dockerization**

A `Dockerfile` will make deployment trivial.

```
# Use an official Golang runtime as a parent image
FROM golang:1.21-alpine

WORKDIR /app

# Copy the Go module files and download dependencies
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go app
RUN go build -o /obsidian-sync-server .

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD [ "/obsidian-sync-server" ]
```

### Part 2: The Obsidian Plugin (Frontend)

The plugin is the user-facing component that lives in their vault.

#### **1. Project Structure (TypeScript)**

This follows the standard Obsidian plugin template.

```
/obsidian-sync-plugin
|-- .hotreload    # For development
|-- node_modules/
|-- main.ts       # Plugin entry point, lifecycle hooks
|-- manifest.json # Plugin metadata
|-- styles.css    # CSS for the settings tab
|-- api/
|   |-- client.ts # Functions to call the backend API
|-- settings/
|   |-- tab.ts    # The UI for the settings page
|-- sync/
|   |-- manager.ts # Core logic for push, pull, auto-sync
|-- tsconfig.json
|-- package.json
```

#### **2. Core Logic**

- **Device ID:** On first load (`onload`), check if a `deviceId` is stored in the plugin's data (`this.loadData()`). If not, generate a UUID v4 and save it.
- **State Tracking:** The plugin must store the `last_sync_hash` locally. This is crucial for the `/check` API call.
- **Automatic Push:**
    - Use `this.registerEvent(this.app.vault.on('modify', ...))` to detect file changes.
    - To avoid syncing on every keystroke, use a "debouncer". When a file is modified, you start a 30-second timer. If another modification happens within that window, you reset the timer. The push operation only happens after 30 seconds of inactivity.
- **Automatic Pull (via SSE):**
    - On `onload`, establish a connection to the `/api/events` endpoint.
    - Listen for the `new_changes` event.
    - When the event is received, compare its `latest_hash` with the plugin's stored `last_sync_hash`.
    - If they are different, automatically trigger a `pull()` operation. This is far more efficient than polling every 30 seconds.

#### **3. Settings Page UI (`settings/tab.ts`)**

This will be a simple UI with:

- Input field for the Server URL (e.g., `http://localhost:8080`).
- A status indicator (`Last synced: [time]`, `Status: Connected / Up-to-date / Changes pending`).
- **Four Buttons:**
    1. **Initial Sync:** Wipes the server and pushes the entire local vault. Should have a big warning confirmation.
    2. **Sync (Push):** Manually triggers a push of all changed files since the last sync.
    3. **Pull:** Fetches the entire vault from the server, overwriting all local files. Also needs a warning.
    4. **Check Status:** Manually triggers a call to the `/check` endpoint and updates the status indicator.

### Monorepo Structure

To manage this as a single project, you can structure your repository like this:

```
/obsidian-self-hosted-sync
|-- server/          # Contains the Go backend project
|   |-- main.go
|   |-- Dockerfile
|   |-- ...
|-- plugin/          # Contains the Obsidian plugin project
|   |-- main.ts
|   |-- manifest.json
|   |-- ...
|-- .gitignore
|-- README.md
```

### Development Roadmap

1. **Setup the Monorepo:** Create the directory structure above.
2. **Backend: Basic Server & API:**
    - Create the Go project.
    - Implement the HTTP server and stub out all the API endpoints. Have them return mock data for now.
3. **Backend: Git & File Logic:**
    - Implement the `git.go` functions to `init`, `add`, `commit`, and get the current `hash`.
    - Implement the file I/O logic for the `push` and `pull` handlers.
4. **Plugin: Basic Shell & Settings:**
    - Use the [Obsidian Sample Plugin](https://github.com/obsidianmd/obsidian-sample-plugin "null") template to create the plugin project.
    - Build the settings tab UI with the server URL input and the four buttons.
5. **Connect Plugin to Backend:**
    - Implement the `api/client.ts` in the plugin.
    - Wire up the `Initial Sync` and `Pull` buttons to their respective API calls. This is the most critical connection to test first.
6. **Implement Push Logic:**
    - Implement the logic to detect changed files in the plugin.
    - Wire up the `Sync (Push)` button.
7. **Implement Auto-Sync & Real-Time:**
    - Add the debounced auto-push functionality to the plugin.
    - Implement the SSE handler on the server and the event listener in the plugin to enable automatic pulling of remote changes.
8. **Dockerize & Refine:**
    - Finalize the `Dockerfile` for the server.
    - Write a `README.md` with instructions on how to run the server via Docker and install the plugin. Test the entire workflow from scratch.

This plan provides a clear path from concept to a fully functional, self-hosted sync solution for Obsidian. The approach is robust, efficient, and avoids unnecessary complexity.
