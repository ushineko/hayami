#!/usr/bin/env bash
# Photograph hayami for the README, on KDE/Wayland.
#
# Ported from fynedesygn's tools/screenshot.sh (same author, MIT), which is
# angou's made generic. Each shot starts a fresh ./hayami on what it wants,
# grabs its window, and kills it, so the whole gallery is one command with
# nothing to click -- the difference between pictures that track the program
# and pictures that quietly go stale.
#
# What is different here:
#
#   1. The user's own panel is almost certainly running, with the same app ID
#      as the one this starts. So the window is found by comparing window ids
#      before and after the launch, never by class alone, and only the PID
#      this script started is killed. The preferences window is a second
#      window of the same process and carries the same class, so it is told
#      from the panel by its title.
#   2. The panel is translucent: its cards are faded and the space between
#      them is not painted at all. A grab of the desktop cropped to the panel
#      photographs whatever is behind it -- the first run of this had a line
#      of somebody's chat through the Bandwidth card. So the window is
#      activated and grabbed alone, which keeps its transparency as alpha and
#      can show nothing of any other window.
#   3. The readings are the desk's own. The settings are a throwaway copy of
#      the user's, so the live arrangement and sections are shown and nothing
#      is written back; the copy drops the remembered position, so the shot is
#      not stacked on top of the user's panel.
#
# The pane shots go through tools/shot-tui.sh, which crops the desktop with
# tools/crop.py, in an alacritty made opaque by a scratch XDG_CONFIG_HOME: a
# translucent terminal photographs whatever is behind it.
#
# Requires kdotool, spectacle, alacritty, and python3 with Pillow.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${HAYAMI_BIN:-$REPO_DIR/hayami}"
TUI="${HAYAMI_TUI:-$REPO_DIR/hayami-tui}"
CLASS="io.ushineko.hayami"
PANEL_TITLE="^hayami\$"
PREFS_TITLE="^hayami preferences"
SETTINGS_SRC="${XDG_CONFIG_HOME:-$HOME/.config}/hayami/settings.yaml"
ME="$(basename "$0")"

usage() {
    cat <<'USAGE'
usage: tools/screenshot.sh [--out DIR] --all
       tools/screenshot.sh [--out DIR] --what panel|prefs:<page>|pane:column|pane:row

  --out DIR     where the images go; default docs/img
  --all         the whole gallery: the panel, the pane at 100 columns in its
                grid and at 160 in rows, and every page of the preferences
                window (the pages are read from ./hayami --help)
  --what SHOT   one image, written to DIR/gallery-<shot>.png with the colon
                as a hyphen (prefs:about -> gallery-prefs-about.png)

  $SETTLE       seconds to wait after the window appears before the grab;
                default 12 for the panel (two cooler polls, and a trend with
                something in it), 4 for a preferences page, 30 for a pane (a
                pane's trend is a character a sample, so it needs the time
                to be more than a dot)

The alt text in README.md describes what is in each image. It is the only
description a screen-reader user gets, and a stale one is worse than none:
check it still matches before committing a new capture.
USAGE
}

die() { printf '%s: %s\n' "$ME" "$*" >&2; exit 1; }

out_dir="$REPO_DIR/docs/img"
what=""
all=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --out) shift; out_dir="${1:-}" ;;
        --what) shift; what="${1:-}" ;;
        --all) all=1 ;;
        -h|--help) usage; exit 0 ;;
        *) printf '%s: unknown argument: %s\n' "$ME" "$1" >&2; usage >&2; exit 2 ;;
    esac
    shift
done
if [[ "$all" -eq 0 && -z "$what" ]]; then
    usage >&2
    exit 2
fi

for tool in kdotool spectacle alacritty python3; do
    command -v "$tool" >/dev/null || die "$tool is not installed"
done
python3 -c "import PIL" 2>/dev/null || die "python3 Pillow is not installed"
[[ -x "$BIN" ]] || die "$BIN not found or not executable (run make build)"
[[ -x "$TUI" ]] || die "$TUI not found or not executable (run make build)"
[[ -f "$SETTINGS_SRC" ]] || die "no settings at $SETTINGS_SRC: there is nothing live to photograph"

SCRATCH=$(mktemp -d)
LAUNCHED=""     # the panel this script started and has not yet stopped

# stop kills the panel this script started, and only that one.
stop() {
    if [[ -n "$LAUNCHED" ]]; then
        kill "$LAUNCHED" 2>/dev/null || true
        wait "$LAUNCHED" 2>/dev/null || true
        LAUNCHED=""
    fi
}
cleanup() { stop; rm -rf -- "$SCRATCH"; }
trap cleanup EXIT INT TERM

# The throwaway settings. The remembered position is dropped so the
# compositor places this panel rather than on top of the user's own, which
# sits exactly there.
SETTINGS="$SCRATCH/settings.yaml"
sed -e '/^  placed:/d' -e '/^  x:/d' -e '/^  "y":/d' -- "$SETTINGS_SRC" >"$SETTINGS"

# An opaque terminal for the pane shots, keeping the user's font and colours
# when there is a config to import them from.
mkdir -p "$SCRATCH/xdg/alacritty"
{
    if [[ -f "$HOME/.config/alacritty/alacritty.toml" ]]; then
        printf '[general]\nimport = ["%s"]\n\n' "$HOME/.config/alacritty/alacritty.toml"
    fi
    printf '[window]\nopacity = 1.0\n'
} >"$SCRATCH/xdg/alacritty/alacritty.toml"

# ids lists hayami's windows, one per line.
ids() {
    timeout 10 kdotool search --class "^${CLASS//./\\.}\$" 2>/dev/null || true
}

# newWindow waits for a hayami window that was not in the list before and
# whose title matches, and prints its id.
newWindow() {
    local title="$1" before="$2" w waited=0
    while [[ "$waited" -lt 80 ]]; do
        for w in $(ids); do
            if ! grep -qxF -- "$w" <<<"$before" &&
                [[ "$(timeout 10 kdotool getwindowname "$w" 2>/dev/null || true)" =~ $title ]]; then
                printf '%s\n' "$w"
                return 0
            fi
        done
        sleep 0.25
        waited=$((waited + 1))
    done
    return 1
}

# grab photographs one window alone, then checks the picture is of it: the
# window's shape, and not one flat colour.
grab() {
    local wid="$1" dest="$2" active="" tries=0 dim
    # Activating is asynchronous, and `spectacle -a` grabs whatever is active
    # the moment it fires: confirm focus first, or the shot is of the
    # terminal that ran this.
    while [[ "$tries" -lt 12 ]]; do
        timeout 10 kdotool windowactivate "$wid" >/dev/null 2>&1 || true
        sleep 0.5
        active=$(timeout 10 kdotool getactivewindow 2>/dev/null || true)
        [[ "$active" == "$wid" ]] && break
        tries=$((tries + 1))
    done
    [[ "$active" == "$wid" ]] || {
        printf '%s: could not focus the window (active=%s want=%s)\n' "$ME" "$active" "$wid" >&2
        return 1
    }
    mkdir -p -- "$(dirname -- "$dest")"
    rm -f -- "$dest"
    # -S drops the compositor's drop shadow, which pads the image unevenly.
    timeout 30 spectacle -a -b -n -S -o "$dest" >/dev/null 2>&1 || true
    sleep 1.5
    [[ -s "$dest" ]] || { printf '%s: spectacle produced nothing\n' "$ME" >&2; return 1; }
    dim=$(timeout 10 kdotool getwindowgeometry "$wid" | awk '/Geometry/{print $2}')
    python3 - "$dest" "$dim" <<'PY'
import os, sys
from PIL import Image, ImageStat

path, dim = sys.argv[1], sys.argv[2]
im = Image.open(path).convert("RGB")
w, h = (float(v) for v in dim.split("x"))
want, got = w / h, im.width / im.height
if abs(want - got) / want > 0.05:
    sys.exit("captured %dx%d, but the window is %gx%g -- wrong window grabbed"
             % (im.width, im.height, w, h))
if max(ImageStat.Stat(im).stddev) < 4:
    sys.exit("%s is one flat colour -- the window had not drawn" % path)
print("  %s  %dx%d  %.0fK" % (os.path.basename(path), im.width, im.height,
                              os.path.getsize(path) / 1024))
PY
}

# window starts ./hayami, photographs the new window it opened with the title
# wanted, and stops it. page is empty for the panel and names a preferences
# page otherwise.
window() {
    local page="$1" dest="$2" title="$PANEL_TITLE" settle="${SETTLE:-12}" before wid rc=0
    if [[ -n "$page" ]]; then
        title="$PREFS_TITLE"
        settle="${SETTLE:-4}"
    fi
    before=$(ids)

    "$BIN" --settings "$SETTINGS" ${page:+"--preferences=$page"} >/dev/null 2>&1 &
    LAUNCHED=$!

    wid=$(newWindow "$title" "$before") || {
        printf '%s: no new window titled %s appeared\n' "$ME" "$title" >&2
        stop
        return 1
    }
    sleep "$settle"
    grab "$wid" "$dest" || rc=1
    stop
    return "$rc"
}

# pane photographs the terminal panel at a width, in an arrangement.
pane() {
    local cols="$1" rows="$2" arrangement="$3" dest="$4"
    mkdir -p -- "$(dirname -- "$dest")"
    XDG_CONFIG_HOME="$SCRATCH/xdg" SETTLE="${SETTLE:-30}" \
        "$REPO_DIR/tools/shot-tui.sh" "$dest" "$cols" "$rows" \
        "$TUI" --settings "$SETTINGS" --arrangement "$arrangement"
}

# shoot takes one --what value.
shoot() {
    local shot="$1" dest
    dest="$out_dir/gallery-${shot/:/-}.png"
    case "$shot" in
        panel) window "" "$dest" ;;
        prefs:?*) window "${shot#prefs:}" "$dest" ;;
        pane:column) pane 100 16 grid "$dest" ;;
        pane:row) pane 160 19 row "$dest" ;;
        *) printf '%s: no shot named %s\n' "$ME" "$shot" >&2; return 2 ;;
    esac
}

if [[ "$all" -eq 0 ]]; then
    shoot "$what"
    exit 0
fi

pages=$("$BIN" --help 2>&1 | sed -n 's/.*on one of: //p' | tr -d ',')
[[ -n "$pages" ]] || die "./hayami --help names no preferences pages"

shots=(panel pane:column pane:row)
for p in $pages; do
    shots+=("prefs:$p")
done
for s in "${shots[@]}"; do
    shoot "$s"
done
echo
echo "Now check the alt text in README.md still describes what is in each image."
