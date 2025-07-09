import { Notice, TFile, TAbstractFile, normalizePath } from 'obsidian';
import YamanakaPlugin from '../main';
import { ApiClient } from '../api/client';
import Tar from 'tar-js'; // Changed import style
import * as pako from 'pako';

export class SyncManager {
    plugin: YamanakaPlugin;
    apiClient: ApiClient;
    isSyncing: boolean = false;

    constructor(plugin: YamanakaPlugin) {
        this.plugin = plugin;
        this.apiClient = plugin.apiClient;
    }

    private async setSyncing(state: boolean, statusMessage: string) {
        if (state) {
            if (this.isSyncing) {
                console.warn("[Yamanaka] Sync already in progress. Ignoring new request.");
                return false;
            }
            this.isSyncing = true;
        } else {
            this.isSyncing = false;
        }
        this.plugin.updateStatusBar(statusMessage);
        return true;
    }

    async check() {
        try {
            return await this.apiClient.check(
                this.plugin.settings.deviceId,
                this.plugin.settings.lastSyncHash
            );
        } catch (err) {
            new Notice(err.message);
            console.error(err);
            return { status: "error" };
        }
    }

    async pull() {
        if (!await this.setSyncing(true, 'Syncing: Pulling from server...')) return;

        try {
            const response = await this.apiClient.pull(this.plugin.settings.deviceId);
            const serverFiles = new Map(response.files.map(f => [f.path, f]));
            const localFiles = this.plugin.app.vault.getFiles();

            // Delete local files that are not on the server
            for (const localFile of localFiles) {
                if (!serverFiles.has(localFile.path)) {
                    console.log(`[Yamanaka] Deleting local file: ${localFile.path}`);
                    await this.plugin.app.vault.delete(localFile, true);
                }
            }

            // Create/update files from server
            for (const [path, serverFile] of serverFiles.entries()) {
                const localFile = this.plugin.app.vault.getAbstractFileByPath(path);
                const content = Buffer.from(serverFile.content, 'base64');

                if (localFile instanceof TFile) {
                    const localContent = await this.plugin.app.vault.readBinary(localFile);
                    if (Buffer.compare(new Uint8Array(localContent), new Uint8Array(content)) !== 0) {
                        console.log(`[Yamanaka] Updating file: ${path}`);
                        await this.plugin.app.vault.modifyBinary(localFile, content);
                    }
                } else {
                    console.log(`[Yamanaka] Creating new file: ${path}`);
                    await this.plugin.app.vault.createBinary(path, content);
                }
            }
            
            this.plugin.settings.lastSyncHash = response.hash;
            await this.plugin.saveSettings();
            new Notice("Pull complete!");

        } catch (err) {
            new Notice(`Pull failed: ${err.message}`);
            console.error(err);
        } finally {
            await this.setSyncing(false, 'Idle');
        }
    }

    async push(filesToUpdate: Set<string>, filesToDelete: Set<string>) {
        if (!await this.setSyncing(true, `Syncing: Pushing ${filesToUpdate.size + filesToDelete.size} changes...`)) return;

        try {
            const updatePayload = [];
            for (const path of filesToUpdate) {
                const file = this.plugin.app.vault.getAbstractFileByPath(path);
                if (file instanceof TFile) {
                    const content = await this.plugin.app.vault.readBinary(file);
                    updatePayload.push({ path, content: Buffer.from(content).toString('base64') });
                }
            }

            const response = await this.apiClient.push(
                this.plugin.settings.deviceId,
                updatePayload,
                Array.from(filesToDelete)
            );

            this.plugin.settings.lastSyncHash = response.new_hash;
            await this.plugin.saveSettings();
            new Notice("Push successful!");

        } catch (err) {
            new Notice(`Push failed: ${err.message}`);
            console.error(err);
        } finally {
            await this.setSyncing(false, 'Idle');
        }
    }

    async initialSync() {
        if (!await this.setSyncing(true, 'Syncing: Performing initial sync...')) return;

        try {
            const files = this.plugin.app.vault.getFiles();
            const tape = new Tar();
            let fileCount = 0;

            for (const file of files) {
                const content = await this.plugin.app.vault.readBinary(file);
                tape.append(file.path, new Uint8Array(content));
                fileCount++;
            }

            const tarball = tape.out;
            const compressed = pako.gzip(tarball);
            const blob = new Blob([compressed], { type: 'application/gzip' });

            new Notice(`Archived ${fileCount} files. Uploading to server...`);

            const response = await this.apiClient.initialSync(this.plugin.settings.deviceId, blob);
            this.plugin.settings.lastSyncHash = response.new_hash;
            await this.plugin.saveSettings();
            new Notice("Initial Sync successful!");

        } catch (err) {
            new Notice(`Initial Sync failed: ${err.message}`);
            console.error(err);
        } finally {
            await this.setSyncing(false, 'Idle');
        }
    }
}
