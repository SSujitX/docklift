---
layout: home
title: Docklift Docs
hero:
  name: Docklift
  text: Self-hosted Docker PaaS
  tagline: Deploy from GitHub or a ZIP on your VPS — domains, HTTPS, databases, and ops tools in one dashboard.
  actions:
    - theme: brand
      text: Quick Start
      link: /guide/quick-start
    - theme: alt
      text: Useful Commands
      link: /guide/commands
    - theme: alt
      text: GitHub
      link: https://github.com/SSujitX/docklift
features:
  - title: One-command install
    details: curl install on Ubuntu/Debian. Dashboard at SERVER_IP:8080 with a one-time setup code.
    link: /guide/installation
    linkText: Installation
  - title: Git to deploy
    details: GitHub App, Dockerfile or Railpack builds, multi-service projects, auto-deploy on push.
    link: /guide/deployment
    linkText: Deployment
  - title: Domains & HTTPS
    details: Custom domains with Let's Encrypt. Host ports stay opt-in — domains preferred.
    link: /guide/domains
    linkText: Custom domains
  - title: Managed databases
    details: Postgres, MySQL, MariaDB, Redis, Mongo — private by default, linkable into apps.
    link: /guide/databases
    linkText: Databases
  - title: Ops toolbox
    details: System stats, web terminal, logs, backup/restore, and a full commands reference.
    link: /guide/commands
    linkText: Commands
  - title: Open source
    details: Your server, your data. Coolify / Dokku / CapRover-style workflow with a real UI.
    link: /guide/introduction
    linkText: Introduction
---

## Install in one line

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Then open `http://SERVER_IP:8080`, paste the setup code from the installer output, and create your admin account.

[Continue with Quick Start →](/guide/quick-start)
