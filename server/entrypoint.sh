#!/bin/sh
set -e

# Add /app/data as a safe directory for git
git config --global --add safe.directory /app/data
# Set git user configuration globally
git config --global user.name "yamanaka"
git config --global user.email "yamanaka@obsidian.sync"

# Initialize the vault if it's not already a git repository
if [ ! -d "/app/data/.git" ]; then
    echo "No .git directory found in /app/data. Initializing new Git repository."
    mkdir -p /app/data
    cd /app/data
    git init
    # Set repository-specific user details as a fallback/override
    git config user.name "yamanaka"
    git config user.email "yamanaka@obsidian.sync"
    cd /app
else
    echo ".git directory found in /app/data."
    if [ -d "/app/data/.git" ]; then
        cd /app/data
        git config user.name "yamanaka"
        git config user.email "yamanaka@obsidian.sync"
        cd /app
    fi
fi

# Execute the main application passed as CMD
exec "$@"
