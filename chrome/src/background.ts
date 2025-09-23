const goPort = 50999;
const INTERNAL_TESTALIVE_PORT = "EchoDaemon_Internal_alive_test";
const nextSeconds = 25;
const SECONDS = 1000;
const DEBUG = false;

var alivePort: chrome.runtime.Port | null = null;
var isFirstStart = true;
var isAlreadyAwake = false;
var timer: number;
var firstCall: number;
var lastCall: number;
var wakeup: NodeJS.Timeout | undefined = undefined;
var wsTest = undefined;
var wCounter = 0;

const starter = `-------- >>> ${convertNoDate(Date.now())} UTC - Service Worker for EchoDaemon Keep-Alive is starting <<< --------`;

console.log(starter);

letsStart();

// Helper: should skip forwarding this URL?
function shouldSkipForward(url: string): boolean {
    try {
        const u = new URL(url);
        const itag = u.searchParams.get("itag");
        return itag === "243";
    } catch {
        return false;
    }
}

function letsStart() {
    if (wakeup === undefined) {
        isFirstStart = true;
        isAlreadyAwake = true;
        firstCall = Date.now();
        lastCall = firstCall;
        timer = 300;
        wakeup = setInterval(Highlander, timer);
        console.log(`-------- >>> EchoDaemon has been started at ${convertNoDate(firstCall)}`);
    }
}

chrome.runtime.onInstalled.addListener(
    async () => await initialize()
);

chrome.tabs.onCreated.addListener(onCreatedTabListener);
chrome.tabs.onUpdated.addListener(onUpdatedTabListener);
chrome.tabs.onRemoved.addListener(onRemovedTabListener);
chrome.windows.onRemoved.addListener((windowId) => {
    wCounter--;
    if (wCounter > 0) {
        return;
    }
    if (wakeup !== undefined) {
        isAlreadyAwake = false;
        console.log("Shutting down EchoDaemon");
        clearInterval(wakeup);
        wakeup = undefined;
    }
});

chrome.windows.onCreated.addListener(async (window) => {
    let w = await chrome.windows.getAll();
    wCounter = w.length;
    if (wCounter == 1) {
        updateJobs();
    }
});

interface RequestData {
    url: string;
    method: string;
    headers: Record<string, string>;
    cookies: string;
    body: string;
}
// Track in-flight requests; send only when we have URL/method + headers + cookies/body
const requests: Record<string, Partial<RequestData>> = {};

function maybeSend(requestId: string) {
    const r = requests[requestId];
    if (!r) return;
    if (!r.url || !r.method || !r.headers || r.cookies === undefined || r.body === undefined) {
        return; // wait for missing parts
    }
    // Skip forwarding when itag=243 is present
    if (shouldSkipForward(r.url!)) {
        if (DEBUG) console.log("Skipping forward for itag=243:", r.url);
        delete requests[requestId];
        return;
    }
    const requestData: RequestData = {
        url: r.url!,
        method: r.method!,
        headers: r.headers!,
        cookies: r.cookies as string,
        body: r.body as string,
    };
    fetch(`http://localhost:${goPort}/capture?`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(requestData)
    }).catch(err => console.error("Failed to send request data:", err));
    delete requests[requestId];
}

// Intercept requests for playback chunk headers
chrome.webRequest.onBeforeSendHeaders.addListener(
    (details: chrome.webRequest.WebRequestHeadersDetails) => {
        if (!details.url.includes("googlevideo.com")) return;
        // Reduce work early if we know we'll skip
        if (shouldSkipForward(details.url)) return;
        const entry = requests[details.requestId] || {};
        entry.url = details.url;
        entry.method = details.method ?? entry.method ?? "GET";
        entry.headers = details.requestHeaders?.reduce((acc, header) => {
            if (header.name && header.value) acc[header.name] = header.value;
            return acc;
        }, {} as Record<string, string>) ?? entry.headers ?? {};
        requests[details.requestId] = entry;
        maybeSend(details.requestId);
    },
    { urls: ["*://*.googlevideo.com/*"] },
    ["requestHeaders"]
);

// Intercept requests for playback chunk body and cookies and join
chrome.webRequest.onBeforeRequest.addListener(
    (details: chrome.webRequest.WebRequestBodyDetails) => {
        if (!details.url.includes("googlevideo.com")) return;
        // Reduce work early if we know we'll skip
        if (shouldSkipForward(details.url)) return;
        const entry = requests[details.requestId] || {};
        entry.url = details.url;
        entry.method = details.method ?? entry.method ?? "GET";
        let body = "";
        if (details.requestBody && details.requestBody.raw && details.requestBody.raw[0]?.bytes) {
            try {
                const decoder = new TextDecoder("utf-8");
                body = decoder.decode(details.requestBody.raw[0].bytes);
            } catch (err) {
                console.error("Failed to decode request body:", err);
            }
        }
        entry.body = body;
        // capture cookies asynchronously, then attempt send
        chrome.cookies.getAll({ url: details.url }, (cookies) => {
            entry.cookies = cookies.map(cookie => `${cookie.name}=${cookie.value}`).join("; ");
            requests[details.requestId] = entry;
            maybeSend(details.requestId);
        });
        requests[details.requestId] = entry;
    },
    { urls: ["*://*.googlevideo.com/*"] },
    ["requestBody"]
);

// Intercept requests for YouTube Music Player to get the track ID
// and send it to the Go server.
chrome.webRequest.onBeforeRequest.addListener(
    (details: chrome.webRequest.WebRequestBodyDetails) => {
        if (!details.url.includes("music.youtube.com/youtubei/v1/player")) return;
        if (details.requestBody && details.requestBody.raw) {
            const decoder = new TextDecoder("utf-8");
            const bodyText = decoder.decode(details.requestBody.raw[0].bytes);
            try {
                const bodyJson = JSON.parse(bodyText);
                if (bodyJson.videoId) {
                    fetch(`http://localhost:${goPort}/capturestart`, {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ trackId: bodyJson.videoId })
                    }).catch(err => console.error("Failed to send track ID:", err));
                }
            } catch (error) {
                console.error("Failed to parse YouTube Music Player request body:", error);
            }
        }
    },
    { urls: ["*://music.youtube.com/youtubei/v1/player*"] },
    ["requestBody"]
);

async function getCookies(url: string): Promise<string> {
    return new Promise((resolve) => {
        chrome.cookies.getAll({ url }, (cookies) => {
            if (!cookies) {
                resolve("");
                return;
            }
            resolve(cookies.map(cookie => `${cookie.name}=${cookie.value}`).join("; "));
        });
    });
}

async function updateJobs() {
    if (!isAlreadyAwake) {
        letsStart();
    }
}

async function checkTabs() {
    let results = await chrome.tabs.query({});
    results.forEach(onCreatedTabListener);
}

function onCreatedTabListener(tab: chrome.tabs.Tab): void {
    if (DEBUG) console.log("Created TAB id=", tab.id);
}

function onUpdatedTabListener(tabId: number, changeInfo: chrome.tabs.TabChangeInfo, tab: chrome.tabs.Tab): void {
    if (DEBUG) console.log("Updated TAB id=", tabId);
}

function onRemovedTabListener(tabId: number): void {
    if (DEBUG) console.log("Removed TAB id=", tabId);
}

// ---------------------------
// HIGHLANDER KEEP-ALIVE
// ---------------------------
async function Highlander() {

    const now = Date.now();
    const age = now - firstCall;
    lastCall = now;

    const str = `HIGHLANDER ------< ROUND >------ Time elapsed from first start: ${convertNoDate(age)}`;
    console.log(str)

    if (alivePort == null) {
        alivePort = chrome.runtime.connect({ name: INTERNAL_TESTALIVE_PORT })
        alivePort.onDisconnect.addListener((p) => {
            if (chrome.runtime.lastError) {
                if (DEBUG) console.log(`(DEBUG Highlander) Expected disconnect error. ServiceWorker status should be still RUNNING.`);
            } else {
                if (DEBUG) console.log(`(DEBUG Highlander): port disconnected`);
            }
            alivePort = null;
        });
    }
    if (alivePort) {
        alivePort.postMessage({ content: "ping" });
        if (chrome.runtime.lastError) {
            if (DEBUG) console.log(`(DEBUG Highlander): postMessage error: ${chrome.runtime.lastError.message}`)
        } else {
            if (DEBUG) console.log(`(DEBUG Highlander): "ping" sent through ${alivePort.name} port`)
        }
    }
    if (isFirstStart) {
        isFirstStart = false;
        setTimeout(() => {
            nextRound();
        }, 100);
    }
}

function convertNoDate(long: number): string {
    var dt = new Date(long).toISOString()
    return dt.slice(-13, -5) // HH:MM:SS only
}

function nextRound() {
    clearInterval(wakeup);
    timer = nextSeconds * SECONDS;
    wakeup = setInterval(Highlander, timer);
}

async function initialize() {
    await checkTabs();
    updateJobs();
}

//////////////////////////////
// END HIGHLANDER KEEP-ALIVE//
//////////////////////////////