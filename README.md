# Echo Daemon

A new take on the classic YouTube downloader that relies on intercepting requests in Chrome made to YouTube Music.
Where most YouTube downloaders need an update almost weekly to patch the signature forging logic, Echo Daemon excels at being consistently able to download tracks without requiring constant updates. 

⚠️ This project is a proof of concept and for educational/research purposes only.  

## Features
- Downloads High Quality Audio From YouTube Music
- Parses YouTube UMP format responses to produce a WEBM audio file
- Converts WEBM audio to MP3 using FFMPEG
- Queries the YouTube API and Spotify API to get the best metadata for Artist, Title, Album Title, and Album Artwork
- Uses a Python ML to detect the genre of the downloaded audio.
- Enriches downloaded MP3 file with metadata and saves to the filesystem.


## How to use
- Clone this repo using ```git clone https://github.com/gcottom/echo-daemon.git``` then ```cd echo-daemon```
- Retrieve a developer API key (clientID, clientSecret) from Spotify and add it to the settings.yaml
- Install nodejs if not already installed.
- Build the Chrome extension by first navigating to the chrome folder in a terminal and installing webpack by running ``` npm install webpack ``` and then running ``` npm run build ```  
- Install the Chrome extension by activating developer mode in Chrome, then go to the Extensions menu, select "Load Unpacked", navigate to the chrome folder of this project and open the dist folder at chrome/dist then use the "select" button in the dialog to load the extension.
- Install and launch Docker if not already installed.
- All settings are in the settings.yaml, update the local dirs for your system
- Run the command ```./start.sh``` to launch the backend, 
- Wait for Docker to build the image (can take a few minutes)
- Server will launch, wait for the "Now serving on port 50999" message
- Navigate to YouTube Music and start streaming, every track that you listen to will be saved to the data folder.
- The start script will automatically move the files you've downloaded from the Docker mounted data folder to the local_music_dir that you specify in settings.yaml
- It will only attempt to download one song at a time to avoid receiving a ban.
- ETA for downloads is shown in the logs.
- Note that downloading the audio/ump data will take approximately half of the total length of the song in seconds. 3 minute song ~90 seconds, 12 minute song ~ 6 minutes to download. Avoids unthrottling connections to prevent receiving a ban. 
