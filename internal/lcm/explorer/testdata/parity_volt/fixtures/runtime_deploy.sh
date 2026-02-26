#!/bin/bash

# Shell script fixture for parity testing
# Demonstrates complex shell patterns, functions, and control structures

set -euo pipefail

# Constants
readonly PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
readonly LOG_LEVEL="${LOG_LEVEL:-INFO}"
readonly MAX_RETRIES="${MAX_RETRIES:-3}"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to retry a command
retry_command() {
    local command="$1"
    local max_attempts=$2
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if eval "$command"; then
            log_info "Command succeeded on attempt $attempt"
            return 0
        fi
        log_warn "Command failed (attempt $attempt/$max_attempts)"
        attempt=$((attempt + 1))
        sleep 1
    done

    log_error "Command failed after $max_attempts attempts"
    return 1
}

# Function to deploy application
deploy() {
    local environment="${1:-staging}"
    local service_name="${2:-app}"

    log_info "Deploying $service_name to $environment"

    # Export environment variables
    export ENVIRONMENT="$environment"
    export SERVICE_NAME="$service_name"

    # Run deployment steps
    if [ "$environment" = "production" ]; then
        log_info "Running production deployment"
        retry_command "kubectl apply -f k8s/" "$MAX_RETRIES"
    else
        log_info "Running staging deployment"
        docker-compose up -d
    fi

    log_info "Deployment completed successfully"
}

# Function to perform health checks
health_check() {
    local endpoint="$1"
    local timeout="${2:-30}"

    log_info "Performing health check on $endpoint"

    if command_exists curl; then
        if curl -f -s -o /dev/null -m "$timeout" "$endpoint"; then
            log_info "Health check passed"
            return 0
        fi
    fi

    log_error "Health check failed"
    return 1
}

# Main execution
main() {
    local command="${1:-deploy}"
    shift || true

    case "$command" in
        deploy)
            deploy "$@"
            ;;
        health)
            health_check "$@"
            ;;
        *)
            log_error "Unknown command: $command"
            echo "Usage: $0 {deploy|health} [options]"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"