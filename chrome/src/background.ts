const daemonEndpoint = "http://127.0.0.1:50999";
const videoIDPattern = /^[A-Za-z0-9_-]{11}$/;

// Forward only a track identifier to the local daemon. Do not forward cookies,
// headers or request bodies and do not require a persistent worker.
chrome.webRequest.onBeforeRequest.addListener(
    (details: chrome.webRequest.WebRequestBodyDetails) => {
        const parts = details.requestBody?.raw;
        if (!parts || parts.length !== 1 || !parts[0].bytes) return;

        let id: unknown;
        try {
            const body = JSON.parse(new TextDecoder().decode(parts[0].bytes));
            id = body?.videoId;
        } catch {
            return;
        }
        if (typeof id !== "string" || !videoIDPattern.test(id)) return;

        void fetch(`${daemonEndpoint}/download/${id}`).then(response => {
            if (!response.ok) console.warn("EchoDaemon request failed:", response.status);
        }).catch(() => console.warn("EchoDaemon is not reachable on 127.0.0.1:50999."));
    },
    {urls: ["https://music.youtube.com/youtubei/v1/player*"]},
    ["requestBody"]
);
