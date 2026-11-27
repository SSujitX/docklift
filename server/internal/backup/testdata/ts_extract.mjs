// Restores the file part of a backup with the 2.1.6 backend's own code
// (dist/lib/safeUnzip.js + dist/lib/fsCopy.js): node ts_extract.mjs <dist/lib>
// <archive.zip> <temp dir> <target root>. Each area of the archive replaces
// <target root>/<area>, as backup.ts does.
import fs from 'fs';
import path from 'path';
import { pathToFileURL } from 'url';

const [distLib, zipPath, tempDir, targetRoot] = process.argv.slice(2);
const load = (name) => import(pathToFileURL(path.join(distLib, name)).href);
const { safeExtractZip } = await load('safeUnzip.js');
const { replaceDirContents } = await load('fsCopy.js');

await safeExtractZip(zipPath, tempDir);
for (const area of ['deployments', 'nginx-conf', 'letsencrypt']) {
  const src = path.join(tempDir, area);
  if (fs.existsSync(src)) await replaceDirContents(src, path.join(targetRoot, area));
}
const db = path.join(tempDir, 'database', 'docklift.db');
if (fs.existsSync(db)) fs.copyFileSync(db, path.join(targetRoot, 'docklift.db'));
const key = path.join(tempDir, 'github-app.pem');
if (fs.existsSync(key)) fs.copyFileSync(key, path.join(targetRoot, 'github-app.pem'));
