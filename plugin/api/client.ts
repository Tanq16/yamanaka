import { Notice } from 'obsidian';

// API response types from the Go server
interface CheckResponse {
	status: 'uptodate' | 'new_changes';
	latest_hash?: string;
}

interface SuccessResponse {
	status: 'success';
	new_hash: string;
}

interface PullResponse {
	hash: string;
	files: { path: string; content: string }[];
}

export class ApiClient {
    private baseUrl: string;
    private eventSource: EventSource | null = null;

    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
    }

    private async request(endpoint: string, options: RequestInit = {}): Promise<Response> {
        if (!this.baseUrl) {
            throw new Error("Server URL is not configured.");
        }
        const url = `${this.baseUrl}${endpoint}`;
        try {
            return await fetch(url, options);
        } catch (err) {
            console.error(`[Yamanaka] Network error for ${url}:`, err);
            throw new Error(`Failed to connect to the server at ${this.baseUrl}. Is it running?`);
        }
    }

    updateBaseUrl(newUrl: string) {
        this.baseUrl = newUrl.endsWith('/') ? newUrl.slice(0, -1) : newUrl;
        this.disconnectFromEvents();
    }

    async check(deviceId: string, currentHash: string): Promise<CheckResponse> {
        const response = await this.request(`/api/check?device_id=${deviceId}&current_hash=${currentHash}`);
        if (!response.ok) throw new Error(`Server check failed with status ${response.status}`);
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

    connectToEvents(deviceId: string, onNewChanges: (latestHash: string) => void) {
        if (!this.baseUrl || this.eventSource) {
            return;
        }
        const url = `${this.baseUrl}/api/events?device_id=${deviceId}`;
        this.eventSource = new EventSource(url);

        this.eventSource.onopen = () => {
            console.log('[Yamanaka] SSE connection established.');
            new Notice('Yamanaka: Real-time sync connected.');
        };

        this.eventSource.addEventListener('new_changes', (event) => {
            const data = JSON.parse(event.data);
            console.log('[Yamanaka] Received new_changes event with hash:', data.latest_hash);
            onNewChanges(data.latest_hash);
        });

        this.eventSource.onerror = (err) => {
            console.error('[Yamanaka] SSE connection error:', err);
            new Notice('Yamanaka: Real-time sync disconnected. Will try to reconnect.');
            this.disconnectFromEvents();
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
