import { readFile, readdir, writeFile } from "node:fs/promises";
import { extname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { createHighlighter } from "shiki";

const codeBlockPattern = /<pre><code class="language-([a-z0-9_-]+)">([\s\S]*?)<\/code><\/pre>/g;
const languageMap = new Map([
  ["console", "shellscript"],
  ["dockerfile", "dockerfile"],
  ["json", "json"],
  ["jsonc", "jsonc"],
  ["sh", "shellscript"],
  ["text", "text"],
  ["trb", "trb"],
  ["typerb", "trb"]
]);

const namedEntities = new Map([
  ["amp", "&"],
  ["apos", "'"],
  ["gt", ">"],
  ["lt", "<"],
  ["quot", "\""]
]);

function decodeHTML(value) {
  return value.replace(/&(?:#(\d+)|#x([\da-f]+)|([a-z]+));/gi, (entity, decimal, hexadecimal, name) => {
    if (decimal) return String.fromCodePoint(Number.parseInt(decimal, 10));
    if (hexadecimal) return String.fromCodePoint(Number.parseInt(hexadecimal, 16));
    return namedEntities.get(name.toLowerCase()) ?? entity;
  });
}

async function createSiteHighlighter(grammarPath) {
  const grammar = JSON.parse(await readFile(grammarPath, "utf8"));
  const typerb = {
    ...grammar,
    name: "trb",
    aliases: ["typerb"]
  };
  return createHighlighter({
    langs: [typerb, "dockerfile", "json", "jsonc", "shellscript"],
    themes: ["github-dark-default"]
  });
}

export function highlightHTML(source, highlighter) {
  return source.replace(codeBlockPattern, (_block, authoredLanguage, encodedCode) => {
    const language = languageMap.get(authoredLanguage);
    if (!language) {
      throw new Error(`unsupported documentation code-block language: ${authoredLanguage}`);
    }
    return highlighter.codeToHtml(decodeHTML(encodedCode), {
      lang: language,
      theme: "github-dark-default"
    });
  });
}

async function htmlFiles(directory) {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filePath = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await htmlFiles(filePath));
    else if (entry.isFile() && extname(entry.name) === ".html") files.push(filePath);
  }
  return files;
}

export async function highlightSite(siteDirectory, grammarPath) {
  const highlighter = await createSiteHighlighter(grammarPath);
  try {
    for (const filePath of await htmlFiles(siteDirectory)) {
      const source = await readFile(filePath, "utf8");
      const highlighted = highlightHTML(source, highlighter);
      if (highlighted !== source) await writeFile(filePath, highlighted);
    }
  } finally {
    highlighter.dispose();
  }
}

async function main() {
  const [siteDirectory, grammarPath] = process.argv.slice(2);
  if (!siteDirectory || !grammarPath) {
    throw new Error("usage: node tools/site/highlight.mjs SITE_DIRECTORY TYPERB_GRAMMAR");
  }
  await highlightSite(resolve(siteDirectory), resolve(grammarPath));
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await main();
}
