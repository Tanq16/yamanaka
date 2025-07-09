import { Notice } from 'obsidian';

// API response types from the Go server (updated to reflect backend changes)
interface CheckResponse {
	status: 'ok' | 'error'; // Simplified, hash comparison is removed
	// latest_hash?: string; // Removed
}

interface SuccessResponse {
	status: string; // e.g., "success, push processed and changes broadcasted"
	// new_hash: string; // Removed
}

interface PullResponse {
	// hash: string; // Removed
	files: { path: string; content: string }[]; // content is base64
}

// Event data types from the server for SSE
export interface FileEventData {
    path: string;
    content?: string; // base64 encoded, empty for delete
    // sender_device_id is not expected here as it's filtered by server/client
}

export interface FullSyncEventData {
    message: string;
}


export class ApiClient {
    private baseUrl: string;
    private eventSource: EventSource | null = null;

    constructor(baseUrl: string) {
        this.baseUrl = this.normalizeBaseUrl(baseUrl);
    }

    private async request(endpoint: string, options: RequestInit = {}): Promise<Response> {
        if (!this.baseUrl) {
            throw new Error("Server URL is not configured.");
        }
        // Ensure endpoint starts with a slash if baseUrl doesn't have one and endpoint is not empty
        let fullUrl = this.baseUrl;
        if (endpoint) {
            if (this.baseUrl.endsWith('/') && endpoint.startsWith('/')) {
                fullUrl = this.baseUrl + endpoint.substring(1);
            } else if (!this.baseUrl.endsWith('/') && !endpoint.startsWith('/')) {
                fullUrl = this.baseUrl + '/' + endpoint;
            } else {
                fullUrl = this.baseUrl + endpoint;
            }
        }

        console.log(`[Yamanaka] Making request to: ${fullUrl}`); // Added for debugging

        try {
            // Obsidian's fetch requires a proper scheme for external requests.
            // The `this.baseUrl` should already be normalized by constructor or updateBaseUrl.
            return await fetch(fullUrl, options);
        } catch (err) {
            console.error(`[Yamanaka] Network error for ${fullUrl}:`, err);
            // Check if the error is due to missing scheme (TypeError in browser-like environments)
            if (err instanceof TypeError && (err.message.includes("Invalid URL") || err.message.includes("Failed to parse URL") || err.message.includes("URL constructor:is not a valid URL."))) {
                 throw new Error(`Invalid server URL: ${this.baseUrl}. Please ensure it includes a scheme (e.g., http:// or https://).`);
            }
            throw new Error(`Failed to connect to the server at ${this.baseUrl}. Is it running? Error: ${err.message}`);
        }
    }

    private normalizeBaseUrl(url: string): string {
        if (!url) return '';
        let normalizedUrl = url.trim();
        if (!normalizedUrl.match(/^https?:\/\//i)) {
            normalizedUrl = 'http://' + normalizedUrl;
            console.log(`[Yamanaka] Prepended http:// to base URL. New URL: ${normalizedUrl}`);
        }
        // Remove trailing slash for consistency, request method will handle endpoint joining
        if (normalizedUrl.endsWith('/')) {
            normalizedUrl = normalizedUrl.slice(0, -1);
        }
        return normalizedUrl;
    }

    updateBaseUrl(newUrl: string) {
        this.baseUrl = this.normalizeBaseUrl(newUrl);
        this.disconnectFromEvents(); // Important to disconnect before potentially connecting to a new URL
        // Note: Reconnection should be triggered by the caller (e.g., settings tab or main plugin logic)
        // This is already handled in settings tab: this.plugin.connectToEvents();
    }

    async check(deviceId: string /*, currentHash: string // No longer needed */): Promise<CheckResponse> {
        const response = await this.request(`/api/check?device_id=${deviceId}`); // current_hash parameter removed
        if (!response.ok) {
            // Try to parse error from server if possible, otherwise generic error
            let errorMsg = `Server check failed with status ${response.status}`;
            try {
                const errorBody = await response.json();
                if (errorBody && errorBody.error) {
                    errorMsg = errorBody.error;
                }
            } catch(e) { /* ignore parsing error */ }
            throw new Error(errorMsg);
        }
        return response.json();
    }

    async pull(deviceId: string): Promise<PullResponse> {
        const response = await this.request(`/api/sync/pull?device_id=${deviceId}`);
        if (!response.ok) throw new Error(`Pull failed with status ${response.status}`);
        return response.json();
    }

    async initialSync(deviceId: string, archive: Blob): Promise<SuccessResponse> {
        const response = await this.request(`/api/sync/initial?device_id=${deviceId}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/gzip' },
            body: archive,
        });
        if (!response.ok) throw new Error(`Initial sync failed with status ${response.status}`);
        return response.json();
    }

    async push(deviceId: string, filesToUpdate: { path: string; content: string }[], filesToDelete: string[]): Promise<SuccessResponse> {
        const payload = {
            files_to_update: filesToUpdate,
            files_to_delete: filesToDelete,
        };
        const response = await this.request(`/api/sync/push?device_id=${deviceId}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        });
        if (!response.ok) throw new Error(`Push failed with status ${response.status}`);
        return response.json();
    }

    connectToEvents(
        deviceId: string,
        onFileUpdated: (data: FileEventData) => void,
        onFileDeleted: (data: FileEventData) => void,
        onFullSyncRequired: (data: FullSyncEventData) => void
    ) {
        if (!this.baseUrl) {
            console.warn('[Yamanaka] Base URL not set. Cannot connect to SSE.');
            new Notice('Yamanaka: Server URL not configured. SSE connection failed.');
            return;
        }
        if (this.eventSource) {
            console.log('[Yamanaka] SSE connection already exists or is being established.');
            return;
        }

        const url = `${this.baseUrl}/api/events?device_id=${deviceId}`;
        console.log(`[Yamanaka] Attempting to connect to SSE at ${url}`);
        this.eventSource = new EventSource(url);

        this.eventSource.onopen = () => {
            console.log('[Yamanaka] SSE connection established.');
            new Notice('Yamanaka: Real-time sync connected.');
        };

        this.eventSource.addEventListener('file_updated', (event: MessageEvent) => {
            try {
                const data = JSON.parse(event.data) as FileEventData;
                console.log('[Yamanaka] SSE file_updated:', data);
                onFileUpdated(data);
            } catch (e) {
                console.error('[Yamanaka] Error parsing file_updated event data:', e, event.data);
            }
        });

        this.eventSource.addEventListener('file_deleted', (event: MessageEvent) => {
            try {
                const data = JSON.parse(event.data) as FileEventData;
                console.log('[Yamanaka] SSE file_deleted:', data);
                onFileDeleted(data);
            } catch (e) {
                console.error('[Yamanaka] Error parsing file_deleted event data:', e, event.data);
            }
        });

        this.eventSource.addEventListener('full_sync_required', (event: MessageEvent) => {
            try {
                const data = JSON.parse(event.data) as FullSyncEventData;
                console.log('[Yamanaka] SSE full_sync_required:', data);
                onFullSyncRequired(data);
            } catch (e) {
                console.error('[Yamanaka] Error parsing full_sync_required event data:', e, event.data);
            }
        });

        this.eventSource.onerror = (err) => {
            console.error('[Yamanaka] SSE connection error:', err);
            // EventSource attempts to reconnect automatically by default.
            // We might want to inform the user after several failed attempts or if it's an immediate closure.
            if (this.eventSource?.readyState === EventSource.CLOSED) {
                 new Notice('Yamanaka: Real-time sync disconnected. Check server and settings.');
            } else {
                new Notice('Yamanaka: Real-time sync connection issue. Will attempt to reconnect.');
            }
            // No need to call disconnectFromEvents() here, as EventSource handles retries.
            // If retries fail persistently, it will remain in a broken state.
            // We might want to nullify this.eventSource only on explicit disconnect or unload.
        };
    }

    disconnectFromEvents() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
            console.log('[Yamanaka] SSE connection closed.');
        }
    }
}
