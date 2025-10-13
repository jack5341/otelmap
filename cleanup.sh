#!/bin/bash

# Cleanup script for OTEL Map Server
# Deletes ALL traces and session tokens every 6 hours

set -e

CLICKHOUSE_HOST=${CLICKHOUSE_HOST:-clickhouse}
CLICKHOUSE_PORT=${CLICKHOUSE_PORT:-9000}
CLICKHOUSE_USER=${CLICKHOUSE_USER:-default}
CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-default}
CLICKHOUSE_DATABASE=${CLICKHOUSE_DATABASE:-otel}

echo "Cleaning up ALL data from tables"

# Connect to ClickHouse and run cleanup queries
clickhouse-client \
  --host="$CLICKHOUSE_HOST" \
  --port="$CLICKHOUSE_PORT" \
  --user="$CLICKHOUSE_USER" \
  --password="$CLICKHOUSE_PASSWORD" \
  --database="$CLICKHOUSE_DATABASE" \
  --query="
    -- Delete ALL traces
    DELETE FROM otel_traces;
    
    -- Delete ALL session tokens
    DELETE FROM session_tokens;
  "

echo "Cleanup completed successfully - all data deleted"
