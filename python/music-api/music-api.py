import argparse
import json

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
    parser.add_argument('command', choices=['meta', 'playlist'], help="Command to execute")
    parser.add_argument('id', help="ID for the command")
    args = parser.parse_args()
    if args.command == 'meta':
        get_meta(args.id)
    elif args.command == 'playlist':
        get_playlist(args.id)

if __name__ == "__main__":
    main()