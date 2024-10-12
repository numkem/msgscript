#!/usr/bin/env sh

# Check if arguments are provided
if [ "$#" -gt 0 ]; then
    /app/cli "$@"
else
    /app/server -plugin=/app
fi
