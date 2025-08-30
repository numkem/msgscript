#!/bin/sh
set -e

# Function to display usage information
usage() {
    echo "Usage: $0 [cli|server] [arguments...]"
    echo ""
    echo "Commands:"
    echo "  cli     - Run the CLI application at /app/bin/cli"
    echo "  server  - Run the server application at /app/bin/server"
    echo ""
    echo "Examples:"
    echo "  $0 cli --help"
    echo "  $0 server --port 8080"
    echo "  $0 server"
    exit 1
}

# Check if at least one argument is provided
if [ $# -eq 0 ]; then
    echo "Error: No command specified."
    usage
fi

# Get the command (first argument)
COMMAND="$1"
shift  # Remove the command from arguments list

# Execute based on the command
case "$COMMAND" in
    "cli")
        if [ $# -gt 0 ] && ([ "$1" = "dev" ] || [ "$1" = "devhttp" ]); then
            CLI_COMMAND="$1"
            shift
            exec /app/bin/cli "$CLI_COMMAND" -plugin /app/share/plugins "$@"
        else
            exec /app/bin/cli
        fi
        ;;
    "server")
        exec /app/bin/server -plugin /app/share/plugins "$@"
        ;;
    "--help"|"-h"|"help")
        usage
        ;;
    *)
        echo "Error: Unknown command '$COMMAND'"
        usage
        ;;
esac
