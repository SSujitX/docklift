// Build-time loader: parse root CHANGELOG.md into HTML release blocks for the homepage preview.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createMarkdownRenderer } from "vitepress";

export interface ChangelogRelease {
  version: string;
  html: string;
}

const rootChangelog = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../CHANGELOG.md",
);

function splitReleases(src: string): string[] {
  const trimmed = src.replace(/^\uFEFF/, "").trim();
  if (!trimmed) return [];
  return trimmed.split(/(?=^## \[)/m).filter((block) => block.trim().length > 0);
}

function versionOf(block: string): string {
  const m = block.match(/^## \[([^\]]+)\]/);
  return m?.[1] ?? "";
}

declare const data: ChangelogRelease[];
export { data };

export default {
  watch: [rootChangelog],
  async load(): Promise<ChangelogRelease[]> {
    if (!fs.existsSync(rootChangelog)) {
      console.warn(`changelog.data: missing ${rootChangelog}`);
      return [];
    }
    const src = fs.readFileSync(rootChangelog, "utf8");
    // html: false — commit subjects/bodies must not inject raw HTML into the homepage.
    const md = await createMarkdownRenderer(path.dirname(rootChangelog), {
      html: false,
    });
    return splitReleases(src).map((block) => ({
      version: versionOf(block),
      html: md.render(block),
    }));
  },
};
