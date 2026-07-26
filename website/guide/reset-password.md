# Emergency Reset Password

If you lose access to the admin account, reset the password from the server with Docker. This cannot be done from the web dashboard alone.

## Security

The reset script must run **on the host** where Docklift is installed (SSH or console). Anyone with shell access to the Docker socket can reset the admin password — protect SSH accordingly.

## Production (Docker)

```bash
ssh root@your-server-ip
```

```bash
docker exec -it docklift-backend node dist/scripts/reset-password.js
```

The CLI prints a new random password (typically 16 characters) for the admin user.

## After reset

1. Open `http://SERVER_IP:8080` (or your panel domain)
2. Sign in with the temporary password
3. Go to **Profile Settings** and set a permanent password

## Local development

```bash
cd backend
bun run reset-password
```

## Related

- [Profile Management](./profile.md) — change password while logged in
- [Commands](./commands.md) — full ops cheat sheet
