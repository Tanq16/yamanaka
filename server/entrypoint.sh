#!/bin/sh
set -e

# Add /app/data as a safe directory for git
git config --global --add safe.directory /app/data

# Set git user configuration globally
# These will be used by the git commands run by the server
git config --global user.name "yamanaka"
git config --global user.email "yamanaka@obsidian.sync"

# Initialize the vault if it's not already a git repository
# The InitRepo function in the Go application also does this,
# but running it here ensures the context and ownership are correct
# before the application starts.
if [ ! -d "/app/data/.git" ]; then
    echo "No .git directory found in /app/data. Initializing new Git repository."
    # Ensure the data directory exists
    mkdir -p /app/data
    # Change to the data directory to run git init
    cd /app/data
    git init
    # Set repository-specific user details as a fallback/override
    git config user.name "yamanaka"
    git config user.email "yamanaka@obsidian.sync"
    # Change back to the app directory
    cd /app
else
    echo ".git directory found in /app/data."
    # Ensure repository-specific user details are set if the repo already exists
    # This helps if the global config was set after repo initialization somehow
    # or if different settings are desired per-repo (though here they are the same).
    # We need to be careful not to error out if /app/data is empty and not a git repo yet.
    if [ -d "/app/data/.git" ]; then
        cd /app/data
        git config user.name "yamanaka"
        git config user.email "yamanaka@obsidian.sync"
        cd /app
    fi
fi


# Execute the main application passed as CMD
exec "$@"
