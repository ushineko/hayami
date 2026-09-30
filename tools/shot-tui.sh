#!/usr/bin/env bash
# Photograph a terminal program as a person sees it.
#
# Piping a pane through `script` and stripping the escape codes answers what
# the text is, which is a different question from what it looks like -- and for
# anything about colour it is the wrong question entirely. This starts a real
# terminal of a known size, runs the program in it, and grabs the window.
#
#   shot-tui.sh <out.png> <cols> <rows> <command...>
set -euo pipefail
OUT="$1"; COLS="$2"; ROWS="$3"; shift 3

CLASS="hayami-shot"
alacritty --class "$CLASS,$CLASS" -o "window.dimensions.columns=$COLS" \
    -o "window.dimensions.lines=$ROWS" -o "window.padding.x=6" -o "window.padding.y=6" \
    -e "$@" >/dev/null 2>&1 &
APP=$!
trap 'kill $APP 2>/dev/null || true' EXIT

wid=""
for _ in $(seq 40); do
    for w in $(kdotool search --class "$CLASS" 2>/dev/null || true); do
        wid=$w
    done
    [ -n "$wid" ] && break
    sleep 0.25
done
[ -n "$wid" ] || { echo "the terminal never appeared" >&2; exit 1; }

sleep "${SETTLE:-6}"
T=$(mktemp --suffix=.png)
spectacle -f -b -n -o "$T" >/dev/null 2>&1 || true
sleep 2
G=$(kdotool getwindowgeometry "$wid")
python3 "$(dirname "${BASH_SOURCE[0]}")/crop.py" "$T" "$OUT" \
    "$(printf '%s' "$G" | awk '/Position/{print $2}')" \
    "$(printf '%s' "$G" | awk '/Geometry/{print $2}')"
rm -f "$T"
python3 -c "from PIL import Image; im=Image.open('$OUT'); print('$OUT', im.size)"
