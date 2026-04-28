#!/usr/bin/env bash
# Install the Godot 4.6 headless binary for the current OS into a
# stable cache-friendly location and append it to GITHUB_PATH so
# downstream steps just call `godot --headless ...`.
#
# Usage (in a workflow step):
#   .github/scripts/install-godot.sh 4.6.2-stable
#
# The version argument is the godotengine.org release tag without the
# `v` prefix (e.g. `4.6.2-stable`). The script picks the right archive
# per `RUNNER_OS` (Linux / macOS / Windows) and extracts it under
# `$RUNNER_TOOL_CACHE/godot/<version>/`. Cache-mode runs that hit on
# the directory's existence skip the download.
#
# After this script:
#   - `godot` is on PATH and runs the host's headless binary.
#   - `GODOT_BIN` env var holds the absolute path to the binary, in
#     case downstream steps want it directly.
set -euo pipefail

VERSION="${1:?usage: install-godot.sh <version, e.g. 4.6.2-stable>}"
TOOL_CACHE="${RUNNER_TOOL_CACHE:-$HOME/.cache/runner-tool-cache}"
INSTALL_DIR="$TOOL_CACHE/godot/$VERSION"
BASE_URL="https://github.com/godotengine/godot/releases/download/$VERSION"

# Map OS to archive name and binary suffix. Godot ships separate
# headless / standard builds; we want the standard build because the
# editor mode (`--editor --quit-after`) is needed for the one-shot
# project import that generates `.godot/` cache before the headless
# script run.
case "${RUNNER_OS:-$(uname -s)}" in
    Linux*)
        ARCHIVE="Godot_v${VERSION}_linux.x86_64.zip"
        BIN_NAME="Godot_v${VERSION}_linux.x86_64"
        ;;
    macOS*|Darwin*)
        ARCHIVE="Godot_v${VERSION}_macos.universal.zip"
        # The macOS archive is an .app bundle; the binary lives at
        # Contents/MacOS/Godot inside.
        BIN_NAME="Godot.app/Contents/MacOS/Godot"
        ;;
    Windows*|MINGW*|MSYS*|CYGWIN*)
        ARCHIVE="Godot_v${VERSION}_win64.exe.zip"
        BIN_NAME="Godot_v${VERSION}_win64.exe"
        ;;
    *)
        echo "install-godot.sh: unsupported OS: ${RUNNER_OS:-$(uname -s)}" >&2
        exit 1
        ;;
esac

if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "install-godot.sh: downloading $ARCHIVE → $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
    curl --fail --location --silent --show-error \
        -o "$INSTALL_DIR/$ARCHIVE" \
        "$BASE_URL/$ARCHIVE"
    # `unzip` ships on every GitHub-hosted runner.
    unzip -q "$INSTALL_DIR/$ARCHIVE" -d "$INSTALL_DIR"
    rm "$INSTALL_DIR/$ARCHIVE"
fi

GODOT_BIN="$INSTALL_DIR/$BIN_NAME"
if [[ ! -x "$GODOT_BIN" && ! -f "$GODOT_BIN" ]]; then
    echo "install-godot.sh: expected binary at $GODOT_BIN but it is missing" >&2
    ls -la "$INSTALL_DIR" >&2
    exit 1
fi

# Make sure POSIX runners can execute the binary even after a cache
# restore (the cache action sometimes loses the +x bit on macOS).
chmod +x "$GODOT_BIN" 2>/dev/null || true

# Provide a stable `godot` alias on PATH. We can't symlink reliably
# across all three OS runners, so write a tiny wrapper script under a
# directory that does get added to PATH.
WRAPPER_DIR="$INSTALL_DIR/bin"
mkdir -p "$WRAPPER_DIR"
case "${RUNNER_OS:-$(uname -s)}" in
    Windows*|MINGW*|MSYS*|CYGWIN*)
        # Windows runners use bash on PATH for shell: bash steps;
        # writing a .bat plus a bash wrapper covers both cases.
        cat > "$WRAPPER_DIR/godot.bat" <<EOF
@echo off
"$GODOT_BIN" %*
EOF
        cat > "$WRAPPER_DIR/godot" <<EOF
#!/usr/bin/env bash
exec "$GODOT_BIN" "\$@"
EOF
        chmod +x "$WRAPPER_DIR/godot"
        ;;
    *)
        cat > "$WRAPPER_DIR/godot" <<EOF
#!/usr/bin/env bash
exec "$GODOT_BIN" "\$@"
EOF
        chmod +x "$WRAPPER_DIR/godot"
        ;;
esac

echo "$WRAPPER_DIR" >> "$GITHUB_PATH"
echo "GODOT_BIN=$GODOT_BIN" >> "$GITHUB_ENV"
echo "install-godot.sh: ready — godot binary at $GODOT_BIN"
