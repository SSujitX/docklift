// Application configuration - loads settings from environment variables
import path from 'path';

export const config = {
  port: parseInt(process.env.PORT || '8000'),
  dataPath: path.resolve(process.env.DATA_PATH || './data'),
  deploymentsPath: path.resolve(process.env.DEPLOYMENTS_PATH || './deployments'),
  /** Defaults under dataPath so native/dev installs never write to /data/backups. */
  backupPath: path.resolve(
    process.env.BACKUP_PATH || path.join(process.env.DATA_PATH || './data', 'backups')
  ),
  dockerNetwork: process.env.DOCKER_NETWORK || 'docklift_network',

  // GitHub App settings
  githubAppId: process.env.DOCKLIFT_GITHUB_APP_ID || '',
  githubClientId: process.env.DOCKLIFT_GITHUB_CLIENT_ID || '',
  githubClientSecret: process.env.DOCKLIFT_GITHUB_CLIENT_SECRET || '',
  githubPrivateKeyPath: process.env.DOCKLIFT_GITHUB_PRIVATE_KEY_PATH || './github-app.pem',
  frontendUrl: process.env.DOCKLIFT_FRONTEND_URL || 'http://localhost:3000',
  nginxConfPath: path.resolve(process.env.NGINX_CONF_PATH || './nginx-proxy/conf.d'),
  /** Host port pool for deployed apps (avoid Hyper-V reserved ranges on Windows Docker) */
  portRangeStart: parseInt(process.env.PORT_RANGE_START || '5500', 10),
  portRangeEnd: parseInt(process.env.PORT_RANGE_END || '5600', 10),

  // Let's Encrypt / certbot sidecar
  letsencryptPath: path.resolve(process.env.LETSENCRYPT_PATH || './nginx-proxy/certbot/conf'),
  certbotContainer: process.env.CERTBOT_CONTAINER || 'docklift-certbot',
  certbotEmail: process.env.CERTBOT_EMAIL || '',
  certbotStaging:
    process.env.CERTBOT_STAGING === 'true' || process.env.CERTBOT_STAGING === '1',
};
