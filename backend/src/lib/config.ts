// Application configuration - loads settings from environment variables
import path from 'path';

export const config = {
  port: parseInt(process.env.PORT || '8000'),
  dataPath: path.resolve(process.env.DATA_PATH || './data'),
  deploymentsPath: path.resolve(process.env.DEPLOYMENTS_PATH || './deployments'),
  dockerNetwork: process.env.DOCKER_NETWORK || 'docklift_network',

  // GitHub App settings
  githubAppId: process.env.DOCKLIFT_GITHUB_APP_ID || '',
  githubClientId: process.env.DOCKLIFT_GITHUB_CLIENT_ID || '',
  githubClientSecret: process.env.DOCKLIFT_GITHUB_CLIENT_SECRET || '',
  githubPrivateKeyPath: process.env.DOCKLIFT_GITHUB_PRIVATE_KEY_PATH || './github-app.pem',
  frontendUrl: process.env.DOCKLIFT_FRONTEND_URL || 'http://localhost:3000',
  nginxConfPath: path.resolve(process.env.NGINX_CONF_PATH || './nginx-proxy/conf.d'),
};
