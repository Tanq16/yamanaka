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
            // lastSyncHash is no longer used by the backend's /api/check endpoint
            return await this.apiClient.check(this.plugin.settings.deviceId);
        } catch (err) {
            new Notice(err.message);
            console.error(err);
            return { status: "error" };
        }
    }

    async pull(isAutoSync?: boolean) {
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
                    // Ensure parent directories exist before creating the file
                    const parentDir = path.substring(0, path.lastIndexOf('/'));
                    if (parentDir && !this.plugin.app.vault.getAbstractFileByPath(parentDir)) {
                        try {
                            console.log(`[Yamanaka] Creating folder during pull: ${parentDir}`);
                            await this.plugin.app.vault.createFolder(parentDir);
                        } catch (e) {
                            // Ignore if folder already exists (race condition or already checked by another process)
                            if (!e.message?.includes('already exists')) {
                                console.error(`[Yamanaka] Error creating folder ${parentDir} during pull:`, e);
                                // Optionally rethrow or handle more gracefully depending on desired behavior
                                // For now, we log the error and attempt to create the file anyway,
                                // as createBinary might still succeed or provide a more specific error.
                            }
                        }
                    }
                    console.log(`[Yamanaka] Creating new file: ${path}`);
                    if (localFile) { // It's a folder or something else, delete it first
                        console.warn(`[Yamanaka] Path ${path} exists but is not a TFile. Deleting and recreating.`);
                        await this.plugin.app.vault.delete(localFile, true);
                    }
                    await this.plugin.app.vault.createBinary(path, content);
                }
            }
            
            // this.plugin.settings.lastSyncHash = response.hash; // Hash is removed from PullResponse
            // await this.plugin.saveSettings(); // No settings change needed here anymore regarding hash
            if (!isAutoSync) {
                new Notice("Pull complete!");
            }

        } catch (err) {
            if (!isAutoSync) {
                new Notice(`Pull failed: ${err.message}`);
            }
            console.error(err);
        } finally {
            await this.setSyncing(false, 'Idle');
        }
    }

    async push(filesToUpdate: Set<string>, filesToDelete: Set<string>, isAutoSync?: boolean) {
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

            // this.plugin.settings.lastSyncHash = response.new_hash; // new_hash is removed from SuccessResponse for push
            // await this.plugin.saveSettings(); // No settings change needed here anymore regarding hash
            if (!isAutoSync) {
                new Notice(`Push successful! Server response: ${response.status}`);
            }

        } catch (err) {
            if (!isAutoSync) {
                new Notice(`Push failed: ${err.message}`);
            }
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
            // this.plugin.settings.lastSyncHash = response.new_hash; // new_hash is removed from SuccessResponse for initialSync
            // await this.plugin.saveSettings(); // No settings change needed here anymore regarding hash
            new Notice(`Initial Sync successful! Server response: ${response.status}`);

        } catch (err) {
            new Notice(`Initial Sync failed: ${err.message}`);
            console.error(err);
        } finally {
            await this.setSyncing(false, 'Idle');
        }
    }
}
