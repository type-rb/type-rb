import { readFile } from "node:fs/promises";
import { basename, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const API_ORIGIN = "https://chromewebstore.googleapis.com";

function required(value, name) {
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

export function itemResourceName(publisherId, extensionId) {
  return `publishers/${encodeURIComponent(required(publisherId, "publisher ID"))}/items/${encodeURIComponent(required(extensionId, "extension ID"))}`;
}

async function responseJson(response) {
  const text = await response.text();
  const body = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const message = body.error?.message ?? `${response.status} ${response.statusText}`;
    throw new Error(`Chrome Web Store API request failed: ${message}`);
  }
  return body;
}

function authorizationHeaders(accessToken, extra = {}) {
  return {
    authorization: `Bearer ${required(accessToken, "access token")}`,
    ...extra
  };
}

export async function uploadPackage({
  accessToken,
  publisherId,
  extensionId,
  packagePath,
  fetchImpl = fetch
}) {
  const file = await readFile(required(packagePath, "package path"));
  const name = itemResourceName(publisherId, extensionId);
  const response = await fetchImpl(`${API_ORIGIN}/upload/v2/${name}:upload`, {
    method: "POST",
    headers: authorizationHeaders(accessToken, {
      "content-type": "application/zip",
      "x-goog-upload-file-name": basename(packagePath),
      "x-goog-upload-protocol": "raw"
    }),
    body: file
  });
  return responseJson(response);
}

export async function fetchStatus({ accessToken, publisherId, extensionId, fetchImpl = fetch }) {
  const name = itemResourceName(publisherId, extensionId);
  const response = await fetchImpl(`${API_ORIGIN}/v2/${name}:fetchStatus`, {
    headers: authorizationHeaders(accessToken)
  });
  return responseJson(response);
}

export async function waitForUpload({
  accessToken,
  publisherId,
  extensionId,
  fetchImpl = fetch,
  sleep = (milliseconds) => new Promise((resolveSleep) => setTimeout(resolveSleep, milliseconds)),
  attempts = 60
}) {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const status = await fetchStatus({ accessToken, publisherId, extensionId, fetchImpl });
    if (status.lastAsyncUploadState !== "IN_PROGRESS") {
      return status;
    }
    await sleep(5_000);
  }
  throw new Error("Chrome Web Store package upload did not finish within five minutes");
}

export async function publishItem({
  accessToken,
  publisherId,
  extensionId,
  publishType,
  fetchImpl = fetch
}) {
  const name = itemResourceName(publisherId, extensionId);
  const response = await fetchImpl(`${API_ORIGIN}/v2/${name}:publish`, {
    method: "POST",
    headers: authorizationHeaders(accessToken, { "content-type": "application/json" }),
    body: JSON.stringify({ publishType, blockOnWarnings: true })
  });
  return responseJson(response);
}

export async function submitPackage(options) {
  const upload = await uploadPackage(options);
  let status = upload;
  if (upload.uploadState === "IN_PROGRESS") {
    status = await waitForUpload(options);
  }
  const uploadState = status.uploadState ?? status.lastAsyncUploadState;
  if (uploadState !== "SUCCEEDED") {
    throw new Error(`Chrome Web Store package upload ended in ${uploadState ?? "an unknown state"}`);
  }
  return publishItem({ ...options, publishType: "STAGED_PUBLISH" });
}

export async function releaseStaged(options) {
  const status = await fetchStatus(options);
  const state = status.submittedItemRevisionStatus?.state;
  if (state !== "STAGED") {
    throw new Error(`Chrome Web Store item is ${state ?? "not staged"}; expected STAGED`);
  }
  return publishItem({ ...options, publishType: "DEFAULT_PUBLISH" });
}

async function main() {
  const command = process.argv[2];
  const options = {
    accessToken: process.env.CWS_ACCESS_TOKEN,
    publisherId: process.env.CWS_PUBLISHER_ID,
    extensionId: process.env.CWS_EXTENSION_ID
  };

  let result;
  if (command === "submit") {
    result = await submitPackage({ ...options, packagePath: process.argv[3] });
  } else if (command === "release-staged") {
    result = await releaseStaged(options);
  } else if (command === "status") {
    result = await fetchStatus(options);
  } else {
    throw new Error("Usage: node webstore.mjs <submit PACKAGE.zip|release-staged|status>");
  }
  console.log(JSON.stringify(result, null, 2));
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  await main();
}
