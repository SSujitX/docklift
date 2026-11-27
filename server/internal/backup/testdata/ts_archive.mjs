// Builds a backup archive exactly as backend/src/routes/backup.ts (2.1.6)
// does, with the backend's own archiver: node ts_archive.mjs <node_modules>
// <out.zip> <db> <deployments> <nginx-conf> <letsencrypt> <github-app.pem>
import fs from 'fs';
import path from 'path';
import { pathToFileURL } from 'url';

const [nodeModules, out, db, deployments, nginxConf, letsencrypt, githubKey] = process.argv.slice(2);
const { ZipArchive } = await import(pathToFileURL(path.join(nodeModules, 'archiver', 'index.js')).href);

const output = fs.createWriteStream(out);
const archive = new ZipArchive({ zlib: { level: 9 } });
const closed = new Promise((resolve, reject) => {
  output.on('close', resolve);
  output.on('error', reject);
  archive.on('error', reject);
});
archive.pipe(output);
archive.file(db, { name: 'database/docklift.db' });
if (fs.existsSync(deployments)) archive.directory(deployments, 'deployments');
if (fs.existsSync(nginxConf)) archive.directory(nginxConf, 'nginx-conf');
if (fs.existsSync(letsencrypt)) archive.directory(letsencrypt, 'letsencrypt');
if (fs.existsSync(githubKey)) archive.file(githubKey, { name: 'github-app.pem' });
await archive.finalize();
await closed;
