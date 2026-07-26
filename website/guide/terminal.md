# Web Terminal

Full SSH-like terminal in the browser at `/terminal` (xterm.js), backed by a privileged host session.

## Security

- **Double auth:** your session JWT is not enough. Opening the root shell and running host actions require your **account password** again (step-up)
- **Cancel aborts** — if you dismiss the password prompt, the action does not continue
- Treat the terminal as root access to the VPS; restrict who can log into the panel

## Features

- Interactive bash PTY with tab completion and history
- Root access to the host via Docker privileged mode
- **Host strip** above the shell: update packages, upgrade Docklift, reset stack, reboot, and related ops as plain-text actions
- Upgrade / update dialogs: confirm before start (panel may go offline; target version shown); password; then a wait dialog with refresh timing (Esc/overlay locked until the countdown ends)
- Full-screen mode
- Light and dark shell colors follow the panel theme
- Clipboard: select to copy; `Ctrl+C` copy/interrupt; `Ctrl+V` paste
- Common tools available in the environment (for example `htop`, `docker`, `git`, `nano`)

## Common commands

```bash
# List running containers
docker ps

# View container logs
docker logs <container_id>

# Live resource usage
docker stats

# System process monitor
htop
```

Docklift platform containers use names like `docklift-backend`. User apps use `dl_<slug>_<id>_<service>`.

```bash
docker ps --filter name=docklift --filter name=dl_
```

## When to use the terminal vs the UI

| Task | Prefer |
|------|--------|
| Deploy / redeploy / domains | Dashboard |
| Live build logs | Project deployments |
| Host package updates / reboot | Terminal host strip (with password) |
| Ad-hoc debugging | Terminal |
| Password reset | SSH + [reset script](./reset-password.md) — not from the web UI alone |
