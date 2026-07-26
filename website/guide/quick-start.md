# Quick Start

Short path from a fresh VPS to a live app with a domain.

## 1. Install

On Ubuntu/Debian with Docker:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

The installer prints something like:

```text
Dashboard: http://YOUR_SERVER_IP:8080
Setup code: <bootstrap-secret>
```

## 2. Open the dashboard

Open `http://SERVER_IP:8080` in your browser.

If you lost the setup code:

```bash
docker logs docklift-backend | grep -A8 "Fresh install"
```

```bash
sudo cat /opt/docklift/data/.bootstrap-secret
```

## 3. Complete setup

1. Paste the **Setup code**
2. Create the first admin account (or [restore a backup](./backup.md))
3. Sign in

The bootstrap secret is consumed and deleted after a successful setup.

## 4. Create a project

1. Click **New Project**
2. Choose a source: GitHub URL, private repo (after [GitHub connect](./github.md)), or ZIP upload
3. Optionally set env vars and build mode (**Auto** is fine for most apps)
4. Create the project

## 5. Deploy

Click **Deploy** and watch live build logs. You can cancel mid-build.

By default the app is **Private by default** on the project network — not exposed as `IP:port` until you publish host ports or add a domain.

## 6. Add a domain (preferred)

1. Point a DNS **A record** at your server IP
2. Open the project → **Domains**
3. Add the hostname (e.g. `app.example.com`)
4. Wait for HTTPS to become active

Prefer domains over raw `SERVER_IP:5500`-style URLs. Host ports (`5500`–`5600`) are opt-in under **Build Settings → Publish host ports**.

## Optional next steps

- Enable [Auto-Deploy](./autodeploy.md) on the Source tab for push-to-deploy
- [Link a managed database](./databases.md) instead of pasting credentials by hand
- Set a [panel domain](./domains.md) under **Settings → Domain** for HTTPS access to Docklift itself

## Upgrade later

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```
