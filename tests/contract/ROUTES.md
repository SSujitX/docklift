# Docklift HTTP routes and contract coverage

Derived by reading `backend/src/index.ts` and every router in `backend/src/routes/`.
Count check: `grep -cE "router\.(get|post|put|patch|delete)\(" backend/src/routes/*.ts` sums to **99**
(auth 9, backup 12, databases 8, deployments 14, domains 8, files 3, github 15, logs 2, ports 2,
projects 15, system 11). `backup.ts:914` is the one multi-line `router.post(` (`/restore-upload`).
Plus `GET /api/health` (index.ts) and the `/ws/terminal` upgrade (services/terminal.ts): **101 entries**.

The **Go handler** column is the handler function under `server/internal/` (file:line); the mount
table, group auth guards and rate limiters are in `server/internal/app/router/router.go`.

Auth types:

- **public** — no credentials.
- **public+limit** — public, behind the 100/15 min auth limiter (`/status` is skipped).
- **public+setup** — public, behind both the 10/15 min setup limiter and the auth limiter; needs the bootstrap secret.
- **bearer** — `authMiddleware`: `Authorization: Bearer <session JWT>` (purpose `sse`/`terminal` refused, `pwdv` checked) or `X-Internal-Secret` (TS only; Go removed the internal-secret bypass, the header is ignored).
- **bearer+limit** — bearer, inside `/api/auth` (auth limiter applies). Go only: `/me`, `/sse-token` and
  `/terminal-token` are skipped by the auth limiter and have their own 1000/15 min limiter instead.
- **step-up** — bearer plus `password` in the body (`requireStepUpPassword`; internal identity refused).
  Go only: 10 wrong passwords per user and client address in 15 minutes, then 429 with `Retry-After`
  (shared with `/api/system/execute`; a correct password resets the count).
- **sse** — `sseAuthMiddleware`: `?token=` with `purpose: "sse"` only (Bearer and internal secret not accepted).
- **bearer|setup** — `X-Setup-Token` (valid, before first admin) or bearer + step-up. Go (deliberate fix): registering the first admin deletes `.setup-token` (also deleted at boot when users exist), and the guard refuses the token once any user exists (403) or when the user count fails (503) before taking the restore lock or reading the upload.
- **terminal** — WebSocket: trusted `Origin` (else 403) then `?token=` with `purpose: "terminal"` (else 401).
  Go only: the token is single-use (a second upgrade with it is 401; terminal tokens carry a random `jti`),
  the password step must complete within 60 s, and open sessions and SSE streams end when the user's
  password changes (at once via change-password, within 30 s after `docklift reset-password`).

| # | Method | Path | Auth | TS source | Go handler (server/internal/) | Covered by |
|---|---|---|---|---|---|---|
| 1 | GET | /api/health | public | index.ts:151 | api/health/health.go `Mount` | TestHealth, TestSecurityHeaders, TestCORS, TestCriticalRestoreGate |
| 2 | GET | /api/auth/status | public (not limited) | routes/auth.ts:53 | api/auth/auth.go:70 `status` | TestAuthStatusAfterSetup, TestFreshInstallFlow, TestRateLimitHeaders |
| 3 | POST | /api/auth/register | public+setup | routes/auth.ts:81 | api/auth/auth.go:110 `register` | TestRegisterAfterSetup, TestFreshInstallFlow, TestSetupRateLimit |
| 4 | POST | /api/auth/login | public+limit | routes/auth.ts:166 | api/auth/auth.go:201 `login` | TestLogin, TestRequestBodies, TestRateLimitHeaders, TestAuthRateLimitExhaustion |
| 5 | GET | /api/auth/me | bearer+limit | routes/auth.ts:206 | api/auth/auth.go:246 `me` | TestMe, TestSelfSignedTokens, TestInternalSecretHeader, TestFreshInstallFlow |
| 6 | PATCH | /api/auth/profile | bearer+limit | routes/auth.ts:233 | api/auth/auth.go:263 `profile` | TestFreshInstallFlow/profile_update |
| 7 | POST | /api/auth/change-password | bearer+limit | routes/auth.ts:267 | api/auth/auth.go:297 `changePassword` | TestFreshInstallFlow/change_password_invalidates_older_tokens |
| 8 | GET | /api/auth/setup-token | public+setup | routes/auth.ts:315 | api/auth/auth.go:354 `setupToken` | TestFreshInstallFlow, TestRegisterAfterSetup, TestSetupRateLimit |
| 9 | POST | /api/auth/sse-token | bearer+limit | routes/auth.ts:343 | api/auth/auth.go:382 `purposeToken` | TestPurposeTokens, TestContainerLogs, TestSystemLogStreams |
| 10 | POST | /api/auth/terminal-token | bearer+limit | routes/auth.ts:371 | api/auth/auth.go:382 `purposeToken` | TestPurposeTokens, TestTerminalWebSocket |
| 11 | GET | /api/backup | bearer | routes/backup.ts:378 | api/backup/backup.go:179 `list` | TestBackupLifecycle, TestBackupFilenameValidation |
| 12 | GET | /api/backup/uploads | bearer | routes/backup.ts:414 | api/backup/backup.go:190 `listUploads` | TestBackupUploads, TestFreshInstallFlow, TestAuthenticatedRestore |
| 13 | DELETE | /api/backup/uploads/:filename | bearer | routes/backup.ts:447 | api/backup/backup.go:219 `deleteUpload` | TestBackupUploads, TestBackupFilenameValidation |
| 14 | POST | /api/backup/create | bearer | routes/backup.ts:470 | api/backup/backup.go:306 `create` | TestBackupLifecycle |
| 15 | POST | /api/backup/restore/:filename | step-up | routes/backup.ts:621 | api/backup/restore.go:65 `restoreFromBackup` | TestBackupRestoreValidation, TestAuthenticatedRestore (lock), TestCriticalRestoreGate |
| 16 | DELETE | /api/backup/:filename | bearer | routes/backup.ts:835 | api/backup/backup.go:243 `deleteBackup` | TestBackupLifecycle, TestBackupFilenameValidation |
| 17 | GET | /api/backup/download/:filename | bearer | routes/backup.ts:859 | api/backup/backup.go:267 `download` | TestBackupLifecycle, TestBackupFilenameValidation |
| 18 | POST | /api/backup/upload | bearer | routes/backup.ts:895 | api/backup/backup.go:330 `upload` | TestBackupUploads, TestAuthenticatedRestore |
| 19 | POST | /api/backup/restore-upload | bearer\|setup | routes/backup.ts:914 (gate index.ts:207) | api/backup/restore.go:119 `restoreUpload` | TestFreshInstallFlow, TestBackupRestoreValidation, TestAuthenticatedRestore (lock), TestCriticalRestoreGate |
| 20 | POST | /api/backup/restore-from-upload/:filename | step-up | routes/backup.ts:1175 | api/backup/restore.go:90 `restoreFromUpload` | TestAuthenticatedRestore, TestBackupRestoreValidation |
| 21 | POST | /api/backup/clear-critical-restore | step-up (critical-gate exempt) | routes/backup.ts:1407 | api/backup/backup.go:375 `clearCritical` | TestCriticalRestoreGate, TestBackupFilenameValidation |
| 22 | GET | /api/backup/critical-status | bearer (critical-gate exempt) | routes/backup.ts:1424 | api/backup/backup.go:366 `criticalStatus` | TestCriticalRestoreGate, TestBackupFilenameValidation |
| 23 | GET | /api/databases/engines | bearer | routes/databases.ts:39 | api/databases/databases.go:61 `engines` | TestDatabaseEngines |
| 24 | GET | /api/databases | bearer | routes/databases.ts:50 | api/databases/databases.go:67 `list` | TestSeeded/managed_database_listing_and_connection, TestDatabaseValidationWithoutDocker |
| 25 | GET | /api/databases/links/by-app/:appProjectId | bearer | routes/databases.ts:72 | api/databases/databases.go:78 `linksByApp` | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 26 | POST | /api/databases | bearer | routes/databases.ts:95 | api/databases/databases.go:118 `create` | TestDatabaseValidationWithoutDocker (+ docker: create_with_docker) |
| 27 | GET | /api/databases/:id/connection | bearer | routes/databases.ts:242 | api/databases/databases.go:325 `connection` | TestSeeded/managed_database_listing_and_connection, TestDatabaseValidationWithoutDocker |
| 28 | GET | /api/databases/:id/links | bearer | routes/databases.ts:279 | api/databases/databases.go:384 `links` | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 29 | POST | /api/databases/:id/links | bearer | routes/databases.ts:306 | api/databases/databases.go:420 `createLink` | TestSeeded/database_links, TestSeeded/project_reads_resync_status_from_docker |
| 30 | DELETE | /api/databases/:id/links/:linkId | bearer | routes/databases.ts:437 | api/databases/databases.go:560 `deleteLink` | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 31 | GET | /api/deployments/:projectId | bearer | routes/deployments.ts:335 | api/deployments/deployments.go:60 `list` | TestDeploymentHistory, TestSeeded/rollback_guards |
| 32 | GET | /api/deployments/:projectId/services | bearer | routes/deployments.ts:372 | api/deployments/deployments.go:126 `services` | TestDeployWithoutDocker, TestServiceDomainsAndSSL |
| 33 | GET | /api/deployments/:projectId/services/:serviceId/ssl | bearer | routes/deployments.ts:401 | api/deployments/domains.go:47 `serviceSSL` | TestServiceDomainsAndSSL |
| 34 | POST | /api/deployments/:projectId/services/:serviceId/dns-check | bearer | routes/deployments.ts:419 | api/deployments/domains.go:69 `dnsCheck` | TestServiceDomainsAndSSL (lookups need DOCKLIFT_NETWORK=1) |
| 35 | POST | /api/deployments/:projectId/services/:serviceId/ssl/retry | bearer | routes/deployments.ts:444 | api/deployments/domains.go:113 `sslRetry` | TestServiceDomainsAndSSL |
| 36 | PUT | /api/deployments/:projectId/services/:serviceId | bearer | routes/deployments.ts:477 | api/deployments/domains.go:165 `updateService` | TestServiceDomainsAndSSL, TestPanelDomains |
| 37 | POST | /api/deployments/:projectId/deploy | bearer | routes/deployments.ts:1548 | api/deployments/deployments.go:230 `deploy` | TestDeployWithoutDocker, TestDeployInProgressConflicts, TestPorts, TestSeeded/webhook_triggers… (+ docker) |
| 38 | POST | /api/deployments/:projectId/stop | bearer | routes/deployments.ts:1551 | api/deployments/deployments.go:269 `stop` | TestStopRestartCancelWithoutRuntime, TestDeployInProgressConflicts (+ docker) |
| 39 | POST | /api/deployments/:projectId/restart | bearer | routes/deployments.ts:1711 | api/deployments/deployments.go:273 `restart` | TestStopRestartCancelWithoutRuntime, TestDeployInProgressConflicts (+ docker) |
| 40 | POST | /api/deployments/:projectId/rollback | step-up | routes/deployments.ts:1836 | api/deployments/deployments.go:287 `rollback` | TestDeploymentNotFoundAndValidation, TestSeeded/rollback_guards, TestStepUpPasswordThrottle (+ docker) |
| 41 | POST | /api/deployments/:projectId/redeploy | bearer | routes/deployments.ts:2154 | api/deployments/deployments.go:230 `deploy` | TestDeployWithoutDocker/redeploy…, TestDeployInProgressConflicts |
| 42 | POST | /api/deployments/:projectId/cancel | bearer | routes/deployments.ts:2157 | api/deployments/deployments.go:277 `cancel` | TestStopRestartCancelWithoutRuntime/idle_cancel, TestDeploymentHistory |
| 43 | GET | /api/deployments/:projectId/logs | bearer | routes/deployments.ts:2357 | api/deployments/deployments.go:166 `logs` | TestDeploymentLogsAndStats, TestDeploymentNotFoundAndValidation (+ docker) |
| 44 | GET | /api/deployments/:projectId/stats | bearer | routes/deployments.ts:2380 | api/deployments/deployments.go:180 `stats` | TestDeploymentLogsAndStats, TestDeploymentNotFoundAndValidation (+ docker) |
| 45 | GET | /api/domains/ssl/email | bearer | routes/domains.ts:88 | api/domains/domains.go:145 `getEmail` | TestACMEEmail, TestACMEEmailDefaultsToAdmin |
| 46 | PUT | /api/domains/ssl/email | bearer | routes/domains.ts:98 | api/domains/domains.go:156 `putEmail` | TestACMEEmail |
| 47 | GET | /api/domains | bearer | routes/domains.ts:109 | api/domains/domains.go:190 `list` | TestPanelDomains |
| 48 | POST | /api/domains | bearer | routes/domains.ts:144 | api/domains/domains.go:254 `add` | TestPanelDomains |
| 49 | POST | /api/domains/dns-check | bearer | routes/domains.ts:180 | api/domains/domains.go:295 `dnsCheck` | TestPanelDomains (lookups need DOCKLIFT_NETWORK=1) |
| 50 | GET | /api/domains/:domain/ssl | bearer | routes/domains.ts:202 | api/domains/domains.go:335 `sslStatus` | TestPanelDomains |
| 51 | POST | /api/domains/:domain/ssl/retry | bearer | routes/domains.ts:221 | api/domains/domains.go:361 `sslRetry` | TestPanelDomains |
| 52 | DELETE | /api/domains/:domain | bearer | routes/domains.ts:248 | api/domains/domains.go:384 `remove` | TestPanelDomains |
| 53 | GET | /api/files/:projectId | bearer | routes/files.ts:14 | api/files/files.go:116 `tree` | TestFilesTree, TestFilesSymlinks, TestFilesDirLinkEscape |
| 54 | GET | /api/files/:projectId/content | bearer | routes/files.ts:34 | api/files/files.go:283 `read` | TestFilesReadWrite, TestFilesPathSecurity, TestFilesSymlinks, TestFilesDirLinkEscape |
| 55 | PUT | /api/files/:projectId/content | bearer | routes/files.ts:78 | api/files/files.go:315 `write` | TestFilesReadWrite, TestFilesPathSecurity, TestFilesSymlinks, TestFilesDirLinkEscape |
| 56 | POST | /api/github/manifest | bearer | routes/github.ts:315 | api/github/github.go:217 `manifest` | TestGithubManifestAndSetupState |
| 57 | GET | /api/github/manifest/callback | public | routes/github.ts:373 | api/github/github.go:269 `manifestCallback` | TestGithubManifestAndSetupState (code exchange needs DOCKLIFT_NETWORK=1); Go: query `state` must equal the state cookie (TS: either) |
| 58 | GET | /api/github/app-status | bearer | routes/github.ts:456 | api/github/github.go:370 `appStatus` | TestGithubUnconfigured, TestSeeded/github_app_configured_state, TestSeeded/github_disconnect |
| 59 | POST | /api/github/check-installation | bearer | routes/github.ts:483 | api/github/github.go:415 `checkInstallation` | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 60 | POST | /api/github/install-session | bearer | routes/github.ts:693 | api/github/github.go:532 `installSession` | TestGithubUnconfigured, TestSeeded/github_app_configured_state; Go: `installUrl` carries `?state=` |
| 61 | GET | /api/github/install | bearer | routes/github.ts:707 | api/github/github.go:553 `install` | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 62 | GET | /api/github/setup | public | routes/github.ts:721 | api/github/github.go:568 `setup` | TestGithubManifestAndSetupState, TestSeeded/github_app_configured_state; Go: query `state` must equal the state cookie (TS: either) |
| 63 | GET | /api/github/callback | public | routes/github.ts:778 | api/github/github.go:645 `callback` | TestGithubManifestAndSetupState/legacy_callback |
| 64 | GET | /api/github/status | bearer | routes/github.ts:792 | api/github/github.go:659 `status` | TestGithubUnconfigured, TestSeeded/github_app_configured_state, TestSeeded/github_disconnect |
| 65 | GET | /api/github/installations | bearer | routes/github.ts:839 | api/github/github.go:709 `installations` | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 66 | GET | /api/github/repos | bearer | routes/github.ts:862 | api/github/repos.go:169 `repos` | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 67 | GET | /api/github/branches | bearer | routes/github.ts:1073 | api/github/repos.go:289 `refs` | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 68 | GET | /api/github/tags | bearer | routes/github.ts:1110 | api/github/repos.go:289 `refs` | TestGithubUnconfigured |
| 69 | POST | /api/github/disconnect | bearer | routes/github.ts:1155 | api/github/github.go:747 `disconnect` | TestGithubUnconfigured, TestSeeded/github_disconnect |
| 70 | POST | /api/github/webhook | public (HMAC for push) | routes/github.ts:1191 | api/github/webhook.go:87 `webhook` | TestGithubWebhookWithoutSecret, TestSeeded/webhook_* (3 subtests), TestCriticalRestoreGate |
| 71 | GET | /api/logs/:projectId/containers | bearer | routes/logs.ts:10 | api/logs/logs.go:83 `containers` | TestContainerLogs (+ docker) |
| 72 | GET | /api/logs/:projectId/stream/:containerName | sse | routes/logs.ts:54 | api/logs/logs.go:147 `stream` | TestContainerLogs |
| 73 | GET | /api/ports | bearer | routes/ports.ts:10 | api/ports/ports.go:100 `list` | TestPorts, TestInternalSecretHeader |
| 74 | DELETE | /api/ports/:port | bearer | routes/ports.ts:98 | api/ports/ports.go:173 `release` | TestPorts |
| 75 | GET | /api/projects | bearer | routes/projects.ts:100 | api/projects/projects.go:179 `list` | TestProjectReadUpdate, TestSeeded/project_reads_resync_status_from_docker |
| 76 | GET | /api/projects/:id | bearer | routes/projects.ts:147 | api/projects/projects.go:195 `get` | TestProjectReadUpdate, TestDeployWithoutDocker, TestPorts |
| 77 | POST | /api/projects | bearer | routes/projects.ts:195 | api/projects/create.go:218 `create` | TestProjectCreateValidation, TestProjectCreateUnsafeZip, TestProjectCreateUploadFields; Go: `http://` GitHub URLs refused |
| 78 | PATCH | /api/projects/:id | bearer | routes/projects.ts:366 | api/projects/projects.go:257 `update` | TestProjectReadUpdate, TestPorts; Go: `github_url` validated as on create |
| 79 | GET | /api/projects/:id/build/detect | bearer | routes/projects.ts:418 | api/projects/storage.go:64 `detect` | TestProjectBuildDetect, TestDeploymentNotFoundAndValidation; Go: a scanned service named `docklift*` is refused (400) |
| 80 | GET | /api/projects/:id/storage | bearer | routes/projects.ts:462 | api/projects/storage.go:97 `listStorage` | TestProjectStorage (+ docker: TestDockerStorage) |
| 81 | POST | /api/projects/:id/storage | bearer | routes/projects.ts:474 | api/projects/storage.go:109 `createStorage` | TestProjectStorage (+ docker: TestDockerStorage) |
| 82 | DELETE | /api/projects/:id/storage/:storageId | bearer | routes/projects.ts:525 | api/projects/storage.go:174 `deleteStorage` | TestProjectStorage (+ docker: TestDockerStorage) |
| 83 | DELETE | /api/projects/:id | bearer | routes/projects.ts:551 | api/projects/projects.go:320 `remove` | TestProjectDelete, TestDeployInProgressConflicts (+ docker); Go: compose runs outside the checkout and only `docklift-<id8>-*` images are removed (no `--rmi all`) |
| 84 | GET | /api/projects/:id/env | bearer | routes/projects.ts:690 | api/projects/env.go:80 `listEnv` | TestEnvVariables, TestSeeded/database_links |
| 85 | POST | /api/projects/:id/env | bearer | routes/projects.ts:709 | api/projects/env.go:97 `createEnv` | TestEnvVariables, TestSeeded/managed_credential_env_is_locked |
| 86 | POST | /api/projects/:id/env/bulk | bearer | routes/projects.ts:758 | api/projects/env.go:176 `bulkEnv` | TestEnvVariables, TestSeeded/managed_credential_env_is_locked |
| 87 | DELETE | /api/projects/:id/env/:envId | bearer | routes/projects.ts:843 | api/projects/env.go:237 `deleteEnv` | TestEnvVariables, TestSeeded/managed_credential_env_is_locked, TestSeeded/database_links |
| 88 | PATCH | /api/projects/:id/auto-deploy | bearer | routes/projects.ts:887 | api/projects/projects.go:362 `setAutoDeploy` | TestProjectAutoDeployUpload, TestSeeded/github_auto-deploy_toggles |
| 89 | GET | /api/projects/:id/auto-deploy | bearer | routes/projects.ts:932 | api/projects/projects.go:414 `getAutoDeploy` | TestProjectAutoDeployUpload, TestSeeded/github_auto-deploy_toggles |
| 90 | GET | /api/system/stats | bearer | routes/system.ts:522 | api/system/system.go:165 `statsHandler` | TestSystemStats, Test401Shapes |
| 91 | GET | /api/system/logs/:service | sse | routes/system.ts:535 (gate index.ts:192) | api/system/system.go:176 `logStream` | TestSystemLogStreams, TestSelfSignedTokens, TestInternalSecretHeader |
| 92 | GET | /api/system/quick | bearer | routes/system.ts:557 | api/system/system.go:191 `quick` | TestSystemStats |
| 93 | GET | /api/system/ip | bearer | routes/system.ts:575 | api/system/system.go:202 `ip` | TestSystemStats |
| 94 | POST | /api/system/purge | step-up | routes/system.ts:593 | api/system/purge.go:74 `purge` | TestSystemDangerousEndpointsRequireStepUp, TestSeeded/purge_without_docker |
| 95 | POST | /api/system/reboot | step-up | routes/system.ts:734 | api/system/system.go:250 `reboot` | TestSystemDangerousEndpointsRequireStepUp, TestFreshInstallFlow (rejection only) |
| 96 | POST | /api/system/reset | step-up | routes/system.ts:777 | api/system/system.go:270 `reset` | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 97 | POST | /api/system/execute | bearer + password | routes/system.ts:817 | api/system/system.go:388 `execute` | TestSystemExecute, TestInternalSecretHeader, TestStepUpPasswordThrottle |
| 98 | GET | /api/system/version | bearer | routes/system.ts:983 | api/system/system.go:209 `versionHandler` | TestSystemStats, TestHealth |
| 99 | POST | /api/system/update-system | step-up | routes/system.ts:994 | api/system/system.go:337 `updateSystem` | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 100 | POST | /api/system/upgrade | step-up | routes/system.ts:1035 | api/system/system.go:357 `upgrade` | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 101 | GET (upgrade) | /ws/terminal | terminal | services/terminal.ts:95 | api/terminal/terminal.go (Attach) | TestTerminalWebSocket, TestFreshInstallFlow (revoked terminal token) |

Cross-cutting behaviour (index.ts) — security headers (66), CORS (81), body parsers/limits (98-104),
restore-critical and maintenance gate (126), rate limiters (47, 57, 174-176), mount-level auth (179-222),
error handler (225), Express 404 — is covered by TestSecurityHeaders, TestCORS, TestRequestBodies,
TestUnknownAPIPath, Test401Shapes, TestCriticalRestoreGate, TestAuthenticatedRestore, TestRateLimitHeaders,
TestSetupRateLimit and TestAuthRateLimitExhaustion.

Go only: bodies are parsed inside each group after its rate limiter and auth guard (TS parsed every
body first). Unauthenticated requests to a guarded group get 401 without the body being read;
`/api/auth/*` and the public GitHub callbacks accept JSON/urlencoded bodies up to 64 KB (413 above);
the webhook reads its raw body (10 MB) only when it carries a signature and decodes it only after
the HMAC matches; authenticated routes keep the 10 MB limit, at most 100 000 JSON values, and a
shared in-flight budget (503 + `Retry-After` when saturated).
