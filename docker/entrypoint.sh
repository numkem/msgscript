#!/bin/sh
set -e

usage() {
    echo "Usage: $0 [cli|server] [arguments...]"
    echo ""
    echo "Commands:"
    echo "  cli     - Run the CLI"
    echo "  server  - Run the server"
    echo ""
    echo "Examples:"
    echo "  $0 cli --help"
    echo "  $0 server --port 8080"
    echo "  $0 server"
    exit 1
}

if [ $# -eq 0 ]; then
    echo "Error: No command specified."
    usage
fi

# Get the command (first argument)
COMMAND="$1"
shift

case "$COMMAND" in
    "cli")
        if [ $# -gt 0 ] && ([ "$1" = "dev" ] || [ "$1" = "devhttp" ]); then
            CLI_COMMAND="$1"
            shift
            exec /app/cli "$CLI_COMMAND" -plugin /app/plugins "$@"
        else
            exec /app/cli
        fi
        ;;
    "server")
        exec /app/server -plugin /app/plugins -wexec ./app/worker "$@"
        ;;
    "--help"|"-h"|"help")
        usage
        ;;
    *)
        echo "Error: Unknown command '$COMMAND'"
        usage
        ;;
esac
