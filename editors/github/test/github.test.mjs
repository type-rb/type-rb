import assert from "node:assert/strict";
import { test } from "node:test";

import {
  highlightBlob,
  isTypeRBPath,
  pullRequestMetadataFromPayload,
  rawFileURL,
  repositoryFromPathname
} from "../src/github.js";

test("preserves GitHub's blank-line placeholder in blob views", () => {
  let replacementCalls = 0;
  const blankLine = {
    id: "LC2",
    textContent: "",
    dataset: {},
    ownerDocument: {},
    replaceChildren() {
      replacementCalls += 1;
    }
  };
  const document = {
    querySelector() {
      return { value: "record Entry\n\nend" };
    },
    querySelectorAll() {
      return [blankLine];
    }
  };
  const highlighter = {
    tokenizeSource() {
      return [[], [], []];
    }
  };

  highlightBlob(
    document,
    { pathname: "/type-rb/example/blob/main/src/audit.trb" },
    highlighter
  );

  assert.equal(replacementCalls, 0);
});

test("recognizes TypeRB paths and GitHub repository routes", () => {
  assert.equal(isTypeRBPath("src/main.trb"), true);
  assert.equal(isTypeRBPath("src/main.rb"), false);
  assert.deepEqual(repositoryFromPathname("/type-rb/type-rb/pull/42/changes"), {
    owner: "type-rb",
    name: "type-rb"
  });
});

test("builds a constrained raw file URL", () => {
  const oid = "a".repeat(40);
  assert.equal(
    rawFileURL(
      "https://github.com",
      { owner: "type-rb", name: "type-rb" },
      oid,
      "examples/hello world.trb"
    ),
    `https://github.com/type-rb/type-rb/raw/${oid}/examples/hello%20world.trb`
  );
  assert.equal(rawFileURL("https://github.com", { owner: "x", name: "y" }, oid, "../secret.trb"), null);
});

test("extracts exact base and head sources for renamed pull request files", () => {
  const baseOid = "a".repeat(40);
  const headOid = "b".repeat(40);
  const digest = "digest";
  const metadata = pullRequestMetadataFromPayload({
    payload: {
      pullRequestsChangesRoute: {
        comparison: { fullDiff: { baseOid, headOid } },
        pullRequest: {
          headRepositoryOwnerLogin: "contributor",
          headRepositoryName: "type-rb"
        },
        diffSummaries: [{ path: "src/new.trb", pathDigest: digest }],
        diffContents: [{
          path: "src/new.trb",
          pathDigest: digest,
          oldCommitOid: baseOid,
          newCommitOid: headOid,
          oldTreeEntry: { path: "src/old.trb" },
          newTreeEntry: { path: "src/new.trb" }
        }]
      }
    }
  }, { owner: "type-rb", name: "type-rb" });

  assert.deepEqual(metadata.files.get(digest), {
    digest,
    path: "src/new.trb",
    oldOid: baseOid,
    newOid: headOid,
    oldPath: "src/old.trb",
    newPath: "src/new.trb",
    oldRepository: { owner: "type-rb", name: "type-rb" },
    newRepository: { owner: "contributor", name: "type-rb" }
  });
});
