"use strict";

const path = require("node:path");

function stripJSONCComments(source) {
	let result = "";
	let inString = false;
	let escaped = false;
	let lineComment = false;
	let blockComment = false;
	for (let index = 0; index < source.length; index += 1) {
		const current = source[index];
		const next = source[index + 1];
		if (lineComment) {
			if (current === "\n" || current === "\r") {
				lineComment = false;
				result += current;
			} else {
				result += " ";
			}
			continue;
		}
		if (blockComment) {
			if (current === "*" && next === "/") {
				result += "  ";
				index += 1;
				blockComment = false;
			} else {
				result += current === "\n" || current === "\r" ? current : " ";
			}
			continue;
		}
		if (inString) {
			result += current;
			if (escaped) {
				escaped = false;
			} else if (current === "\\") {
				escaped = true;
			} else if (current === '"') {
				inString = false;
			}
			continue;
		}
		if (current === '"') {
			inString = true;
			result += current;
			continue;
		}
		if (current === "/" && next === "/") {
			lineComment = true;
			result += "  ";
			index += 1;
			continue;
		}
		if (current === "/" && next === "*") {
			blockComment = true;
			result += "  ";
			index += 1;
			continue;
		}
		result += current;
	}
	return result;
}

function projectPaths(configPath, source) {
	const config = JSON.parse(stripJSONCComments(source));
	const root = path.dirname(path.resolve(configPath));
	const sourceDir = config.sourceDir === undefined || config.sourceDir === "" ? "." : config.sourceDir;
	const outDir = config.outDir === undefined || config.outDir === "" ? "build" : config.outDir;
	if (typeof sourceDir !== "string" || typeof outDir !== "string") {
		throw new TypeError("sourceDir and outDir must be strings");
	}
	return {
		name: typeof config.name === "string" ? config.name.trim() : "",
		mode: typeof config.mode === "string" ? config.mode : "",
		root,
		sourceRoot: path.resolve(root, sourceDir),
		outputRoot: path.resolve(root, outDir),
		runnable: config.mode !== "typescript" || config.typescript?.runtime !== "browser"
	};
}

function containsPath(root, filename) {
	const relative = path.relative(path.resolve(root), path.resolve(filename));
	return relative === "" || (relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative));
}

function literalGlobPattern(value) {
	const replacements = { "*": "[*]", "?": "[?]", "[": "[[]", "]": "[]]", "{": "[{]", "}": "[}]" };
	return [...value].map((character) => replacements[character] ?? character).join("");
}

function projectForPath(projects, filename) {
	let selected;
	for (const project of projects) {
		if (!containsPath(project.sourceRoot, filename)) {
			continue;
		}
		if (selected === undefined || project.sourceRoot.length > selected.sourceRoot.length) {
			selected = project;
		}
	}
	return selected;
}

function excludeGeneratedProjects(projects) {
	return projects.filter((project) => !projects.some((candidate) =>
		candidate !== project && containsPath(candidate.outputRoot, project.configPath)
	));
}

module.exports = { containsPath, excludeGeneratedProjects, literalGlobPattern, projectForPath, projectPaths, stripJSONCComments };
