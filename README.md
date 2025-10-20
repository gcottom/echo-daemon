✨ Echo Daemon  

A new take on the YouTube Music downloader — intelligent, resilient, and built for research.  

Echo Daemon rethinks the classic YouTube downloader.  
Instead of depending on brittle signature-patching logic, it intercepts YouTube Music network requests directly in Chrome, giving it long-term reliability and minimal maintenance.  

⚠️ Disclaimer: This project is a proof of concept, intended for educational and research purposes only.  
You are responsible for complying with all applicable terms of service and copyright laws.  


🚀 Features  
	•	High-Quality Audio Capture from YouTube Music  
	•	Parses YouTube UMP (Unified Media Player) format responses to extract clean .webm audio  
	•	Converts WEBM → MP3 automatically via FFmpeg  
	•	Metadata Enrichment: Queries the YouTube API and Spotify API to fetch artist, title, album, and artwork  
	•	Genre Classification: Uses a Python ML model to infer track genre  
	•	Automatic Tagging: Embeds metadata into the final MP3 and saves it to disk  
	•	Passive or On-Demand Modes: Works both while streaming or through manual track requests  


🧩 Architecture Overview  
Chrome Extension  ->  Echo Daemon Backend (Go)  ->  FFmpeg  ->  YouTube API For Base/Fallback Metadata (Python) -> Spotify API For Complete Metadata (Go)  ->  ML Genre Identifier (Python) -> ID3 Tagger  ->  Enriched MP3 Audio


⚙️ Setup & Usage  
	1.	Clone the repo  
  ```git clone https://github.com/gcottom/echo-daemon.git```  
  ```cd echo-daemon```  
  2.	Retrieve Spotify API credentials (clientID, clientSecret) and add them to settings.yaml.  
  3.	Install Node.js (if not already installed).  
  4.	Build the Chrome extension:  
  ```cd chrome```  
  ```npm install webpack```  
  ```npm run build```  
  5.	Load the extension in Chrome → Extensions > Developer Mode > Load Unpacked → select chrome/dist/.  
  6.	Install and launch Docker.  
  7.	Update paths in settings.yaml for your environment.  
  8.	Run the backend:  
  ```./start.sh```  
  Wait for the message:  
  ```echo-daemon ready!```  
  9.	Stream from YouTube Music — tracks you play are saved to data/ and then moved to local_music_dir.  
  10.   To download a track or playlist on demand, start the echo-daemon server in a docker container, find the video ID in the URL, and make a GET request to the backend:  
    ```http://localhost:50999/download/VIDEO_ID_HERE``` or ```http://localhost:50999/download/PLAYLIST_ID_HERE``` 
You can do this in any browser or using curl or Postman. 

📄 Environment Variables (settings.yaml)  
  •	save_dir: Directory that the Docker container can write save files to (has to be local to the container)  
  •	temp_dir: Directory that in progress downloads are written to (has to be local to the container)  
  •	music_dir: Linked Directory that allows the container to write to the directory specified in local_music_dir (do not change this)  
  •	local_music_dir: Directory that you want completed downloads to go to (absolute path)  
  •	local_music_root: The root of your music library (this is used to prevent duplicates appearing in your library, absolute path)  
  •	local_data_dir: The absolute path of the data directory in the root of this project  
  •	spotify_client_id: Your Spotify Client ID, retrieved from Spotify Developer Portal  
  •	spotify_client_secret: Your Spotify Client Secret, retrieved from the Spotify Developer Portal  

  
🧠 Notes  
  •	Keeps consistent performance without frequent patches.  
  •	All request interception is local; no third-party proxies involved.  
  •	Fully containerized for portability.  

🪪 License  
MIT License - see LICENSE for details.   
