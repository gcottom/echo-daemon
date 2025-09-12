import os
import argparse
from musicnn.tagger import top_tags
import json

os.environ['LIBROSA_CACHE_DIR'] = '/tmp/librosa_cache'
os.environ['NUMBA_CACHE_DIR'] = '/tmp/numba_cache'

def process_track(file_path):
    tops = 5
    t = top_tags(file_path, model='MSD_musicnn', topN=tops, print_tags=False)
    v = top_tags(file_path, model='MSD_vgg', topN=tops, print_tags=False)
    x = top_tags(file_path, model='MTT_musicnn', topN=tops, print_tags=False)
    y = top_tags(file_path, model='MTT_vgg', topN=tops, print_tags=False)
    z = {}
    w = tops
    for l in t:
        z[l] = w
        w=w-1
    w=tops
    for l in v:
        if l in z: 
            z[l] = z[l] +w
        else:
            z[l] = w
        w=w-1
    w=tops
    for l in x:
        if l in z: 
            z[l] = z[l] +w
        else:
            z[l] = w
        w=w-1
    w=tops
    for l in y:
        if l in z: 
            z[l] = z[l] +w
        else:
            z[l] = w
        w=w-1
    s = sorted(z.items(), key=lambda x:x[1], reverse=True)
    z = dict(s)
    g = ['classical', 'techno', 'strings', 'drums', 'electronic', 'rock', 'piano', 'ambient', 'violin', 'vocal', 'synth', 'indian', 'opera', 'harpsichord', 'flute', 'pop', 'sitar', 'classic', 'choir', 'new age', 'dance', 'harp', 'cello', 'country', 'metal', 'choral', 'alternative', 'indie', '00s', 'alternative rock', 'jazz', 'chillout', 'classic rock', 'soul', 'indie rock', 'Mellow', 'electronica', '80s', 'folk', '90s', 'chill', 'instrumental', 'punk', 'oldies', 'blues', 'hard rock', 'acoustic', 'experimental', 'Hip-Hop', '70s', 'party', 'easy listening', 'funk', 'electro', 'heavy metal', 'Progressive rock', '60s', 'rnb', 'indie pop', 'sad', 'House']
    for k in z:
        if k in g:
            z = k
            break
    if z == dict(s):
        z = list(z.keys())[0]

    print(json.dumps({"genre": z.title()}))

def main():
    parser = argparse.ArgumentParser(description='Process a music track and send genre to an API.')
    parser.add_argument('file_path', type=str, help='The path of the track to process')
    args = parser.parse_args()
    
    process_track(args.file_path)

if __name__ == "__main__":
    main()