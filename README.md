# Echo Daemon

A new take on the classic YouTube downloader that relies on intercepting requests in Chrome made to YouTube Music.
Where most YouTube downloaders need an update almost weekly to patch the signature forging logic, Echo Daemon excels at being consistently able to download tracks without requiring constant updates. 

- Downloads High Quality Audio From YouTube Music
- Parses YouTube UMP format responses to produce a WEBM audio file
- Converts WEBM audio to MP3 using FFMPEG
- Queries the YouTube API and Spotify API to get the best metadata for Artist, Title, Album Title, and Album Artwork
- Uses a Python ML to detect the genre of the downloaded audio.
- Enriches downloaded MP3 file with metadata and saves to the filesystem.


## How to use:
- Download Project Source and Unzip
- Retrieve a developer API key (clientID, secret) from Spotify and add it to the settings.yaml
- Install nodejs if not already installed.
- Build the Chrome extension by first navigating to the chrome folder in a terminal and installing webpack by running ``` npm install webpack ``` and then running ``` npm run build ```  
- Install the Chrome extension by activating developer mode in Chrome, then go to the Extensions menu, select "Load Unpacked", navigate to the chrome folder of this project and open the dist folder at chrome/dist then use the "select" button in the dialog to load the extension.
- Install and launch Docker if not already installed.
- If you want to use library deduplication filtering, open the docker-compose.yaml and change the last volume mount to the path of the directory that contains your library, ensure that you do not remove ":/app/music:ro" from the suffix of the volume mount or Docker won't be able to access your files for download deduplication.
- In a terminal run ``` docker compose up ```
- Wait for Docker to build the image (can take a few minutes)
- Server will launch, wait for the "Now serving on port 50999" message
- Navigate to YouTube Music and start streaming, every track that you listen to will be saved to the data folder.
- If you want saved files to be moved to a specific directory, the start.sh script is a good place to start (modify DIR_A and DIR_B to your respective directory paths, remove sync.sh script references if you do not intend to backup library to AWS via the AWS CLI)
