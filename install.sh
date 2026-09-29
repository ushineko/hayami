#!/usr/bin/env bash
#
# Installs hayami, its launcher entry and its icon. Idempotent: safe to re-run.
#
# Nothing here touches your settings, your KWin rule, your cached readings, or
# any other program's autostart. It builds from this checkout and copies four
# files into ~/.local.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"
AUTOSTART_DIR="${HOME}/.config/autostart"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

APP_ID="io.ushineko.hayami"

DRY_RUN=0
AUTOSTART=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        --autostart) AUTOSTART=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: install.sh [--dry-run] [--autostart]

  --dry-run     Show what would be installed, change nothing
  --autostart   Also start the panel when you log in

Installs:
  ~/.local/bin/hayami                                      the desktop panel
  ~/.local/bin/hayami-tui                                  the terminal panel
  ~/.local/share/applications/io.ushineko.hayami.desktop   the launcher entry
  ~/.local/share/icons/hicolor/scalable/apps/hayami.svg    its icon

With --autostart, also:
  ~/.config/autostart/io.ushineko.hayami.desktop           starts it at login

Building needs Go, CGO and a C toolchain with the OpenGL and X11 or Wayland
development headers; the message on failure names the packages. The terminal
panel builds without CGO and works on a machine with no display at all.

The titlebar is the compositor's to remove and is not installed here: it is a
KWin rule the program offers from its preferences window, or `hayami window
install`. This script does not touch any other program's autostart either --
replacing something you already run is your decision, and the README says how.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

run() {
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: $*"
    else
        "$@"
    fi
}

echo "Installing hayami from ${REPO_DIR} ..."

if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is not installed. hayami needs Go 1.26 or newer to build." >&2
    echo "       Arch: pacman -S go   Debian/Ubuntu: apt install golang-go" >&2
    exit 1
fi

echo "Building hayami ..."
if [ "$DRY_RUN" -eq 1 ]; then
    echo "  would run: make -C $REPO_DIR build"
elif ! make -C "$REPO_DIR" build; then
    echo
    echo "hayami did not build. The desktop panel is a Fyne window and needs CGO," >&2
    echo "OpenGL and the X11 or Wayland development headers:" >&2
    echo "    Arch:          base-devel libgl libxi libxcursor libxrandr libxinerama" >&2
    echo "    Debian/Ubuntu: build-essential libgl1-mesa-dev xorg-dev" >&2
    echo "    Fedora:        gcc mesa-libGL-devel libXi-devel libXcursor-devel libXrandr-devel libXinerama-devel" >&2
    echo >&2
    echo "The terminal panel needs none of them: CGO_ENABLED=0 go build ./cmd/hayami-tui" >&2
    exit 1
fi

echo "Installing to ${BIN_DIR} ..."
run install -Dm755 "${REPO_DIR}/hayami" "${BIN_DIR}/hayami"
run install -Dm755 "${REPO_DIR}/hayami-tui" "${BIN_DIR}/hayami-tui"
run install -Dm644 "${REPO_DIR}/packaging/${APP_ID}.desktop" "${APP_DIR}/${APP_ID}.desktop"
run install -Dm644 "${REPO_DIR}/packaging/hayami.svg" "${ICON_DIR}/hayami.svg"

if [ "$AUTOSTART" -eq 1 ]; then
    echo "Starting it at login ..."
    run install -Dm644 "${REPO_DIR}/packaging/${APP_ID}.desktop" \
        "${AUTOSTART_DIR}/${APP_ID}.desktop"
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    run update-desktop-database "${APP_DIR}"
fi
# The icon cache is per theme directory and only some desktops need it poked;
# a failure here costs nothing but a stale icon until the next login.
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    if [ "$DRY_RUN" -eq 1 ]; then
        echo "  would run: gtk-update-icon-cache -f -t ${HOME}/.local/share/icons/hicolor"
    else
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
    fi
fi

echo
echo "Done."
case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *) echo "Note: ${BIN_DIR} is not on your PATH." ;;
esac
echo
echo "Next:"
echo
echo "  hayami                # the panel, or \"hayami\" in your application launcher"
echo "  hayami-tui            # the same readings in a terminal"
echo "  hayami window install # ask KWin for no titlebar and always on top"
if [ "$AUTOSTART" -eq 0 ]; then
    echo
    echo "  ./install.sh --autostart    # and start it when you log in"
fi
