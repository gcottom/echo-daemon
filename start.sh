#!/bin/bash
export LANG="en_US.UTF-8"
export LC_ALL="en_US.UTF-8"
export MUSIC_ROOT=$(grep '^local_music_root:' settings.yaml | cut -d ':' -f2- | xargs)

# Logging helper early so we can log during dependency checks
log(){ printf '%s %s\n' "$(date '+%F %T')" "$*" | tee -a "${HOME}/echo-daemon.log"; }

# Ensure rsync present; install only if missing
if ! command -v rsync >/dev/null 2>&1; then
  if command -v brew >/dev/null 2>&1; then
    log "rsync not found; installing via Homebrew..."
    if ! brew install rsync; then
      log "ERROR: Failed to install rsync"; exit 1;
    fi
  else
    log "ERROR: rsync not found and Homebrew is not installed. Please install rsync."; exit 1;
  fi
else
  log "rsync already installed: $(rsync --version | head -n1)"
fi

# Define directories
DIR_A=$(grep '^local_data_dir:' settings.yaml | cut -d ':' -f2- | xargs)
echo "DIR_A is $DIR_A"
DIR_B=$(grep '^local_music_dir:' settings.yaml | cut -d ':' -f2- | xargs)
echo "DIR_B is $DIR_B"
LOG="${HOME}/echo-daemon.log"
INTERVAL=15   # seconds between scans
mkdir -p "$DIR_A"

cleanup(){
  log "Shutting down"
  # Attempt to gracefully stop compose stack
  docker compose down -v >/dev/null 2>&1
  exit 0
}
trap cleanup INT TERM

[ -d "$DIR_A" ] || { log "ERROR: Source missing: $DIR_A"; exit 1; }
[ -d "$DIR_B" ] || { log "ERROR: Dest missing:   $DIR_B"; exit 1; }

log "Starting echo-daemon helper script: moving files from $DIR_A -> $DIR_B every ${INTERVAL}s"
# Start compose stack in background; --abort-on-container-exit ensures if any container exits the whole stack stops
( docker compose up --abort-on-container-exit ) &
COMPOSE_PID=$!
log "docker compose up started with PID $COMPOSE_PID"

# Give containers a brief moment to start
sleep 3

while true; do
    # If compose process died, exit the helper script
    if ! kill -0 "$COMPOSE_PID" 2>/dev/null; then
        log "docker compose process ($COMPOSE_PID) is no longer running; exiting helper script"
        exit 1
    fi

    # Optional: detect any exited container early (in case compose hasn't torn down yet)
    if docker compose ps --format '{{.Service}} {{.State}}' 2>/dev/null | grep -E '\b(exit|exited|dead)\b' >/dev/null; then
        log "Detected a container in exited/dead state; stopping"
        docker compose down -v >/dev/null 2>&1
        exit 1
    fi

    find "$DIR_A" -type f -print0 | while IFS= read -r -d '' file; do
        if [ ! -e "$file" ]; then continue; fi
        rel_path="${file#$DIR_A/}"
        dest_file="$DIR_B/$rel_path"
        dest_dir="$(dirname "$dest_file")"
        mkdir -p "$dest_dir"
        if rsync -a --remove-source-files "$file" "$dest_file"; then
            log "Moved: $file -> $dest_file"
        else
            log "Failed to move: $file"
        fi
    done
done