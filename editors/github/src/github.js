const DIRECTION_MARKS = /[\u200e\u200f\u202a-\u202e\u2066-\u2069]/g;
const COMMIT_OID = /^[0-9a-f]{40}$/;
const MAX_SOURCE_BYTES = 2_000_000;

export function isTypeRBPath(path) {
  return typeof path === "string" && path.toLowerCase().endsWith(".trb");
}

export function repositoryFromPathname(pathname) {
  const parts = pathname.split("/").filter(Boolean);
  if (parts.length < 2) {
    return null;
  }
  return { owner: parts[0], name: parts[1] };
}

function safePath(path) {
  if (!isTypeRBPath(path)) {
    return false;
  }
  const parts = path.split("/");
  return parts.every((part) => part.length > 0 && part !== "." && part !== "..");
}

export function rawFileURL(origin, repository, oid, path) {
  if (!repository || !COMMIT_OID.test(oid) || !safePath(path)) {
    return null;
  }
  const repositoryPath = [repository.owner, repository.name].map(encodeURIComponent).join("/");
  const filePath = path.split("/").map(encodeURIComponent).join("/");
  return `${origin}/${repositoryPath}/raw/${oid}/${filePath}`;
}

function changesRoute(root) {
  return root?.payload?.pullRequestsChangesRoute ?? root?.pullRequestsChangesRoute ?? null;
}

export function pullRequestMetadataFromPayload(root, repository) {
  const route = changesRoute(root);
  if (!route) {
    return null;
  }
  const comparison = route.comparison?.fullDiff ?? route.pullRequest?.comparison ?? {};
  const baseOid = comparison.baseOid;
  const headOid = comparison.headOid;
  const headOwner = route.pullRequest?.headRepositoryOwnerLogin;
  const headName = route.pullRequest?.headRepositoryName;
  const headRepository = headOwner && headName ? { owner: headOwner, name: headName } : repository;
  const contents = new Map((route.diffContents ?? []).map((item) => [item.pathDigest, item]));
  const summaries = route.diffSummaries?.length ? route.diffSummaries : route.diffContents ?? [];
  const files = new Map();

  for (const summary of summaries) {
    const content = contents.get(summary.pathDigest);
    const path = content?.path ?? summary.path;
    const oldPath = content && Object.hasOwn(content, "oldTreeEntry")
      ? content.oldTreeEntry?.path ?? null
      : path;
    const newPath = content && Object.hasOwn(content, "newTreeEntry")
      ? content.newTreeEntry?.path ?? null
      : path;
    files.set(summary.pathDigest, {
      digest: summary.pathDigest,
      path,
      oldOid: content?.oldCommitOid ?? baseOid,
      newOid: content?.newCommitOid ?? headOid,
      oldPath,
      newPath,
      oldRepository: repository,
      newRepository: headRepository
    });
  }

  return { baseOid, headOid, files };
}

function pullRequestMetadata(document, repository) {
  for (const script of document.querySelectorAll('script[type="application/json"]')) {
    try {
      const parsed = JSON.parse(script.textContent);
      const metadata = pullRequestMetadataFromPayload(parsed, repository);
      if (metadata) {
        return metadata;
      }
    } catch {
      // GitHub owns other application/json script elements with unrelated data.
    }
  }
  return null;
}

function cleanPathText(value) {
  return value?.replace(DIRECTION_MARKS, "").trim() ?? "";
}

function regionPath(region) {
  const heading = region.querySelector('h3 a[href^="#diff-"] code, h3 a[href^="#diff-"]');
  const headingPath = cleanPathText(heading?.textContent);
  if (headingPath) {
    return headingPath;
  }
  return region.querySelector("[data-file-path]")?.getAttribute("data-file-path") ?? "";
}

function replaceWithSegments(element, segments, source) {
  if (!element || segments.length === 0 || element.dataset.typerbSource === source) {
    return;
  }
  const document = element.ownerDocument;
  const nodes = segments.map((segment) => {
    if (!segment.className) {
      return document.createTextNode(segment.text);
    }
    const span = document.createElement("span");
    span.className = segment.className;
    span.textContent = segment.text;
    return span;
  });
  element.replaceChildren(...nodes);
  element.dataset.typerbSource = source;
}

function replaceWithSource(element, source, lines) {
  if (!element || element.dataset.typerbSource === source) {
    return;
  }
  const document = element.ownerDocument;
  const nodes = [];
  for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
    if (lineIndex > 0) {
      nodes.push(document.createTextNode("\n"));
    }
    for (const segment of lines[lineIndex]) {
      if (!segment.className) {
        nodes.push(document.createTextNode(segment.text));
        continue;
      }
      const span = document.createElement("span");
      span.className = segment.className;
      span.textContent = segment.text;
      nodes.push(span);
    }
  }
  element.replaceChildren(...nodes);
  element.dataset.typerbSource = source;
}

export function highlightBlob(document, location, highlighter) {
  if (!location.pathname.includes("/blob/") || !isTypeRBPath(decodeURIComponent(location.pathname))) {
    return;
  }
  const source = document.querySelector('[data-testid="read-only-cursor-text-area"]')?.value;
  if (typeof source !== "string") {
    return;
  }
  const lines = highlighter.tokenizeSource(source);
  for (const element of document.querySelectorAll(".react-file-line[id^=\"LC\"]")) {
    const lineNumber = Number.parseInt(element.id.slice(2), 10);
    const segments = lines[lineNumber - 1];
    if (segments) {
      replaceWithSegments(element, segments, element.textContent);
    }
  }
}

function suggestionBelongsToTypeRB(code) {
  const region = code.closest('[role="region"][id^="diff-"]');
  if (region && isTypeRBPath(regionPath(region))) {
    return true;
  }
  const pathOwner = code.closest("[data-file-path], [data-path]");
  return isTypeRBPath(pathOwner?.getAttribute("data-file-path") ?? pathOwner?.getAttribute("data-path"));
}

export function highlightMarkdown(document, highlighter) {
  const candidates = document.querySelectorAll(
    'pre[lang="trb"] > code, pre[lang="typerb"] > code, pre[lang="suggestion"] > code'
  );
  for (const code of candidates) {
    const language = code.parentElement?.getAttribute("lang")?.toLowerCase();
    if (language === "suggestion" && !suggestionBelongsToTypeRB(code)) {
      continue;
    }
    const source = code.textContent;
    replaceWithSource(code, source, highlighter.tokenizeSource(source));
  }
}

async function fetchTokenLines({ origin, repository, oid, path, highlighter, cache, fetchImpl }) {
  const url = rawFileURL(origin, repository, oid, path);
  if (!url) {
    return null;
  }
  if (!cache.has(url)) {
    if (cache.size >= 64) {
      cache.delete(cache.keys().next().value);
    }
    cache.set(url, (async () => {
      const response = await fetchImpl(url, { credentials: "include" });
      if (!response.ok || response.headers.get("content-type")?.includes("text/html")) {
        return null;
      }
      const source = await response.text();
      if (source.length > MAX_SOURCE_BYTES) {
        return null;
      }
      return highlighter.tokenizeSource(source);
    })().catch(() => null));
  }
  return cache.get(url);
}

async function sideTokenLines({ side, file, origin, repository, highlighter, cache, fetchImpl }) {
  const oldSide = side === "left";
  const oid = oldSide ? file.oldOid : file.newOid;
  const path = oldSide ? file.oldPath : file.newPath;
  const preferredRepository = oldSide ? file.oldRepository : file.newRepository;
  const preferred = await fetchTokenLines({
    origin,
    repository: preferredRepository,
    oid,
    path,
    highlighter,
    cache,
    fetchImpl
  });
  if (preferred || oldSide || preferredRepository.owner === repository.owner && preferredRepository.name === repository.name) {
    return preferred;
  }
  return fetchTokenLines({ origin, repository, oid, path, highlighter, cache, fetchImpl });
}

function fallbackDiffLines(region, highlighter) {
  const state = {
    left: { line: null, ruleStack: highlighter.initialState() },
    right: { line: null, ruleStack: highlighter.initialState() }
  };
  for (const cell of region.querySelectorAll(".diff-text-cell[data-line-number][data-diff-side]")) {
    const side = cell.getAttribute("data-diff-side");
    const target = cell.querySelector(".diff-text-inner");
    const lineNumber = Number.parseInt(cell.getAttribute("data-line-number"), 10);
    if (!target || !state[side] || !Number.isFinite(lineNumber) || target.dataset.typerbSource === target.textContent) {
      continue;
    }
    if (state[side].line !== lineNumber - 1) {
      state[side].ruleStack = highlighter.initialState();
    }
    const source = target.textContent;
    const result = highlighter.tokenizeLine(source, state[side].ruleStack);
    replaceWithSegments(target, result.segments, source);
    state[side] = { line: lineNumber, ruleStack: result.ruleStack };
  }
}

async function highlightDiffRegion({ region, file, repository, origin, highlighter, cache, fetchImpl }) {
  const [oldLines, newLines] = await Promise.all([
    sideTokenLines({ side: "left", file, origin, repository, highlighter, cache, fetchImpl }),
    sideTokenLines({ side: "right", file, origin, repository, highlighter, cache, fetchImpl })
  ]);
  for (const cell of region.querySelectorAll(".diff-text-cell[data-line-number][data-diff-side]")) {
    const target = cell.querySelector(".diff-text-inner");
    const lineNumber = Number.parseInt(cell.getAttribute("data-line-number"), 10);
    const lines = cell.getAttribute("data-diff-side") === "left" ? oldLines : newLines;
    const segments = lines?.[lineNumber - 1];
    if (target && segments) {
      replaceWithSegments(target, segments, target.textContent);
    }
  }
  fallbackDiffLines(region, highlighter);
}

export async function highlightPullRequestDiff(document, location, highlighter, cache, fetchImpl = fetch) {
  if (!/\/pull\/\d+\/(?:files|changes)(?:\/|$)/.test(location.pathname)) {
    return;
  }
  const repository = repositoryFromPathname(location.pathname);
  if (!repository) {
    return;
  }
  const metadata = pullRequestMetadata(document, repository) ?? {
    baseOid: null,
    headOid: null,
    files: new Map()
  };

  const work = [];
  for (const region of document.querySelectorAll('[role="region"][id^="diff-"]')) {
    const digest = region.id.slice("diff-".length);
    const file = metadata.files.get(digest) ?? {
      digest,
      path: regionPath(region),
      oldOid: metadata.baseOid,
      newOid: metadata.headOid,
      oldPath: regionPath(region),
      newPath: regionPath(region),
      oldRepository: repository,
      newRepository: repository
    };
    if (!isTypeRBPath(file.path) && !isTypeRBPath(file.oldPath) && !isTypeRBPath(file.newPath)) {
      continue;
    }
    work.push(highlightDiffRegion({
      region,
      file,
      repository,
      origin: location.origin,
      highlighter,
      cache,
      fetchImpl
    }));
  }
  await Promise.all(work);
}

export async function highlightGitHubDocument({ document, location, highlighter, cache, fetchImpl }) {
  highlightBlob(document, location, highlighter);
  highlightMarkdown(document, highlighter);
  await highlightPullRequestDiff(document, location, highlighter, cache, fetchImpl);
}
