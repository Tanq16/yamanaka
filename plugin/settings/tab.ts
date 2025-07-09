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
                    const res = await this.plugin.syncManager.check();
                    if (res.status === 'uptodate') {
                        new Notice('Vault is up-to-date.');
                    } else if (res.status === 'new_changes') {
                        new Notice(`New changes available on server!`);
                    }
                    this.updateStatus();
                }));

        new Setting(containerEl)
            .setName('Force Pull')
            .setDesc('DANGER: Overwrites local changes with the version from the server.')
            .addButton(button => button
                .setButtonText('Pull From Server')
                .setWarning()
                .onClick(async () => {
                    if (confirm('Are you sure? This will overwrite any local files that differ from the server.')) {
                        await this.plugin.syncManager.pull();
                        this.updateStatus();
                    }
                }));

        new Setting(containerEl)
            .setName('Force Push')
            .setDesc('Manually pushes all detected local changes to the server.')
            .addButton(button => button
                .setButtonText('Push to Server')
                .onClick(async () => {
                    new Notice("This action is not yet implemented. Pushing happens automatically on file changes.");
                    this.updateStatus();
                }));

        new Setting(containerEl)
            .setName('Initial Sync')
            .setDesc('DANGER: Wipes the server and replaces its content with your current vault.')
            .addButton(button => button
                .setButtonText('Perform Initial Sync')
                .setWarning()
                .onClick(async () => {
                     if (confirm('DANGER! Are you sure you want to wipe the server\'s vault and replace it with this one? This should only be done once from your main device.')) {
                        await this.plugin.syncManager.initialSync();
                        this.updateStatus();
                    }
                }));
	}

    public updateStatus(): void {
        if (this.statusEl) {
            const hash = this.plugin.settings.lastSyncHash;
            this.statusEl.setText(`Device ID: ${this.plugin.settings.deviceId} | Last Sync Hash: ${hash ? hash.substring(0, 7) : 'none'}`);
        }
    }
}
