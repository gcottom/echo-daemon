const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const test = require('node:test');
const vm = require('node:vm');

test('extension sends only a valid video ID to the loopback daemon', async () => {
    const manifest = JSON.parse(readFileSync('dist/manifest.json'));
    assert.deepEqual(manifest.permissions, ['webRequest']);
    assert.ok(manifest.host_permissions.includes('http://127.0.0.1/*'));
    let listener;
    const calls = [];
    const body = text => ({requestBody: {raw: [{bytes: Buffer.from(text)}]}});
    vm.runInNewContext(readFileSync('dist/background.js', 'utf8'), {
        TextDecoder, console,
        chrome: {webRequest: {onBeforeRequest: {addListener: fn => {listener = fn;}}}},
        fetch: (...args) => {calls.push(args); return Promise.resolve({ok: true});},
    });
    listener({});
    listener({requestBody: {raw: []}});
    listener(body('bad JSON'));
    listener(body('{"videoId":"bad/id"}'));
    listener(body('{"videoId":"Example1234","other":"not forwarded"}'));
    assert.deepEqual(calls, [['http://127.0.0.1:50999/download/Example1234']]);
});
