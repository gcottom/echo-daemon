#!/usr/bin/env bash
set -euo pipefail

# === edit these ===
SRC="/Users/gagecottom/Music/Music/Media.localized/Music"
BUCKET="s3://gages-mac-music-archive"
# storage classes: STANDARD | STANDARD_IA | INTELLIGENT_TIERING | GLACIER_IR | DEEP_ARCHIVE (for sync use STANDARD/IA/INTELLIGENT_TIERING)
STORAGE_CLASS="INTELLIGENT_TIERING"
LOG="${HOME}/music-s3-backup.log"
mkdir -p "$(dirname "$LOG")"

ts() { date '+%Y-%m-%d %H:%M:%S'; }

echo "$(ts) === music-s3-backup start ===" | tee -a "$LOG"

# Base args for reliability & cost control
ARGS=(
  "--storage-class" "$STORAGE_CLASS"
  "--sse" "AES256"                    # server-side encryption by S3 (or swap to KMS if you prefer)
  "--exclude" ".DS_Store"
  "--exclude" "._*"
  "--exclude" ".Spotlight-*"
  "--exclude" ".Trashes"
  "--exclude" ".rsync-partial*"
  "--exclude" "Thumbs.db"
)

echo "$(ts) MIRROR=1 (s3 will match local; deletes enabled)" | tee -a "$LOG"
aws s3 sync "$SRC" "$BUCKET" "${ARGS[@]}" --delete | tee -a "$LOG"


# quick report
echo "$(ts) Done." | tee -a "$LOG"
echo "$(ts) === music-s3-backup end ===" | tee -a "$LOG"