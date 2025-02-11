#!/usr/bin/env bash

set -euo pipefail

DB_FILE=${1:-bpf.db}
SQL_FILE=$(mktemp)

# Enable foreign key constraints upfront
sqlite3 "$DB_FILE" <<'EOF'
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS BPFProgram (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    path TEXT UNIQUE,
    type TEXT NOT NULL,
    version TEXT,
    pinning TEXT CHECK (pinning IN ('pinned', 'unpinned')),
    run_time_ns INTEGER DEFAULT 0,
    run_cnt INTEGER DEFAULT 0,
    loaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS BPFMap (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    program_id INTEGER,  -- Nullable for standalone maps
    name TEXT UNIQUE NOT NULL,
    path TEXT UNIQUE,
    type TEXT NOT NULL,
    key_size INTEGER NOT NULL CHECK (key_size > 0),
    value_size INTEGER NOT NULL CHECK (value_size > 0),
    max_entries INTEGER NOT NULL CHECK (max_entries > 0),
    bytes_used INTEGER DEFAULT 0,
    bytes_limit INTEGER DEFAULT 0,
    pinning TEXT CHECK (pinning IN ('pinned', 'unpinned')),
    FOREIGN KEY (program_id) REFERENCES BPFProgram(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS BPFLink (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    program_id INTEGER NOT NULL,  -- Links always belong to a program
    path TEXT UNIQUE,
    event TEXT NOT NULL,
    attach_type TEXT,
    link_id INTEGER,
    pinning TEXT CHECK (pinning IN ('pinned', 'unpinned')),
    attached_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (program_id) REFERENCES BPFProgram(id) ON DELETE CASCADE
);
EOF

# Function to fetch bpftool JSON safely and store in a temporary file
fetch_bpftool_json() {
    local cmd=$1
    local tmp_file
    tmp_file=$(mktemp)

    # Run bpftool and save to a file
    sudo $cmd --json > "$tmp_file" 2>&1
    local status=$?

    # Debugging output
    echo "Running: $cmd" >&2

    if [[ $status -ne 0 ]]; then
        echo "Error running bpftool: $cmd" >&2
        cat "$tmp_file" >&2
        rm -f "$tmp_file"
        return 1
    fi

    echo "$tmp_file"  # Return filename
}

# Function to build SQL statements
build_bpf_sql() {
    # Fetch JSON data safely
    prog_json_file=$(fetch_bpftool_json "bpftool prog show") || exit 1
    map_json_file=$(fetch_bpftool_json "bpftool map show") || exit 1
    link_json_file=$(fetch_bpftool_json "bpftool link show") || exit 1

    echo "BEGIN TRANSACTION;" > "$SQL_FILE"

    # Insert Programs
    jq -c '.[]' "$prog_json_file" | while read -r entry; do
        id=$(echo "$entry" | jq '.id')
        name=$(echo "$entry" | jq -r '.name')
        path=$(echo "$entry" | jq -r '.pinned // empty')
        type=$(echo "$entry" | jq -r '.type')
        run_time_ns=$(echo "$entry" | jq '.run_time_ns // 0')
        run_cnt=$(echo "$entry" | jq '.run_cnt // 0')
        pinning=$( [[ -n "$path" ]] && echo "pinned" || echo "unpinned" )

        echo "INSERT OR REPLACE INTO BPFProgram (id, name, path, type, pinning, run_time_ns, run_cnt)
              VALUES ($id, '$name', '$path', '$type', '$pinning', $run_time_ns, $run_cnt);" >> "$SQL_FILE"
    done

    # Insert Maps
    jq -c '.[]' "$map_json_file" | while read -r entry; do
        name=$(echo "$entry" | jq -r '.name')
        path=$(echo "$entry" | jq -r '.pinned // empty')
        type=$(echo "$entry" | jq -r '.type')
        key_size=$(echo "$entry" | jq '.key_size // .bytes_key // -1')
        value_size=$(echo "$entry" | jq '.value_size // .bytes_value // -1')
        max_entries=$(echo "$entry" | jq '.max_entries // -1')
        bytes_used=$(echo "$entry" | jq '.bytes_used // 0')
        bytes_limit=$(echo "$entry" | jq '.bytes_limit // 0')
        pinning=$( [[ -n "$path" ]] && echo "pinned" || echo "unpinned" )

        map_type=$(echo "$entry" | jq -r '.type')

        # Ensure valid key_size, value_size, max_entries
        if [[ $key_size -le 0 || $value_size -le 0 || $max_entries -le 0 ]]; then
            echo "-- Skipping invalid map: $name (type: $map_type, key_size: $key_size, value_size: $value_size, max_entries: $max_entries)"
            continue
        fi

        program_id=$(sqlite3 "$DB_FILE" "SELECT id FROM BPFProgram WHERE path = '$path' OR name = '$name' LIMIT 1;")
        [[ -z "$program_id" ]] && program_id="NULL"

        echo "INSERT OR REPLACE INTO BPFMap (program_id, name, path, type, key_size, value_size, max_entries, bytes_used, bytes_limit, pinning)
              VALUES ($program_id, '$name', '$path', '$type', $key_size, $value_size, $max_entries, $bytes_used, $bytes_limit, '$pinning');" >> "$SQL_FILE"
    done

    # Insert Links
    jq -c '.[]' "$link_json_file" | while read -r entry; do
        id=$(echo "$entry" | jq '.id')
        path=$(echo "$entry" | jq -r '.pinned // empty')
        event=$(echo "$entry" | jq -r '.target')
        attach_type=$(echo "$entry" | jq -r '.attach_type')
        link_id=$(echo "$entry" | jq '.id')

        program_id=$(sqlite3 "$DB_FILE" "SELECT id FROM BPFProgram WHERE path = '$path' OR name = '$name' LIMIT 1;")
        if [[ -z "$program_id" ]]; then
            echo "-- Skipping link $id (event: $event, path: $path): No associated program found."
            continue
        fi

        echo "INSERT OR REPLACE INTO BPFLink (program_id, path, event, attach_type, link_id, pinning)
              VALUES ($program_id, '$path', '$event', '$attach_type', $link_id, 'pinned');" >> "$SQL_FILE"
    done
}

echo "BEGIN TRANSACTION;" > "$SQL_FILE"
build_bpf_sql
echo "COMMIT;" >> "$SQL_FILE"

# Execute the transaction
sqlite3 "$DB_FILE" < "$SQL_FILE"

echo "BPF database populated successfully."
rm -f "$SQL_FILE"
