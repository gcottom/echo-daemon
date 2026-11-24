# EchoDaemon

EchoDaemon is a prototype for organizing audio: a local Go daemon transcodes
with FFmpeg, enriches tags, and writes finished MP3 files. An optional Chrome
extension can forward YouTube Music track IDs to the daemon. It does not
forward cookies, headers, or request bodies. Only use the project with media
you have permission to process. The upstream playback interface may change;
local pipeline tests do not establish current streaming-service compatibility.

## Start the daemon

1. Install Docker with Compose.
2. Run `docker compose up --build` from the repository root. The Compose
   defaults bind the API to **127.0.0.1:50999** and write completed MP3s to
   `data/`. The application creates its output and temporary directories.
3. Check startup with `curl -fsS http://127.0.0.1:50999/healthz`.
4. Optionally set Spotify credentials in your local `settings.yaml` for
   enrichment. Do not commit credentials. If Spotify is unset or unavailable,
   already obtained source metadata is retained.

Set the host environment variable `MUSIC_ROOT` to an existing music library
if you want duplicate detection against an existing library. Compose mounts it
read-only; the default is `./music`. Completed output remains in `data/`.
On Bash hosts, `./start.sh` wraps Compose; optionally set `LOCAL_MUSIC_DIR`
to an existing destination if you want the wrapper to move new MP3s into
your library. That optional mode requires `rsync` and does not overwrite
existing files. The output filesystem must support hard links so incomplete
writes stay out of the completed directory.

## Build the extension

From `chrome/`, run `npm ci`, `npm run build`, and `npm test`.
In Chrome Extensions, use **Load unpacked** and select `chrome/dist/`.
Reload an existing installation after updating; this build requests access
only to the player page and the local daemon.

## Verify locally

The server requires Go 1.24 or newer and a C compiler for its ONNX binding.
From `server/`, run `go build ./...`, `go vet ./...`, and
`go test -race ./...`. With FFmpeg and its MP3 encoder on PATH, the tests
convert a synthesized tone, tag it using AudioMeta, check safe file handling,
and independently decode the completed MP3. The metadata fallback tests do
not need real Spotify credentials.

A native server can use `ECHO_CONFIG` to specify a YAML configuration path
and defaults to `127.0.0.1:50999`. Keep that listen address local: the
prototype is not an authenticated public service.

## License

MIT. See [LICENSE](LICENSE).

### Docker inference mode

The default Compose build uses the heuristic genre engine when the optional
ONNX Runtime shared library is absent. Docker still builds the Go ONNX
wrapper with CGO enabled. This avoids a release-asset download in the default
build and does not change the capture or save pipeline.

To opt into native inference, set ENABLE_ONNX=1 before building. ORT_VERSION
overrides the default 1.22.0; use an official ONNX Runtime release with a
published Linux archive for your architecture and compatible C API. Missing
opt-in artifacts fail the build rather than silently changing modes.
