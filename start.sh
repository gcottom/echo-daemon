#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

command -v docker >/dev/null 2>&1 || { echo 'Docker with Compose is required.' >&2; exit 1; }
docker compose version >/dev/null
export MUSIC_ROOT="${MUSIC_ROOT:-./music}"
mkdir -p -- data temp "$MUSIC_ROOT"

# With no destination, leave completed files in data/.
if [[ -z "${LOCAL_MUSIC_DIR:-}" ]]; then
    exec docker compose up --build --abort-on-container-exit
fi

command -v rsync >/dev/null 2>&1 || { echo 'Install rsync to use LOCAL_MUSIC_DIR.' >&2; exit 1; }
[[ -d "$LOCAL_MUSIC_DIR" ]] || { echo 'LOCAL_MUSIC_DIR must be an existing directory.' >&2; exit 1; }
destination="$(cd -- "$LOCAL_MUSIC_DIR" && pwd -P)"
source_dir="$(cd data && pwd -P)"
case "$destination/" in "$source_dir/"*) echo 'Destination must be outside data/.' >&2; exit 1;; esac
( docker compose up --build --abort-on-container-exit ) &
compose_pid=$!
cleanup() {
    trap - EXIT
    docker compose stop >&2 || true
    wait "$compose_pid" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

while kill -0 "$compose_pid" 2>/dev/null; do
    # Completed MP3s are published atomically. Never replace an existing
    # library file and never move a staging file.
    rsync -a --ignore-existing --remove-source-files --include='*/' --include='*.mp3' --exclude='*' data/ "$destination/"
    sleep 15
done
wait "$compose_pid"
