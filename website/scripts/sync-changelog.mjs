#!/usr/bin/env node
// Copy root CHANGELOG.md into the VitePress tree so /changelog stays in sync.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const websiteRoot = path.resolve(__dirname, "..");
const repoRoot = path.resolve(websiteRoot, "..");
const source = path.join(repoRoot, "CHANGELOG.md");
const dest = path.join(websiteRoot, "changelog.md");

if (!fs.existsSync(source)) {
  console.error(`sync-changelog: missing ${source}`);
  process.exit(1);
}

const body = fs.readFileSync(source, "utf8").trimEnd();
const out = `---
title: Changelog
description: Release history for Docklift — generated from the repository CHANGELOG.md.
editLink: false
lastUpdated: false
---

# Changelog

One section per release, written by the Release workflow from the git log since the previous tag
(semantic-release wrote the entries up to 3.0.0).
Source of truth: repository root \`CHANGELOG.md\` (do not edit this page by hand).

${body}
`;

fs.writeFileSync(dest, out, "utf8");
console.log(`sync-changelog: wrote ${path.relative(repoRoot, dest)}`);
