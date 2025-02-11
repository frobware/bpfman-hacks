#!/usr/bin/env bash

set -euo pipefail

DB_FILE="bpf.db"
DELETE_DB=false

while getopts "k" opt; do
    case "$opt" in
        k) DELETE_DB=true ;;
        *) echo "Usage: $0 [-k] [db_file]" >&2; exit 1 ;;
    esac
done
shift $((OPTIND - 1))

if [[ $# -gt 0 ]]; then
    DB_FILE="$1"
    shift
fi

# Delete the database if -k was passed.
if [[ "$DELETE_DB" == true ]]; then
    echo "Deleting database: $DB_FILE" >&2
    rm -f "$DB_FILE"
fi

fetch_bpftool_json() {
    local args="$@"
    local tmp_file

    if ! tmp_file=$(mktemp --tmpdir "bpftool-${args// /-}_XXXXXX"); then
        return 1
    fi

    sudo bpftool "$@" --json 2>/dev/null | jq '.' > "$tmp_file"
    local status=$?

    if [[ $status -ne 0 ]]; then
        echo "Error running bpftool: $args" >&2
        rm -f "$tmp_file"
        return 1
    fi

    echo "$tmp_file"  # Return filename
}

sqlite3 "$DB_FILE" < ./schema.sql

link_json_file=$(fetch_bpftool_json link show) || exit 1
map_json_file=$(fetch_bpftool_json map show) || exit 1
prog_json_file=$(fetch_bpftool_json prog show) || exit 1

total_links=$(jq '. | length' "$link_json_file")
total_maps=$(jq '. | length' "$map_json_file")
total_programs=$(jq '. | length' "$prog_json_file")

echo "Total Links:    $total_links"
echo "Total Maps:     $total_maps"
echo "Total Programs: $total_programs"

ls -l "$link_json_file"
ls -l "$map_json_file"
ls -l "$prog_json_file"

cargo run "bpf.db" "$prog_json_file" "$map_json_file" "$link_json_file"
