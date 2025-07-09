import { App, Notice, Plugin, TAbstractFile, TFile } from 'obsidian';
import { v4 as uuidv4 } from 'uuid';
import { ApiClient } from './api/client';
import { YamanakaSettingTab } from './settings/tab';
import { SyncManager } from './sync/manager';

interface YamanakaPluginSettings {
	serverUrl: string;
	deviceId: string;
    lastSyncHash: string;
    autoSync: boolean;
}

const DEFAULT_SETTINGS: YamanakaPluginSettings = {
	serverUrl: '',
	deviceId: '',
    lastSyncHash: '',
    autoSync: true,
}

export default class YamanakaPlugin extends Plugin {
	settings: YamanakaPluginSettings;
    apiClient: ApiClient;
    syncManager: SyncManager;
    settingsTab: YamanakaSettingTab;
    statusBar: HTMLElement;

    private debounceTimer: NodeJS.Timeout | null = null;
    private filesToUpdate: Set<string> = new Set();
    private filesToDelete: Set<string> = new Set();
    private isApplyingServerChange: boolean = false; // Flag to prevent SSE changes from re-triggering push

	async onload() {
		console.log('Loading Yamanaka Sync Plugin...');
		
		await this.loadSettings();
        
        if (!this.settings.deviceId) {
            this.settings.deviceId = uuidv4();
            await this.saveSettings();
        }

        this.apiClient = new ApiClient(this.settings.serverUrl);
        this.syncManager = new SyncManager(this);

		this.settingsTab = new YamanakaSettingTab(this.app, this);
		this.addSettingTab(this.settingsTab);

        this.statusBar = this.addStatusBarItem();
		this.updateStatusBar('Idle');

		this.registerVaultEvents();
        this.connectToEvents();
		this.addPluginCommands();
	}

	addPluginCommands() {
		this.addCommand({
			id: 'yamanaka-manual-push',
			name: 'Yamanaka: Manual Push',
			callback: async () => {
				if (this.syncManager.isSyncing) {
					new Notice("Sync is already in progress.");
					return;
				}
				if (this.filesToUpdate.size === 0 && this.filesToDelete.size === 0) {
					new Notice("No local changes to push.");
					// The old logic to check server for changes first is removed as '/api/check' no longer provides hash comparison.
					// Clients now rely on SSE for server change notifications.
					// A manual pull can be initiated separately if desired.
					try {
						// You could still perform a basic server check if desired:
						await this.syncManager.check(); // This just pings the server now.
						new Notice("Server is reachable. No local changes to push.");
					} catch (e) {
						new Notice(`Server check failed: ${e.message}. No local changes to push.`);
					}
					return;
				}
				new Notice(`Pushing ${this.filesToUpdate.size} updates and ${this.filesToDelete.size} deletions...`);
				await this.syncManager.push(this.filesToUpdate, this.filesToDelete);
				this.filesToUpdate.clear();
				this.filesToDelete.clear();
			}
		});

		this.addCommand({
			id: 'yamanaka-manual-pull',
			name: 'Yamanaka: Manual Pull',
			callback: async () => { // Corrected syntax: async () =>
				if (this.syncManager.isSyncing) {
					new Notice("Sync is already in progress.");
					return;
				}
				new Notice("Starting manual pull...");
				await this.syncManager.pull();
			}
		});
	}

    connectToEvents() {
        if (this.settings.serverUrl && this.settings.autoSync) {
            this.apiClient.connectToEvents(
                this.settings.deviceId,
                (data) => this.handleFileUpdatedEvent(data),
                (data) => this.handleFileDeletedEvent(data),
                (data) => this.handleFullSyncRequiredEvent(data)
            );
        }
    }

    async handleFileUpdatedEvent(data: import('./api/client').FileEventData) {
        if (!data.path || typeof data.content !== 'string') {
            console.error('[Yamanaka] Invalid file_updated event data:', data);
            return;
        }
        console.log(`[Yamanaka] SSE: Received file update for ${data.path}`);
        this.isApplyingServerChange = true;
        try {
            const filePath = normalizePath(data.path); // Ensure path format is correct
            const file = this.app.vault.getAbstractFileByPath(filePath);
            const contentBuffer = Buffer.from(data.content, 'base64');

            // Ensure parent directories exist
            const parentDir = filePath.substring(0, filePath.lastIndexOf('/'));
            if (parentDir && !this.app.vault.getAbstractFileByPath(parentDir)) {
                try {
                    await this.app.vault.createFolder(parentDir);
                    console.log(`[Yamanaka] Created folder: ${parentDir}`);
                } catch (e) {
                    // Ignore if folder already exists (race condition or already checked)
                    if (!e.message?.includes('already exists')) {
                         console.error(`[Yamanaka] Error creating folder ${parentDir}:`, e);
                         throw e; // Rethrow if it's not an "already exists" error
                    }
                }
            }

            if (file instanceof TFile) {
                console.log(`[Yamanaka] Modifying file via SSE: ${filePath}`);
                await this.app.vault.modifyBinary(file, contentBuffer);
            } else {
                if (file) { // It's a folder or something else, delete it first
                    console.warn(`[Yamanaka] Path ${filePath} exists but is not a file. Deleting and recreating.`);
                    await this.app.vault.delete(file, true);
                }
                console.log(`[Yamanaka] Creating file via SSE: ${filePath}`);
                await this.app.vault.createBinary(filePath, contentBuffer);
            }
            new Notice(`Yamanaka: Synced ${data.path} from server.`);
        } catch (error) {
            console.error(`[Yamanaka] Error applying server update for ${data.path}:`, error);
            new Notice(`Yamanaka: Error syncing ${data.path} from server.`);
        } finally {
            this.isApplyingServerChange = false;
        }
    }

    async handleFileDeletedEvent(data: import('./api/client').FileEventData) {
        if (!data.path) {
            console.error('[Yamanaka] Invalid file_deleted event data:', data);
            return;
        }
        console.log(`[Yamanaka] SSE: Received file delete for ${data.path}`);
        this.isApplyingServerChange = true;
        try {
            const filePath = normalizePath(data.path);
            const file = this.app.vault.getAbstractFileByPath(filePath);
            if (file) {
                console.log(`[Yamanaka] Deleting file via SSE: ${filePath}`);
                await this.app.vault.delete(file, true); // true for recursive delete if it's a folder by mistake
                new Notice(`Yamanaka: Deleted ${data.path} as per server change.`);
            } else {
                console.log(`[Yamanaka] File ${filePath} to delete not found locally.`);
            }
        } catch (error) {
            console.error(`[Yamanaka] Error applying server delete for ${data.path}:`, error);
            new Notice(`Yamanaka: Error deleting ${data.path} as per server.`);
        } finally {
            this.isApplyingServerChange = false;
        }
    }

    handleFullSyncRequiredEvent(data: import('./api/client').FullSyncEventData) {
        console.log('[Yamanaka] SSE: Received full_sync_required event:', data.message);
        new Notice(`Yamanaka: Server requires full sync. ${data.message}. Pulling all changes...`);
        this.syncManager.pull(); // This will pull all files
    }

    registerVaultEvents() {
        this.registerEvent(this.app.vault.on('create', (file) => {
            if (this.isApplyingServerChange) return;
            if (!(file instanceof TFile)) return;
            console.log(`[Yamanaka] Local file created: ${file.path}`);
            this.handleFileChange(file.path);
        }));

        this.registerEvent(this.app.vault.on('modify', (file) => {
            if (this.isApplyingServerChange) return;
            if (!(file instanceof TFile)) return;
            console.log(`[Yamanaka] Local file modified: ${file.path}`);
            this.handleFileChange(file.path);
        }));

        this.registerEvent(this.app.vault.on('delete', (file) => {
            if (this.isApplyingServerChange) return;
            console.log(`[Yamanaka] Local file deleted: ${file.path}`);
            this.filesToDelete.add(file.path);
            this.filesToUpdate.delete(file.path); // No need to update if it's deleted
            this.triggerDebouncedPush();
        }));

        this.registerEvent(this.app.vault.on('rename', (file, oldPath) => {
            if (this.isApplyingServerChange) return;
            console.log(`[Yamanaka] Local file renamed: ${oldPath} -> ${file.path}`);
            this.filesToDelete.add(oldPath);
            this.handleFileChange(file.path); // New path is treated as a change/creation
        }));
    }

    handleFileChange(path: string) {
        if (this.isApplyingServerChange) return;
        this.filesToUpdate.add(path);
        this.triggerDebouncedPush();
    }

    triggerDebouncedPush() {
        if (!this.settings.autoSync) return;
        if (this.syncManager.isSyncing) return; // Don't trigger if a sync is already happening

        if (this.debounceTimer) {
            clearTimeout(this.debounceTimer);
        }
        this.updateStatusBar('Changes pending...');
        this.debounceTimer = setTimeout(async () => {
            await this.syncManager.push(this.filesToUpdate, this.filesToDelete);
            this.filesToUpdate.clear();
            this.filesToDelete.clear();
        }, 3000); // 3-second debounce window
    }

    updateStatusBar(text: string) {
        this.statusBar.setText(`Yamanaka: ${text}`);
    }

	onunload() {
        console.log('Unloading Yamanaka Sync Plugin.');
        this.apiClient.disconnectFromEvents();
	}

	async loadSettings() {
		this.settings = Object.assign({}, DEFAULT_SETTINGS, await this.loadData());
	}

	async saveSettings() {
		await this.saveData(this.settings);
	}
}
