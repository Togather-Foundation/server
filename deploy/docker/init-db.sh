#!/bin/bash
# PostgreSQL initialization script for Togather Server
# Enables required extensions: PostGIS, pgvector, pg_trgm
# Reference: specs/001-deployment-infrastructure/spec.md FR-003

set -e

# This script runs during container initialization via docker-entrypoint-initdb.d
# It executes with the postgres superuser before the database accepts connections

echo "Initializing Togather database with required extensions..."

# Connect to the target database and enable extensions
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Enable PostGIS extension for geospatial data support
    -- Used for place coordinates and geographic queries
    CREATE EXTENSION IF NOT EXISTS postgis;
    
    -- Enable pgvector extension for vector embeddings
    -- Used for semantic search and similarity matching
    CREATE EXTENSION IF NOT EXISTS vector;
    
    -- Enable pg_trgm extension for trigram text search
    -- Used for fuzzy text matching and search optimization
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
    
    -- Enable pg_stat_statements for query performance monitoring
    -- Useful for identifying slow queries and optimization opportunities
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
    
    -- Verify extensions are enabled
    SELECT extname, extversion 
    FROM pg_extension 
    WHERE extname IN ('postgis', 'vector', 'pg_trgm', 'pg_stat_statements');
    
    -- Log success
    DO \$\$
    BEGIN
        RAISE NOTICE 'Database initialization complete. Extensions enabled: postgis, vector, pg_trgm, pg_stat_statements';
    END \$\$;
EOSQL

echo "Database initialization complete!"
