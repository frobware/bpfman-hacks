#!/usr/bin/env bash

# Default values.
author=red-hat-konflux
dry_run=false
repos=()

# Function to execute autoprat commands with optional dry-run.
# Auto-injects author and repo; caller provides additional flags.
autoprat() {
    local cmd_output
    cmd_output=$(command autoprat -a "$author" -r "$current_repo" "$@")
    echo "$cmd_output"

    # Only pipe to shell if this generates commands (has -P flag).
    if [[ "$*" == *"-P"* ]] && [ "$dry_run" = false ]; then
        echo "$cmd_output" | sh
    fi
}

# Parse command line arguments.
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -n|--dry-run)
                dry_run=true
                shift
                ;;
            -a|--author)
                author="$2"
                shift 2
                ;;
            -*)
                echo "Unknown option: $1"
                echo "Usage: $0 [-n|--dry-run] [-a|--author AUTHOR] REPO [REPO ...]"
                exit 1
                ;;
            *)
                repos+=("$1")
                shift
                ;;
        esac
    done

    # Require at least one repo.
    if [ ${#repos[@]} -eq 0 ]; then
        echo "Error: At least one repository must be specified"
        echo "Usage: $0 [-n|--dry-run] [-a|--author AUTHOR] REPO [REPO ...]"
        exit 1
    fi
}

# Main execution function.
main() {
    parse_args "$@"

    # Check if process_repo function is defined.
    if ! declare -f process_repo >/dev/null; then
        echo "Error: process_repo() function must be defined"
        exit 1
    fi

    for repo in "${repos[@]}"; do
        current_repo="$repo"
        process_repo "$repo"
    done
}
