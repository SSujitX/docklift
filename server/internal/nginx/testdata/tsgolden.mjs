// Renders nginx confs through the TypeScript backend (backend/dist) for the
// Go golden tests. Usage: node tsgolden.mjs <backend/dist dir> < cases.json
// LETSENCRYPT_PATH must point at the fixture certificates before startup.
// Prints a JSON array with one rendered conf per case.
import { pathToFileURL } from 'node:url';
import path from 'node:path';

const dist = process.argv[2];
const ssl = await import(pathToFileURL(path.join(dist, 'services', 'nginxSsl.js')).href);

let input = '';
for await (const chunk of process.stdin) input += chunk;
const cases = JSON.parse(input);

const out = cases.map((c) => {
  if (c.kind === 'service') {
    return ssl.buildHttpHttpsServers({
      comment: `Main server block for ${c.name}`,
      serverNames: c.hosts.join(' '),
      primaryDomain: c.hosts[0],
      proxyLocation: ssl.buildServiceProxyLocation(c.id, c.container, c.port),
      enableHttps: c.enableHttps,
    });
  }
  const body = ssl.buildHttpHttpsServers({
    comment: `Panel domain ${c.domain}`,
    serverNames: c.domain,
    primaryDomain: c.domain,
    proxyLocation: ssl.buildPanelProxyLocation(c.port),
    enableHttps: c.enableHttps,
  });
  return `${ssl.PANEL_CONF_MARKER}\n# docklift-panel-port: ${c.port}\n${body}`;
});
process.stdout.write(JSON.stringify(out));
process.exit(0);
