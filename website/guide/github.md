# GitHub Integration

Connect a GitHub App you own so Docklift can list private repositories, clone on deploy, and receive push webhooks for [Auto-Deploy](./autodeploy.md).

## Connect (install)

1. Go to **Settings → GitHub**
2. Click **Connect GitHub**
3. Enter an app name (e.g. `my-docklift`)
4. Click **Create GitHub App**
5. Complete the redirect on GitHub
6. Select which repositories to grant access
7. Click **Install** on GitHub
8. Docklift auto-detects the installation

No personal access tokens are required for the normal flow. Set `DOCKLIFT_FRONTEND_URL` to your public dashboard URL (for example `https://panel.example.com` or `http://SERVER_IP:8080`) so OAuth/App callbacks resolve correctly.

## Disconnect

1. Go to **Settings → GitHub**
2. Open **Connect GitHub**
3. Click **Disconnect GitHub**

To fully remove the app from GitHub as well:

**GitHub → Settings → Applications → Installed GitHub Apps → Configure → Uninstall**

## Features

- List repos from every connected personal account and organization
- Filter/search across accounts on **New Project → Private**
- Select a **branch** or **tag** to deploy (tags are newest-first; tag pins do not follow push auto-deploy)
- Auto-pull on redeploy
- Private repository support
- Manifest-based GitHub App creation (no hand-rolled tokens)

## Useful API endpoints

Authenticated admin session required unless noted.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/github/app-status` | App + installation status |
| `POST` | `/api/github/manifest` | Start GitHub App manifest flow |
| `POST` | `/api/github/install-session` | Create install session |
| `POST` | `/api/github/check-installation` | Poll for completed install |
| `GET` | `/api/github/repos` | `{ repositories, failedInstallations, fallbackSingle }` |
| `GET` | `/api/github/branches` | List branches (`?repo=owner/name`) |
| `GET` | `/api/github/tags` | List tags newest-first (`?repo=owner/name`) |
| `POST` | `/api/github/disconnect` | Disconnect the app from Docklift |
| `POST` | `/api/github/webhook` | GitHub webhook receiver (signature verified) |

## Tips

- Prefer installing the app on only the orgs/repos you deploy
- After reconnecting on a new server, restore from [backup](./backup.md) or recreate the GitHub App so private clones work again
- Enable [Auto-Deploy](./autodeploy.md) per project once the source repo is connected
