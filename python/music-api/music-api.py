import argparse
import json
import gettext as _gettext

# Ensure gettext returns a fallback instead of raising if translations are missing (e.g., in PyInstaller)
# We set a safe wrapper that always enables fallback and never raises when translation catalogs are absent.
_real_gettext_translation = _gettext.translation

def _safe_translation(domain, localedir=None, languages=None, *args, **kwargs):
    kwargs.setdefault('fallback', True)
    try:
        return _real_gettext_translation(domain, localedir=localedir, languages=languages, *args, **kwargs)
    except Exception:
        return _gettext.NullTranslations()

_gettext.translation = _safe_translation

from ytmusicapi import YTMusic

ytmusic = YTMusic()

def get_meta(id):
    data = ytmusic.get_song(id)
    response = {
        'title': data['videoDetails']['title'],
        'author': data['videoDetails']['author'],
        'image': data['videoDetails']['thumbnail']['thumbnails'][-1]['url'],
    }
    print(json.dumps(response))

def get_playlist(id):
    tracks = ytmusic.get_playlist(id)
    vid = []
    print(tracks)
    for t in tracks["tracks"]:
        vid.append(t["videoId"])
    response = {'tracks': vid}
    print(json.dumps(response))

def main():
    parser = argparse.ArgumentParser(description="Music API Command Line Tool")
    # Note on argument parsing and '--':
    # Callers may insert "--" before an ID that starts with '-' to terminate option parsing.
    # argparse treats "--" as the standard end-of-options marker and does NOT include it
    # in any parsed values. With only positional args defined here, this ensures IDs that
    # begin with '-' are still parsed into args.id correctly.
    parser.add_argument('command', choices=['meta', 'playlist'], help="Command to execute")
    parser.add_argument('id', help="ID for the command")
    args = parser.parse_args()
    if args.command == 'meta':
        get_meta(args.id)
    elif args.command == 'playlist':
        get_playlist(args.id)

if __name__ == "__main__":
    main()