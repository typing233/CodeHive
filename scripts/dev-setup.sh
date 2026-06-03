#!/bin/bash
set -e

echo "=== CodeHive Development Setup ==="

# Check for PostgreSQL
if ! command -v psql &> /dev/null; then
    echo "PostgreSQL client not found. Please install PostgreSQL."
    exit 1
fi

# Create database and user
echo "Creating database..."
sudo -u postgres psql -c "CREATE USER codehive WITH PASSWORD 'codehive';" 2>/dev/null || true
sudo -u postgres psql -c "CREATE DATABASE codehive OWNER codehive;" 2>/dev/null || true
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE codehive TO codehive;" 2>/dev/null || true

# Copy config
if [ ! -f codehive.yaml ]; then
    cp codehive.example.yaml codehive.yaml
    echo "Created codehive.yaml from example"
fi

# Create data directory
mkdir -p data/repos

echo ""
echo "Setup complete! Run with:"
echo "  make dev"
echo ""
echo "Servers will start on:"
echo "  HTTP: http://localhost:3000"
echo "  SSH:  ssh://localhost:2222"
