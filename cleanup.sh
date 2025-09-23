#!/bin/bash
# delete-underscore.sh
# Usage: ./delete-underscore.sh /path/to/dir

TARGET="${1:-.}"

find "$TARGET" -type f -name '._*' -print -delete