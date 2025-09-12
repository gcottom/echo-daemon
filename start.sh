#!/bin/bash
export LANG="en_US.UTF-8"
export LC_ALL="en_US.UTF-8"

brew install rsync
# Define directories
DIR_A="/Volumes/990pro/projects/echo-daemon/data"
DIR_B="/users/gagecottom/music/music/media.localized/automatically add to music.localized"
LOG="${HOME}/echo-daemon.log"
INTERVAL=15   # seconds between scans
BACKUP_INTERVAL=$((60 * 15))
LAST_BACKUP=0
mkdir -p "$DIR_A"
log(){ printf '%s %s\n' "$(date '+%F %T')" "$*" | tee -a "$LOG"; }
trap 'log "Shutting down"; exit 0' INT TERM

[ -d "$DIR_A" ] || { log "ERROR: Source missing: $DIR_A"; exit 1; }
[ -d "$DIR_B" ] || { log "ERROR: Dest missing:   $DIR_B"; exit 1; }

log "Starting echo-daemon helper script: moving files from $DIR_A -> $DIR_B every ${INTERVAL}s"
docker compose build --no-cache
(docker compose up)&
while true; do
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
    now=$(date +%s)
    if (( now - LAST_BACKUP >= BACKUP_INTERVAL )); then
        log "Performing backup..."
        /Volumes/990pro/projects/echo-daemon/sync.sh >> "$LOG" 2>&1
        LAST_BACKUP=$now
    fi
    sleep "$INTERVAL"
done