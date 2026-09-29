#!/usr/bin/env bash
#
# Removes exactly what install.sh puts in place: the two programs, a launcher
# entry, an icon and the autostart entry if there is one. Everything hayami has
# written for you stays, and this prints where it is. Idempotent.

set -euo pipefail

BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/.local/share/applications"
ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"
AUTOSTART_DIR="${HOME}/.config/autostart"

APP_ID="io.ushineko.hayami"

DRY_RUN=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: uninstall.sh [--dry-run]

  --dry-run   List what would be removed, change nothing

Removes only what install.sh placed. Your settings and your cached readings
are left alone and their locations are printed, so you can remove them by hand
if you want to.

The KWin rule is not removed here, because this script did not install it:
`hayami window remove` gives the titlebar back, and the rule is visible in
System Settings under Window Rules either way. Run it before uninstalling, or
the rule stays behind matching a window that no longer exists -- harmless, and
untidy.
USAGE
            exit 0
            ;;
        *) echo "Unknown option: $arg" >&2; exit 1 ;;
    esac
done

echo "Removing hayami ..."

removed=0
for f in "${BIN_DIR}/hayami" \
         "${BIN_DIR}/hayami-tui" \
         "${APP_DIR}/${APP_ID}.desktop" \
         "${ICON_DIR}/hayami.svg" \
         "${AUTOSTART_DIR}/${APP_ID}.desktop"; do
    if [ -e "$f" ]; then
        removed=$((removed + 1))
        if [ "$DRY_RUN" -eq 1 ]; then
            echo "  would remove $f"
        else
            echo "  removing $f"
            rm -f "$f"
        fi
    fi
done
if [ "$removed" -eq 0 ]; then
    echo "  nothing to remove; install.sh has not run, or has already been undone"
fi

if [ "$DRY_RUN" -eq 0 ]; then
    command -v update-desktop-database >/dev/null 2>&1 && \
        update-desktop-database "${APP_DIR}" >/dev/null 2>&1 || true
    command -v gtk-update-icon-cache >/dev/null 2>&1 && \
        gtk-update-icon-cache -f -t "${HOME}/.local/share/icons/hicolor" >/dev/null 2>&1 || true
fi

echo
echo "Left alone:"
echo "  ${XDG_CONFIG_HOME:-${HOME}/.config}/hayami/settings.yaml   your settings"
echo "  ${XDG_CACHE_HOME:-${HOME}/.cache}/hayami/sections.json     the last readings"
echo
echo "The KWin rule, if you installed it, is removed with: hayami window remove"
echo "(run that before removing the binary, or take it out in System Settings)"
