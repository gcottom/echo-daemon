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
const requests: Record<string, Partial<RequestData>> = {};

// Intercept requests for playback chunk headers
chrome.webRequest.onBeforeSendHeaders.addListener(
    (details: chrome.webRequest.WebRequestHeadersDetails) => {
        if (!details.url.includes("googlevideo.com")) return;
        requests[details.requestId] = {
            url: details.url,
            method: details.method ?? "GET",
            headers: details.requestHeaders?.reduce((acc, header) => {
                if (header.name && header.value) acc[header.name] = header.value;
                return acc;
            }, {} as Record<string, string>) ?? {}
        };
    },
    { urls: ["*://*.googlevideo.com/*"] },
    ["requestHeaders"]
);

// Intercept requests for playback chunk body and cookies.
// Join the headers, cookies, and body into a single object and send it to the Go server.
chrome.webRequest.onBeforeRequest.addListener(
    (details: chrome.webRequest.WebRequestBodyDetails) => {
        if (!details.url.includes("googlevideo.com")) return;
        chrome.cookies.getAll({ url: details.url }, (cookies) => {
            let body = "";
            if (details.requestBody && details.requestBody.raw) {
                try {
                    const decoder = new TextDecoder("utf-8");
                    body = decoder.decode(details.requestBody.raw[0].bytes);
                } catch (err) {
                    console.error("Failed to decode request body:", err);
                }
            }
            const storedData = requests[details.requestId] || {};
            const requestData: RequestData = {
                url: details.url,
                method: storedData.method ?? "GET",
                headers: storedData.headers ?? {},
                cookies: cookies.map(cookie => `${cookie.name}=${cookie.value}`).join("; "),
                body: body
            };
            fetch(`http://localhost:${goPort}/capture?`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(requestData)
            }).catch(err => console.error("Failed to send request data:", err));
            delete requests[details.requestId];
        });
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
    if (isAlreadyAwake == false) {
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