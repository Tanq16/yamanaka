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
					// Optionally, trigger a check here
					const status = await this.syncManager.check();
					if (status.status === "new_changes") {
						new Notice("Server has new changes. Consider pulling first.");
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
            this.apiClient.connectToEvents(this.settings.deviceId, (latestHash) => {
                if (latestHash !== this.settings.lastSyncHash) {
                    new Notice('New changes detected on server. Pulling...');
                    this.syncManager.pull();
                }
            });
        }
    }

    registerVaultEvents() {
        this.registerEvent(this.app.vault.on('create', (file) => {
            if (!(file instanceof TFile)) return;
            console.log(`[Yamanaka] File created: ${file.path}`);
            this.handleFileChange(file.path);
        }));

        this.registerEvent(this.app.vault.on('modify', (file) => {
            if (!(file instanceof TFile)) return;
            console.log(`[Yamanaka] File modified: ${file.path}`);
            this.handleFileChange(file.path);
        }));

        this.registerEvent(this.app.vault.on('delete', (file) => {
            console.log(`[Yamanaka] File deleted: ${file.path}`);
            this.filesToDelete.add(file.path);
            this.filesToUpdate.delete(file.path); // No need to update if it's deleted
            this.triggerDebouncedPush();
        }));

        this.registerEvent(this.app.vault.on('rename', (file, oldPath) => {
            console.log(`[Yamanaka] File renamed: ${oldPath} -> ${file.path}`);
            this.filesToDelete.add(oldPath);
            this.handleFileChange(file.path);
        }));
    }

    handleFileChange(path: string) {
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
