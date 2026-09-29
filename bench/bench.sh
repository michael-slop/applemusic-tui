#!/bin/bash
# Usage: bench.sh <amtui-binary> <label> [seconds-per-phase]
# Runs amtui in a detached tmux (160x45), plays a fixed album muted, and
# measures CPU (clock ticks, 100 = one core-second) for amtui and its Chrome,
# first while playing, then while paused. Appends a line to results.tsv.
set -euo pipefail
BIN=$1; LABEL=$2; SECS=${3:-60}
HERE=$(cd "$(dirname "$0")" && pwd)
DRIVE="uv run -q --with websockets python $HERE/drive.py"
PROFILE="$HOME/.config/amtui/chrome"
export AMTUI_CHROME=${AMTUI_CHROME:-/usr/bin/google-chrome-stable}

cleanup() { tmux kill-session -t amtuibench 2>/dev/null || true
  sleep 2; pkill -f "user-data-dir=$PROFILE" 2>/dev/null || true; }
trap cleanup EXIT
pgrep -f "^(\S*/)?amtui" >/dev/null && { echo "amtui already running; stop it first" >&2; exit 1; }
rm -f "$PROFILE/DevToolsActivePort"

tmux new-session -d -s amtuibench -x 160 -y 45 "env AMTUI_CHROME=$AMTUI_CHROME $BIN"
for i in $(seq 60); do [ -s "$PROFILE/DevToolsActivePort" ] && break; sleep 1; done
$DRIVE wait-authed 90 >/dev/null
# The run is muted; MusicKit persists volume in the shared profile, so put the
# user's volume back afterwards or their next real session starts at 0%.
USER_VOL=$($DRIVE eval "MusicKit.getInstance().volume")
restore_volume() { $DRIVE eval "(()=>{MusicKit.getInstance().volume=${USER_VOL:-1};return true})()" >/dev/null 2>&1 || true; }
$DRIVE play-album "Random Access Memories" >/dev/null
sleep 20   # let playback, artwork, lyrics settle

ticks() { local s=0; for p in "$@"; do [ -r /proc/$p/stat ] && s=$((s + $(awk '{print $14+$15}' /proc/$p/stat))); done; echo $s; }
measure() { # prints "amtui chrome"
  local A=$(tmux list-panes -t amtuibench -F "#{pane_pid}") C=$(pgrep -f "user-data-dir=$PROFILE" | tr '\n' ' ')
  local a0=$(ticks $A) c0=$(ticks $C); sleep $SECS
  local a1=$(ticks $A) c1=$(ticks $C)
  echo "$((a1-a0)) $((c1-c0))"
}
rss() { ps -o rss= -p $(tmux list-panes -t amtuibench -F "#{pane_pid}") | awk '{printf "%d", $1/1024}'; }

read pa pc < <(measure); ra=$(rss)
state_play=$($DRIVE state)
$DRIVE pause >/dev/null; sleep 5
read qa qc < <(measure)
norm() { awk -v t=$1 -v s=$SECS 'BEGIN{printf "%.1f", t/s}'; }  # % of one core
printf "%s\t%s\tplay_amtui=%s%%\tplay_chrome=%s%%\tpause_amtui=%s%%\tpause_chrome=%s%%\tamtui_rss=%sMB\t%s\n" \
  "$(date +%H:%M)" "$LABEL" "$(norm $pa)" "$(norm $pc)" "$(norm $qa)" "$(norm $qc)" "$ra" "$state_play" | tee -a "$HERE/results.tsv"
restore_volume
tmux send-keys -t amtuibench q; sleep 3
