#!/bin/bash
# ============================================================================
# Tiny CLI Installer
# ============================================================================
# Usage:
#   curl -sSL https://raw.githubusercontent.com/Varun5711/shorternit/main/scripts/install.sh | bash
#
# Or with a specific version:
#   curl -sSL ... | bash -s -- v1.0.0
# ============================================================================

set -e

REPO="Varun5711/shorternit"
BINARY_NAME="tiny-cli"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux*)   OS="linux" ;;
        darwin*)  OS="darwin" ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *)
            log_error "Unsupported OS: $OS"
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)
            log_error "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    log_info "Detected platform: ${OS}-${ARCH}"
}

# Get the latest version from GitHub
get_latest_version() {
    if [ -n "$1" ]; then
        VERSION="$1"
        log_info "Using specified version: $VERSION"
    else
        log_info "Fetching latest version..."
        VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
        if [ -z "$VERSION" ]; then
            log_error "Failed to fetch latest version"
            exit 1
        fi
        log_info "Latest version: $VERSION"
    fi
}

# Download and install the binary
install_binary() {
    EXT=""
    if [ "$OS" = "windows" ]; then
        EXT=".exe"
    fi

    FILENAME="${BINARY_NAME}-${OS}-${ARCH}${EXT}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    log_info "Downloading ${FILENAME}..."

    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT

    if ! curl -sL -o "${TMP_DIR}/${BINARY_NAME}${EXT}" "$DOWNLOAD_URL"; then
        log_error "Failed to download from ${DOWNLOAD_URL}"
        exit 1
    fi

    # Verify download
    if [ ! -s "${TMP_DIR}/${BINARY_NAME}${EXT}" ]; then
        log_error "Downloaded file is empty"
        exit 1
    fi

    # Make executable
    chmod +x "${TMP_DIR}/${BINARY_NAME}${EXT}"

    # Install
    log_info "Installing to ${INSTALL_DIR}/${BINARY_NAME}${EXT}..."

    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMP_DIR}/${BINARY_NAME}${EXT}" "${INSTALL_DIR}/"
    else
        log_warn "Need sudo to install to ${INSTALL_DIR}"
        sudo mv "${TMP_DIR}/${BINARY_NAME}${EXT}" "${INSTALL_DIR}/"
    fi

    log_info "Installation complete!"
}

# Verify installation
verify_install() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        log_info "Verifying installation..."
        $BINARY_NAME --version
        echo ""
        log_info "${GREEN}Successfully installed!${NC}"
        echo ""
        echo "Usage:"
        echo "  1. Create a .env file with your server addresses:"
        echo "     URL_SERVICE_ADDR=your-server:50051"
        echo "     USER_SERVICE_ADDR=your-server:50052"
        echo ""
        echo "  2. Or use command line flags:"
        echo "     tiny-cli --url-service your-server:50051 --user-service your-server:50052"
        echo ""
        echo "  3. Run the TUI:"
        echo "     tiny-cli"
    else
        log_warn "Binary installed but not in PATH"
        log_info "Add ${INSTALL_DIR} to your PATH or run: ${INSTALL_DIR}/${BINARY_NAME}"
    fi
}

main() {
    echo ""
    echo "  ╔════════════════════════════════════════╗"
    echo "  ║       Tiny CLI Installer               ║"
    echo "  ╚════════════════════════════════════════╝"
    echo ""

    detect_platform
    get_latest_version "$1"
    install_binary
    verify_install
}

main "$@"
