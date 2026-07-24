// Shared nginx vhost templates for ACME + optional HTTPS (Let's Encrypt)
import { certificateFilesExist, nginxCertPaths } from './certs.js';

const ACME_INCLUDE = 'include /etc/nginx/snippets/acme.conf;';

export function buildServiceProxyLocation(
  serviceId: string,
  containerName: string,
  internalPort: number
): string {
  const varName = `target_${serviceId.replace(/-/g, '_')}`;
  return `
    location / {
        set $${varName} ${containerName};
        proxy_pass http://$${varName}:${internalPort};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }`;
}

/** Panel / server domain upstream — prefer docker DNS for dashboard port 8080 */
export function buildPanelProxyLocation(port: number): string {
  const useDashboardContainer = Number(port) === 8080;
  if (useDashboardContainer) {
    return `
    location / {
        set $dashboard docklift-nginx;
        proxy_pass http://$dashboard:80;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
        proxy_buffering off;
    }`;
  }
  return `
    location / {
        proxy_pass http://host.docker.internal:${port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $http_host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }`;
}

export function buildHttpHttpsServers(opts: {
  comment: string;
  serverNames: string;
  primaryDomain: string;
  proxyLocation: string;
  enableHttps: boolean;
}): string {
  const { comment, serverNames, primaryDomain, proxyLocation, enableHttps } = opts;
  const hasCert = enableHttps && certificateFilesExist(primaryDomain);
  const certs = nginxCertPaths(primaryDomain);

  if (!hasCert) {
    return `
# ${comment}
# docklift-ssl: pending|http-only primary=${primaryDomain}
server {
    listen 80;
    server_name ${serverNames};

    resolver 127.0.0.11 valid=30s ipv6=off;

    ${ACME_INCLUDE}
${proxyLocation}
}
`;
  }

  return `
# ${comment}
# docklift-ssl: active primary=${primaryDomain}
server {
    listen 80;
    server_name ${serverNames};

    ${ACME_INCLUDE}

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name ${serverNames};

    ssl_certificate ${certs.fullchain};
    ssl_certificate_key ${certs.privkey};

    resolver 127.0.0.11 valid=30s ipv6=off;
${proxyLocation}
}
`;
}

/** Marker used by panel domain listing */
export const PANEL_CONF_MARKER = '# docklift-panel-domain';
