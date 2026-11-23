// Exercise the generated browser-target source with Fetch-compatible in-memory
// responses. No server or network is used; this is not a browser UI test.
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import path from 'node:path';

const root = process.argv[2];
const { ApiClient } = await import(pathToFileURL(path.join(root, 'generated/client.ts')));
const browser = await import(pathToFileURL(path.join(root, 'trb/platform/typescript/browser/index.ts')));
const { Headers: ClientHeaders } = await import(pathToFileURL(path.join(root, 'trb/http/index.ts')));
const client = new ApiClient(new browser.HttpClient('https://example.invalid'));
const input = { params: { id: 42 }, query: { page: 2, tag: ['first', '空 白'] }, body: { title: 'sample' } };
let calls = [];
let checks = 0;
let responder = () => new Response('{"id":7}', { status: 201, headers: { 'content-type': 'application/json' } });
globalThis.fetch = async (url, options) => {
  calls.push({ url: String(url), options });
  return responder(url, options);
};
const invoke = (value = input, options = {}) => client.create_item(undefined, value, options);
let result = await invoke(input, { headers: new ClientHeaders([{ name: 'x-client', value: 'fixture' }]), timeout_milliseconds: 1000 });
assert.equal(result.kind, 'Ok');
assert.equal(result.value.kind, 'Status201');
assert.equal(result.value.response.__trb_body.id, 7);
checks++;
assert.equal(new URL(calls[0].url).pathname, '/items/42');
assert.deepEqual(new URL(calls[0].url).searchParams.getAll('tag'), ['first', '空 白']);
assert.equal(new URL(calls[0].url).searchParams.get('page'), '2');
checks++;
assert.equal(calls[0].options.method, 'POST');
assert.equal(new Headers(calls[0].options.headers).get('x-client'), 'fixture');
assert.equal(new Headers(calls[0].options.headers).get('content-type'), 'application/json');
checks++;
assert.deepEqual(JSON.parse(calls[0].options.body), input.body);
checks++;

calls = [];
await invoke({ ...input, query: { page: null, tag: [] } });
assert.equal(new URL(calls[0].url).search, '');
checks++;

responder = () => new Response('{"message":"invalid"}', { status: 400, headers: { 'content-type': 'application/json' } });
result = await invoke();
assert.equal(result.kind, 'Ok');
assert.equal(result.value.kind, 'Status400');
assert.equal(result.value.response.__trb_body.message, 'invalid');
checks++;

responder = () => new Response(null, { status: 204 });
result = await invoke();
assert.equal(result.kind, 'Ok');
assert.equal(result.value.kind, 'Status204');
checks++;

responder = () => new Response('unavailable', { status: 503 });
result = await invoke();
assert.equal(result.kind, 'Err');
assert.equal(result.error.__trb_kind, browser.RequestErrorKind.Contract);
assert.equal(result.error.__trb_response.__trb_status, 503);
checks++;

responder = () => new Response('not json', { status: 201, headers: { 'content-type': 'application/json' } });
result = await invoke();
assert.equal(result.kind, 'Err');
assert.equal(result.error.__trb_kind, browser.RequestErrorKind.Contract);
checks++;

responder = () => new Response('{"id":"wrong"}', { status: 201, headers: { 'content-type': 'application/json' } });
result = await invoke();
assert.equal(result.kind, 'Err');
assert.equal(result.error.__trb_kind, browser.RequestErrorKind.Contract);
checks++;

responder = () => { throw new TypeError('synthetic network failure'); };
result = await invoke();
assert.equal(result.kind, 'Err');
assert.equal(result.error.__trb_kind, browser.RequestErrorKind.Network);
checks++;

responder = (_url, options) => new Promise((_resolve, reject) => {
  const abort = () => reject(new DOMException('synthetic abort', 'AbortError'));
  if (options.signal.aborted) abort();
  else options.signal.addEventListener('abort', abort, { once: true });
});
result = await invoke(input, { timeout_milliseconds: 1 });
assert.equal(result.kind, 'Err');
assert.equal(result.error.__trb_kind, browser.RequestErrorKind.Timeout);
checks++;

// Group the successful response/request assertions as four checks, then eight
// independent outcome paths above, for a stable twelve-check contract.
assert.equal(checks, 12);
console.log(`${checks} transport checks passed`);
