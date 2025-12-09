set -e

echo "Creating replication user and slots..."

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Create replication user
    CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'replicator_password';
    
    -- Create replication slots for each replica
    SELECT pg_create_physical_replication_slot('replication_slot_1');
    SELECT pg_create_physical_replication_slot('replication_slot_2');
    SELECT pg_create_physical_replication_slot('replication_slot_3');
    
    -- Grant necessary permissions
    GRANT CONNECT ON DATABASE urlshortener TO replicator;
EOSQL

echo "host replication replicator all md5" >> "$PGDATA/pg_hba.conf"

echo "Replication setup completed!"
