# EchoDaemon

EchoDaemon is a prototype for organizing audio: a local Go daemon transcodes
with FFmpeg, enriches tags, and writes finished MP3 files. An optional Chrome
extension can forward YouTube Music track IDs to the daemon. It does not
forward cookies, headers, or request bodies. Only use the project with media
you have permission to process. The server uses [youtube-go](https://github.com/gcottom/youtube-go)
for direct public audio, with a combined audio/video fallback. No headless
browser, Node, or browser request replay is needed. Formats that require player
JavaScript, sign-in, or SABR are not supported. Local tests do not establish
live YouTube compatibility.

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

The default server build requires Go 1.24 or newer. ONNX builds also require a C compiler.
From `server/`, run `go build ./...`, `go vet ./...`, and
`go test -race ./...`. With FFmpeg and its MP3 encoder on PATH, the tests
convert a synthesized tone, tag it using AudioMeta v4, check safe file handling,
and independently decode the completed MP3. They also round-trip text and
cover art in generated AAC/M4A and verify that retagging leaves decoded audio
unchanged. The metadata fallback tests do not need real Spotify credentials.

A native server can use `ECHO_CONFIG` to specify a YAML configuration path
and defaults to `127.0.0.1:50999`. Keep that listen address local: the
prototype is not an authenticated public service.

## License

MIT. See [LICENSE](LICENSE).

### Docker inference mode

The default Compose build uses the heuristic genre engine without downloading ONNX
Runtime or any model files. The optional ONNX build does not change the
download or save pipeline.

To opt into native CPU inference, build with ENABLE_ONNX=1. Docker compiles
the server using Go's onnx tag and stages the native libraries from Microsoft's
hash-verified ONNX Runtime 1.31.1 wheels for Linux amd64 and arm64. The Go
binding uses C API 31; ORT_VERSION must match the bundled pin for ONNX builds.
Docker fetches the pinned [musicnn ONNX v1 bundle](https://github.com/gcottom/echo-daemon/releases/tag/musicnn-onnx-v1)
during that opt-in build and checks both the archive SHA-256 and every model's
checksum before copying the models and original ISC notice into the image.
Startup does not need to fetch models. Python and GPU runtime dependencies
are not installed in the final image.
Startup logs identify which genre engine actually initialized. Check those logs
as well as /healthz before switching a live deployment.

The optional ONNX files are conversions of upstream pre-trained models. See
[model provenance, checksums, and the original musicnn ISC notice](models/README.md).
