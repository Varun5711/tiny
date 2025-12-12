#!/bin/bash
set -e

echo "==============================================="
echo "Setting up PostgreSQL Primary for Replication"
echo "==============================================="

echo "Step 1: Creating replication user and slots..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Create replication user with REPLICATION privilege
    CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'replicator_password';

    -- Create physical replication slots (one for each replica)
    SELECT pg_create_physical_replication_slot('replication_slot_1');
    SELECT pg_create_physical_replication_slot('replication_slot_2');
    SELECT pg_create_physical_replication_slot('replication_slot_3');

    -- Grant necessary permissions
    GRANT CONNECT ON DATABASE urlshortener TO replicator;
EOSQL

echo "Step 2: Configuring pg_hba.conf for replication connections..."

# Allow replication connections from Docker network (172.0.0.0/8 covers most Docker networks)
cat >> "$PGDATA/pg_hba.conf" <<-EOF

# Replication connections for streaming WAL
host    replication     replicator      all                     md5
host    all             all             0.0.0.0/0              md5
EOF

echo "Step 3: Reloading PostgreSQL configuration..."
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -c "SELECT pg_reload_conf();"

echo "==============================================="
echo "Replication setup completed successfully!"
echo "Primary is ready to accept replica connections"
echo "==============================================="
