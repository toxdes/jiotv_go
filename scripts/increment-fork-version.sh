#!/usr/bin/env bash

suffix_file="$(dirname "$0")/fork-suffix"

if [[ ! -f "$suffix_file" ]]; then
    echo "Error: $suffix_file not found"
    exit 1
fi

suffix=$(cat "$suffix_file")
echo "Current suffix: $suffix"

# Parse prefix and number (e.g., "-toxdes.1" -> prefix="-toxdes.", number=1)
if [[ "$suffix" =~ ^(.+\.)([0-9]+)$ ]]; then
    prefix="${BASH_REMATCH[1]}"
    number="${BASH_REMATCH[2]}"
    new_number=$((number + 1))
    new_suffix="${prefix}${new_number}"
    echo "$new_suffix" > "$suffix_file"
    echo "New suffix: $new_suffix"
else
    echo "Error: suffix '$suffix' does not match expected pattern (e.g., -toxdes.1)"
    exit 1
fi
