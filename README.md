# Yamanaka - Self-Hosted Obsidian Sync

> [!IMPORTANT]
> This repository was entirely prepped by AI. This was by design, as I wanted to test the limits of AI-driven development. While this project pushed those boundaries, the underlying testing and building processes still followed established architectural paradigms. Expand the following to learn more about its creation.

<details>
<summary>How I Architected This</summary>
I have a strong preference for Go and enjoy using it for self-hosting applications. While I've manually developed several self-hosted apps, this project was a specific experiment to determine how far I could develop a repository primarily through prompting, with minimal manual effort.

For this project, I leveraged both Google Gemini and Jules. I began by performing a comprehensive 35-40 minute "brain dump" into Gemini, detailing the entire architecture. This included explaining the plugin's core functionality, its use of Server-Sent Events (SSE), and how Git commits were utilized for history preservation.

Following this initial architectural phase, I conducted two "Canvas sessions" with Gemini to initialize the repository and address initial bugs. Subsequently, I completed four distinct tasks with Jules to resolve further issues and integrate additional features.

The end result was a fully functional and highly effective solution. This approach completely resolved my previous issues with Obsidian Sync, eliminating the need for databases or complex syncing mechanisms. Instead, it relies on simple plaintext syncing, which, thanks to SSE, occurs remarkably quickly. Overall, it was a truly enjoyable and insightful experiment.
</details>

---

Yamanaka is a self-hosted synchronization solution for your Obsidian.md vault. It offers:
*   Real-time, bi-directional sync using Server-Sent Events (SSE).
*   Instantaneous backend updates and Git commits for versioning on every change.
*   A Go-based server and an Obsidian plugin.

## Quickstart

*   **Deploy Server:** Use Docker (see `server/Dockerfile`) or run the Go binary directly.
*   **Install Plugin:** Copy `main.js`, `styles.css`, `manifest.json` from `plugin/` to your vault's `.obsidian/plugins/yamanaka-self-hosted-sync/` directory.
*   **Configure Plugin:** In Obsidian settings, enable Yamanaka and set your server URL (e.g., `http://your_server_ip:8080`).
*   **Start Syncing:** Changes will sync automatically.

## Key Features

*   **Instant Sync:** Changes sync immediately across devices using Server-Sent Events (SSE).
*   **Self-Hosted Server:** Full control over your data with a Go-based backend.
*   **Obsidian Integration:** Companion plugin for seamless vault synchronization.
*   **Version History:**
    *   Server commits changes to Git *instantly* upon receiving them from a client.
    *   Additionally, a periodic Git commit (every 4 hours by default) ensures any other changes are captured.
*   **Easy Deployment:** Docker support for server and simple plugin install.

## Architecture Overview

Yamanaka uses a client-server model:

*   **Backend Server (Go):**
    *   Manages the central vault on its filesystem.
    *   Uses Git for versioning:
        *   Commits changes immediately when pushed by a client.
        *   Performs a periodic commit (default: every 4 hours) as a fallback.
    *   Provides an HTTP API for:
        *   File synchronization (push/pull).
        *   Initial vault setup.
        *   SSE for real-time updates.
    *   Broadcasts file changes via SSE to other connected clients.
*   **Obsidian Plugin (TypeScript):**
    *   Watches for local file changes (create, modify, delete, rename).
    *   Pushes these changes to the server.
    *   Subscribes to server's SSE feed for remote changes and applies them locally.

### Data Flow for Real-time Sync (User Interaction Diagram)

```mermaid
sequenceDiagram
    participant ClientA as Obsidian Client A
    participant ObsidianAPI
    participant Server
    participant ClientB as Obsidian Client B

    Note over ClientA,ClientB: Both connected to Server via SSE (/api/events)

    ClientA->>+ObsidianAPI: User creates/modifies file "note.md"
    ObsidianAPI-->>-ClientA: Vault event triggered
    ClientA->>+Server: POST /api/sync/push (path: "note.md", content: base64_data)
    Server->>Server: Writes "note.md" to its filesystem
    Server->>Server: Commits change to Git
    Server-->>-ClientA: HTTP 200 OK (Push successful)
    Server->>ClientB: SSE Event (event: file_updated, data: {path: "note.md", content: base64_data})

    ClientB->>+ObsidianAPI: Applies change to local "note.md"
    ObsidianAPI-->>-ClientB: Local vault updated
```

## Installation

### 1. Backend Server Setup

*   **Requirements:** Go (1.21+), Git.
*   **Docker (Recommended):**
    1.  Go to `server/` directory.
    2.  Build: `docker build -t yamanaka-server .`
    3.  Run: `docker run -d -p 8080:8080 -v /path/to/your/vault_storage:/app/data --name yamanaka yamanaka-server`
        *   Replace `/path/to/your/vault_storage` with your desired host path for vault data.
*   **Directly with Go:**
    1.  Navigate to `server/`.
    2.  Build: `go build -o yamanaka-server .`
    3.  Run: `./yamanaka-server` (data stored in `server/data/`).

### 2. Obsidian Plugin Setup

1.  Get plugin files (`main.js`, `styles.css`, `manifest.json`):
    *   Download from releases (TODO: Link).
    *   Or build from source: `cd plugin/`, `npm install`, `npm run build`.
2.  Copy to `<YourVault>/.obsidian/plugins/yamanaka-self-hosted-sync/`.
3.  In Obsidian: `Settings` > `Community plugins` > Enable `Yamanaka`.
4.  Configure plugin settings:
    *   **Server URL:** e.g., `http://your_server_ip:8080`.
    *   Enable **Auto Sync**.

## Usage

*   **Automatic Sync:** Enabled by default. File changes are synced in real-time.
*   **Manual Commands:**
    *   `Yamanaka: Manual Push`: Push local changes.
    *   `Yamanaka: Manual Pull`: Fetch entire vault from server.
*   **Initial Sync:**
    *   The plugin's "Initial Sync" button (in settings) will replace the server's vault with the current client's vault. Use with caution. (TODO: Confirm button existence/functionality based on latest plugin code).

## Contributing

(Details TBD)

## License

[MIT License](LICENSE).
