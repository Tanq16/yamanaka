import { App, PluginSettingTab, Setting, Notice } from 'obsidian';
import YamanakaPlugin from '../main';

export class YamanakaSettingTab extends PluginSettingTab {
	plugin: YamanakaPlugin;
    private statusEl: HTMLElement;

	constructor(app: App, plugin: YamanakaPlugin) {
		super(app, plugin);
		this.plugin = plugin;
	}

	display(): void {
		const { containerEl } = this;
		containerEl.empty();

		containerEl.createEl('h2', { text: 'Yamanaka Self-Hosted Sync' });

		new Setting(containerEl)
			.setName('Server URL')
			.setDesc('The address of your self-hosted sync server (e.g., http://192.168.1.10:8080)')
			.addText(text => text
				.setPlaceholder('Enter your server URL')
				.setValue(this.plugin.settings.serverUrl)
				.onChange(async (value) => {
					this.plugin.settings.serverUrl = value;
					await this.plugin.saveSettings();
                    this.plugin.apiClient.updateBaseUrl(value);
                    this.plugin.connectToEvents(); // Attempt to reconnect with new URL
				}));
        
        new Setting(containerEl)
            .setName('Automatic Sync')
            .setDesc('Automatically push and pull changes in the background.')
            .addToggle(toggle => toggle
                .setValue(this.plugin.settings.autoSync)
                .onChange(async (value) => {
                    this.plugin.settings.autoSync = value;
                    await this.plugin.saveSettings();
                    if (value) {
                        this.plugin.connectToEvents();
                    } else {
                        this.plugin.apiClient.disconnectFromEvents();
                    }
                }));

        this.statusEl = containerEl.createEl('p');
        this.updateStatus();

        containerEl.createEl('h3', { text: 'Manual Actions' });

        new Setting(containerEl)
            .setName('Check Server Status')
            .setDesc('Check the connection and see if new changes are available.')
            .addButton(button => button
                .setButtonText('Check Status')
                .onClick(async () => {
                    try {
                        // Server check
                        const res = await this.plugin.apiClient.check(this.plugin.settings.deviceId); // Using apiClient.check directly
                        let statusMessage = "Yamanaka: Server reachable.";
                        if (res.status === 'ok') { // Server check successful
                            // SSE Status Check
                            if (this.plugin.apiClient.isSseConnected()) {
                                statusMessage = "Yamanaka Active. Real-time sync connected.";
                            } else {
                                statusMessage = "Yamanaka Active. Real-time sync DISCONNECTED. Try toggling Auto-Sync or reloading the plugin.";
                            }
                        } else {
                            // This case should ideally be caught by the catch block if apiClient.check throws an error for non-ok server responses.
                            // If res.status can be other than 'ok' without throwing, this handles it.
                            statusMessage = `Yamanaka: Server check returned status: ${res.status}.`;
                        }
                        new Notice(statusMessage);
                    } catch (error) {
                        console.error("[Yamanaka] Check Server Status error:", error);
                        new Notice(`Yamanaka: Failed to connect to server. ${error.message}`);
                    }
                    this.updateStatus(); // Update any persistent status display if needed
                }));

        new Setting(containerEl)
            .setName('Force Pull')
            .setDesc('DANGER: Overwrites local changes with the version from the server.')
            .addButton(button => button
                .setButtonText('Pull From Server')
                // .setWarning() // Removed to use custom styling
                .setClass('yamanaka-custom-warning-yellow')
                .onClick(async () => {
                    if (confirm('Are you sure? This will overwrite any local files that differ from the server.')) {
                        await this.plugin.syncManager.pull(false); // false for isAutoSync to show notices
                        this.updateStatus();
                    }
                }));

        new Setting(containerEl)
            .setName('Force Push')
            .setDesc('Manually pushes all detected local changes to the server.')
            .addButton(button => button
                .setButtonText('Push to Server')
                .onClick(async () => {
                    // Assuming manual push should show notifications.
                    // Gather current files to push, similar to how triggerDebouncedPush would before calling syncManager.push
                    // This requires access to filesToUpdate and filesToDelete from the plugin instance.
                    // For simplicity, let's use the existing command which already handles this.
                    // Or, if we want a dedicated button action here:
                    if (this.plugin.filesToUpdate.size === 0 && this.plugin.filesToDelete.size === 0) {
                        new Notice("No local changes to push.");
                        return;
                    }
                    new Notice(`Pushing ${this.plugin.filesToUpdate.size} updates and ${this.plugin.filesToDelete.size} deletions...`);
                    await this.plugin.syncManager.push(this.plugin.filesToUpdate, this.plugin.filesToDelete, false); // false for isAutoSync
                    this.plugin.filesToUpdate.clear(); // Clear after push
                    this.plugin.filesToDelete.clear(); // Clear after push
                    this.updateStatus();
                }));

        new Setting(containerEl)
            .setName('Initial Sync')
            .setDesc('DANGER: Wipes the server and replaces its content with your current vault.')
            .addButton(button => button
                .setButtonText('Perform Initial Sync')
                // .setWarning() // Removed to use custom styling
                .setClass('yamanaka-custom-warning-yellow')
                .onClick(async () => {
                     if (confirm('DANGER! Are you sure you want to wipe the server\'s vault and replace it with this one? This should only be done once from your main device.')) {
                        await this.plugin.syncManager.initialSync(); // initialSync doesn't have isAutoSync, its notices are always shown.
                        this.updateStatus();
                    }
                }));
	}

    public updateStatus(): void {
        if (this.statusEl) {
            // const hash = this.plugin.settings.lastSyncHash; // Removed
            this.statusEl.setText(`Device ID: ${this.plugin.settings.deviceId}`); // Removed Last Sync Hash
        }
    }
}
