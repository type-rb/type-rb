import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

import { chromeManifest } from "../src/chrome-manifest.js";
import {
  itemResourceName,
  releaseStaged,
  submitPackage
} from "../webstore.mjs";

const testDirectory = dirname(fileURLToPath(import.meta.url));
const packageDirectory = resolve(testDirectory, "..");

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" }
  });
}

async function assertPngDimensions(path, width, height) {
  const image = await readFile(path);
  assert.deepEqual(image.subarray(1, 4).toString(), "PNG");
  assert.equal(image.readUInt32BE(16), width);
  assert.equal(image.readUInt32BE(20), height);
}

test("builds a minimal Manifest V3 content-script extension", () => {
  const manifest = chromeManifest("0.2.0");
  assert.equal(manifest.manifest_version, 3);
  assert.equal(manifest.version, "0.2.0");
  assert.deepEqual(manifest.permissions, undefined);
  assert.deepEqual(manifest.host_permissions, undefined);
  assert.deepEqual(manifest.content_scripts, [{
    matches: ["https://github.com/*"],
    css: ["content.css"],
    js: ["content.js"],
    run_at: "document_idle"
  }]);
});

test("ships all required raster icon sizes", async () => {
  for (const size of [16, 32, 48, 128]) {
    await assertPngDimensions(resolve(packageDirectory, `assets/icon-${size}.png`), size, size);
  }
});

test("ships Chrome Web Store graphics at the required dimensions", async () => {
  await assertPngDimensions(
    resolve(packageDirectory, "store-assets/screenshot-1280x800.png"),
    1280,
    800
  );
  await assertPngDimensions(
    resolve(packageDirectory, "store-assets/promo-small-440x280.png"),
    440,
    280
  );
});

test("constructs the Chrome Web Store v2 item resource", () => {
  assert.equal(
    itemResourceName("publisher-id", "extension-id"),
    "publishers/publisher-id/items/extension-id"
  );
});

test("uploads a package before requesting staged publication", async () => {
  const calls = [];
  const fetchImpl = async (url, init = {}) => {
    calls.push({ url, init });
    if (url.includes(":upload")) {
      return jsonResponse({ uploadState: "SUCCEEDED", crxVersion: "0.2.0" });
    }
    return jsonResponse({ state: "PENDING_REVIEW" });
  };
  const result = await submitPackage({
    accessToken: "token",
    publisherId: "publisher",
    extensionId: "extension",
    packagePath: resolve(packageDirectory, "package.json"),
    fetchImpl
  });

  assert.equal(result.state, "PENDING_REVIEW");
  assert.equal(calls.length, 2);
  assert.match(calls[0].url, /\/upload\/v2\/publishers\/publisher\/items\/extension:upload$/);
  assert.equal(calls[0].init.headers.authorization, "Bearer token");
  assert.match(calls[1].url, /\/v2\/publishers\/publisher\/items\/extension:publish$/);
  assert.deepEqual(JSON.parse(calls[1].init.body), {
    publishType: "STAGED_PUBLISH",
    blockOnWarnings: true
  });
});

test("releases an approved staged revision without uploading again", async () => {
  const calls = [];
  await releaseStaged({
    accessToken: "token",
    publisherId: "publisher",
    extensionId: "extension",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      if (url.includes(":fetchStatus")) {
        return jsonResponse({ submittedItemRevisionStatus: { state: "STAGED" } });
      }
      return jsonResponse({ state: "PUBLISHED" });
    }
  });

  assert.equal(calls.length, 2);
  assert.match(calls[0].url, /:fetchStatus$/);
  assert.match(calls[1].url, /:publish$/);
  assert.equal(JSON.parse(calls[1].init.body).publishType, "DEFAULT_PUBLISH");
});

test("refuses to release a revision that has not been approved and staged", async () => {
  await assert.rejects(
    releaseStaged({
      accessToken: "token",
      publisherId: "publisher",
      extensionId: "extension",
      fetchImpl: async () => jsonResponse({
        submittedItemRevisionStatus: { state: "PENDING_REVIEW" }
      })
    }),
    /expected STAGED/
  );
});
