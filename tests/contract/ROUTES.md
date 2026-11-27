# Docklift HTTP routes and contract coverage

Derived by reading `backend/src/index.ts` and every router in `backend/src/routes/`.
Count check: `grep -cE "router\.(get|post|put|patch|delete)\(" backend/src/routes/*.ts` sums to **99**
(auth 9, backup 12, databases 8, deployments 14, domains 8, files 3, github 15, logs 2, ports 2,
projects 15, system 11). `backup.ts:914` is the one multi-line `router.post(` (`/restore-upload`).
Plus `GET /api/health` (index.ts) and the `/ws/terminal` upgrade (services/terminal.ts): **101 entries**.

Auth types:

- **public** — no credentials.
- **public+limit** — public, behind the 100/15 min auth limiter (`/status` is skipped).
- **public+setup** — public, behind both the 10/15 min setup limiter and the auth limiter; needs the bootstrap secret.
- **bearer** — `authMiddleware`: `Authorization: Bearer <session JWT>` (purpose `sse`/`terminal` refused, `pwdv` checked) or `X-Internal-Secret`.
- **bearer+limit** — bearer, inside `/api/auth` (auth limiter applies).
- **step-up** — bearer plus `password` in the body (`requireStepUpPassword`; internal identity refused).
- **sse** — `sseAuthMiddleware`: `?token=` with `purpose: "sse"` only (Bearer and internal secret not accepted).
- **bearer|setup** — `X-Setup-Token` (valid, before first admin) or bearer + step-up.
- **terminal** — WebSocket: trusted `Origin` (else 403) then `?token=` with `purpose: "terminal"` (else 401).

| # | Method | Path | Auth | TS source | Covered by |
|---|---|---|---|---|---|
| 1 | GET | /api/health | public | index.ts:151 | TestHealth, TestSecurityHeaders, TestCORS, TestCriticalRestoreGate |
| 2 | GET | /api/auth/status | public (not limited) | routes/auth.ts:53 | TestAuthStatusAfterSetup, TestFreshInstallFlow, TestRateLimitHeaders |
| 3 | POST | /api/auth/register | public+setup | routes/auth.ts:81 | TestRegisterAfterSetup, TestFreshInstallFlow, TestSetupRateLimit |
| 4 | POST | /api/auth/login | public+limit | routes/auth.ts:166 | TestLogin, TestRequestBodies, TestRateLimitHeaders, TestAuthRateLimitExhaustion |
| 5 | GET | /api/auth/me | bearer+limit | routes/auth.ts:206 | TestMe, TestSelfSignedTokens, TestInternalSecretHeader, TestFreshInstallFlow |
| 6 | PATCH | /api/auth/profile | bearer+limit | routes/auth.ts:233 | TestFreshInstallFlow/profile_update |
| 7 | POST | /api/auth/change-password | bearer+limit | routes/auth.ts:267 | TestFreshInstallFlow/change_password_invalidates_older_tokens |
| 8 | GET | /api/auth/setup-token | public+setup | routes/auth.ts:315 | TestFreshInstallFlow, TestRegisterAfterSetup, TestSetupRateLimit |
| 9 | POST | /api/auth/sse-token | bearer+limit | routes/auth.ts:343 | TestPurposeTokens, TestContainerLogs, TestSystemLogStreams |
| 10 | POST | /api/auth/terminal-token | bearer+limit | routes/auth.ts:371 | TestPurposeTokens, TestTerminalWebSocket |
| 11 | GET | /api/backup | bearer | routes/backup.ts:378 | TestBackupLifecycle, TestBackupFilenameValidation |
| 12 | GET | /api/backup/uploads | bearer | routes/backup.ts:414 | TestBackupUploads, TestFreshInstallFlow, TestAuthenticatedRestore |
| 13 | DELETE | /api/backup/uploads/:filename | bearer | routes/backup.ts:447 | TestBackupUploads, TestBackupFilenameValidation |
| 14 | POST | /api/backup/create | bearer | routes/backup.ts:470 | TestBackupLifecycle |
| 15 | POST | /api/backup/restore/:filename | step-up | routes/backup.ts:621 | TestBackupRestoreValidation, TestAuthenticatedRestore (lock), TestCriticalRestoreGate |
| 16 | DELETE | /api/backup/:filename | bearer | routes/backup.ts:835 | TestBackupLifecycle, TestBackupFilenameValidation |
| 17 | GET | /api/backup/download/:filename | bearer | routes/backup.ts:859 | TestBackupLifecycle, TestBackupFilenameValidation |
| 18 | POST | /api/backup/upload | bearer | routes/backup.ts:895 | TestBackupUploads, TestAuthenticatedRestore |
| 19 | POST | /api/backup/restore-upload | bearer\|setup | routes/backup.ts:914 (gate index.ts:207) | TestFreshInstallFlow, TestBackupRestoreValidation, TestAuthenticatedRestore (lock), TestCriticalRestoreGate |
| 20 | POST | /api/backup/restore-from-upload/:filename | step-up | routes/backup.ts:1175 | TestAuthenticatedRestore, TestBackupRestoreValidation |
| 21 | POST | /api/backup/clear-critical-restore | step-up (critical-gate exempt) | routes/backup.ts:1407 | TestCriticalRestoreGate, TestBackupFilenameValidation |
| 22 | GET | /api/backup/critical-status | bearer (critical-gate exempt) | routes/backup.ts:1424 | TestCriticalRestoreGate, TestBackupFilenameValidation |
| 23 | GET | /api/databases/engines | bearer | routes/databases.ts:39 | TestDatabaseEngines |
| 24 | GET | /api/databases | bearer | routes/databases.ts:50 | TestSeeded/managed_database_listing_and_connection, TestDatabaseValidationWithoutDocker |
| 25 | GET | /api/databases/links/by-app/:appProjectId | bearer | routes/databases.ts:72 | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 26 | POST | /api/databases | bearer | routes/databases.ts:95 | TestDatabaseValidationWithoutDocker (+ docker: create_with_docker) |
| 27 | GET | /api/databases/:id/connection | bearer | routes/databases.ts:242 | TestSeeded/managed_database_listing_and_connection, TestDatabaseValidationWithoutDocker |
| 28 | GET | /api/databases/:id/links | bearer | routes/databases.ts:279 | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 29 | POST | /api/databases/:id/links | bearer | routes/databases.ts:306 | TestSeeded/database_links, TestSeeded/project_reads_resync_status_from_docker |
| 30 | DELETE | /api/databases/:id/links/:linkId | bearer | routes/databases.ts:437 | TestSeeded/database_links, TestDatabaseValidationWithoutDocker |
| 31 | GET | /api/deployments/:projectId | bearer | routes/deployments.ts:335 | TestDeploymentHistory, TestSeeded/rollback_guards |
| 32 | GET | /api/deployments/:projectId/services | bearer | routes/deployments.ts:372 | TestDeployWithoutDocker, TestServiceDomainsAndSSL |
| 33 | GET | /api/deployments/:projectId/services/:serviceId/ssl | bearer | routes/deployments.ts:401 | TestServiceDomainsAndSSL |
| 34 | POST | /api/deployments/:projectId/services/:serviceId/dns-check | bearer | routes/deployments.ts:419 | TestServiceDomainsAndSSL (lookups need DOCKLIFT_NETWORK=1) |
| 35 | POST | /api/deployments/:projectId/services/:serviceId/ssl/retry | bearer | routes/deployments.ts:444 | TestServiceDomainsAndSSL |
| 36 | PUT | /api/deployments/:projectId/services/:serviceId | bearer | routes/deployments.ts:477 | TestServiceDomainsAndSSL, TestPanelDomains |
| 37 | POST | /api/deployments/:projectId/deploy | bearer | routes/deployments.ts:1548 | TestDeployWithoutDocker, TestDeployInProgressConflicts, TestPorts, TestSeeded/webhook_triggers… (+ docker) |
| 38 | POST | /api/deployments/:projectId/stop | bearer | routes/deployments.ts:1551 | TestStopRestartCancelWithoutRuntime, TestDeployInProgressConflicts (+ docker) |
| 39 | POST | /api/deployments/:projectId/restart | bearer | routes/deployments.ts:1711 | TestStopRestartCancelWithoutRuntime, TestDeployInProgressConflicts (+ docker) |
| 40 | POST | /api/deployments/:projectId/rollback | step-up | routes/deployments.ts:1836 | TestDeploymentNotFoundAndValidation, TestSeeded/rollback_guards (+ docker) |
| 41 | POST | /api/deployments/:projectId/redeploy | bearer | routes/deployments.ts:2154 | TestDeployWithoutDocker/redeploy…, TestDeployInProgressConflicts |
| 42 | POST | /api/deployments/:projectId/cancel | bearer | routes/deployments.ts:2157 | TestStopRestartCancelWithoutRuntime/idle_cancel, TestDeploymentHistory |
| 43 | GET | /api/deployments/:projectId/logs | bearer | routes/deployments.ts:2357 | TestDeploymentLogsAndStats, TestDeploymentNotFoundAndValidation (+ docker) |
| 44 | GET | /api/deployments/:projectId/stats | bearer | routes/deployments.ts:2380 | TestDeploymentLogsAndStats, TestDeploymentNotFoundAndValidation (+ docker) |
| 45 | GET | /api/domains/ssl/email | bearer | routes/domains.ts:88 | TestACMEEmail, TestACMEEmailDefaultsToAdmin |
| 46 | PUT | /api/domains/ssl/email | bearer | routes/domains.ts:98 | TestACMEEmail |
| 47 | GET | /api/domains | bearer | routes/domains.ts:109 | TestPanelDomains |
| 48 | POST | /api/domains | bearer | routes/domains.ts:144 | TestPanelDomains |
| 49 | POST | /api/domains/dns-check | bearer | routes/domains.ts:180 | TestPanelDomains (lookups need DOCKLIFT_NETWORK=1) |
| 50 | GET | /api/domains/:domain/ssl | bearer | routes/domains.ts:202 | TestPanelDomains |
| 51 | POST | /api/domains/:domain/ssl/retry | bearer | routes/domains.ts:221 | TestPanelDomains |
| 52 | DELETE | /api/domains/:domain | bearer | routes/domains.ts:248 | TestPanelDomains |
| 53 | GET | /api/files/:projectId | bearer | routes/files.ts:14 | TestFilesTree, TestFilesSymlinks, TestFilesDirLinkEscape |
| 54 | GET | /api/files/:projectId/content | bearer | routes/files.ts:34 | TestFilesReadWrite, TestFilesPathSecurity, TestFilesSymlinks, TestFilesDirLinkEscape |
| 55 | PUT | /api/files/:projectId/content | bearer | routes/files.ts:78 | TestFilesReadWrite, TestFilesPathSecurity, TestFilesSymlinks, TestFilesDirLinkEscape |
| 56 | POST | /api/github/manifest | bearer | routes/github.ts:315 | TestGithubManifestAndSetupState |
| 57 | GET | /api/github/manifest/callback | public | routes/github.ts:373 | TestGithubManifestAndSetupState (code exchange needs DOCKLIFT_NETWORK=1) |
| 58 | GET | /api/github/app-status | bearer | routes/github.ts:456 | TestGithubUnconfigured, TestSeeded/github_app_configured_state, TestSeeded/github_disconnect |
| 59 | POST | /api/github/check-installation | bearer | routes/github.ts:483 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 60 | POST | /api/github/install-session | bearer | routes/github.ts:693 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 61 | GET | /api/github/install | bearer | routes/github.ts:707 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 62 | GET | /api/github/setup | public | routes/github.ts:721 | TestGithubManifestAndSetupState, TestSeeded/github_app_configured_state |
| 63 | GET | /api/github/callback | public | routes/github.ts:778 | TestGithubManifestAndSetupState/legacy_callback |
| 64 | GET | /api/github/status | bearer | routes/github.ts:792 | TestGithubUnconfigured, TestSeeded/github_app_configured_state, TestSeeded/github_disconnect |
| 65 | GET | /api/github/installations | bearer | routes/github.ts:839 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 66 | GET | /api/github/repos | bearer | routes/github.ts:862 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 67 | GET | /api/github/branches | bearer | routes/github.ts:1073 | TestGithubUnconfigured, TestSeeded/github_app_configured_state |
| 68 | GET | /api/github/tags | bearer | routes/github.ts:1110 | TestGithubUnconfigured |
| 69 | POST | /api/github/disconnect | bearer | routes/github.ts:1155 | TestGithubUnconfigured, TestSeeded/github_disconnect |
| 70 | POST | /api/github/webhook | public (HMAC for push) | routes/github.ts:1191 | TestGithubWebhookWithoutSecret, TestSeeded/webhook_* (3 subtests), TestCriticalRestoreGate |
| 71 | GET | /api/logs/:projectId/containers | bearer | routes/logs.ts:10 | TestContainerLogs (+ docker) |
| 72 | GET | /api/logs/:projectId/stream/:containerName | sse | routes/logs.ts:54 | TestContainerLogs |
| 73 | GET | /api/ports | bearer | routes/ports.ts:10 | TestPorts, TestInternalSecretHeader |
| 74 | DELETE | /api/ports/:port | bearer | routes/ports.ts:98 | TestPorts |
| 75 | GET | /api/projects | bearer | routes/projects.ts:100 | TestProjectReadUpdate, TestSeeded/project_reads_resync_status_from_docker |
| 76 | GET | /api/projects/:id | bearer | routes/projects.ts:147 | TestProjectReadUpdate, TestDeployWithoutDocker, TestPorts |
| 77 | POST | /api/projects | bearer | routes/projects.ts:195 | TestProjectCreateValidation, TestProjectCreateUnsafeZip, TestProjectCreateUploadFields |
| 78 | PATCH | /api/projects/:id | bearer | routes/projects.ts:366 | TestProjectReadUpdate, TestPorts |
| 79 | GET | /api/projects/:id/build/detect | bearer | routes/projects.ts:418 | TestProjectBuildDetect, TestDeploymentNotFoundAndValidation |
| 80 | GET | /api/projects/:id/storage | bearer | routes/projects.ts:462 | TestProjectStorage (+ docker: TestDockerStorage) |
| 81 | POST | /api/projects/:id/storage | bearer | routes/projects.ts:474 | TestProjectStorage (+ docker: TestDockerStorage) |
| 82 | DELETE | /api/projects/:id/storage/:storageId | bearer | routes/projects.ts:525 | TestProjectStorage (+ docker: TestDockerStorage) |
| 83 | DELETE | /api/projects/:id | bearer | routes/projects.ts:551 | TestProjectDelete, TestDeployInProgressConflicts (+ docker) |
| 84 | GET | /api/projects/:id/env | bearer | routes/projects.ts:690 | TestEnvVariables, TestSeeded/database_links |
| 85 | POST | /api/projects/:id/env | bearer | routes/projects.ts:709 | TestEnvVariables, TestSeeded/managed_credential_env_is_locked |
| 86 | POST | /api/projects/:id/env/bulk | bearer | routes/projects.ts:758 | TestEnvVariables, TestSeeded/managed_credential_env_is_locked |
| 87 | DELETE | /api/projects/:id/env/:envId | bearer | routes/projects.ts:843 | TestEnvVariables, TestSeeded/managed_credential_env_is_locked, TestSeeded/database_links |
| 88 | PATCH | /api/projects/:id/auto-deploy | bearer | routes/projects.ts:887 | TestProjectAutoDeployUpload, TestSeeded/github_auto-deploy_toggles |
| 89 | GET | /api/projects/:id/auto-deploy | bearer | routes/projects.ts:932 | TestProjectAutoDeployUpload, TestSeeded/github_auto-deploy_toggles |
| 90 | GET | /api/system/stats | bearer | routes/system.ts:522 | TestSystemStats, Test401Shapes |
| 91 | GET | /api/system/logs/:service | sse | routes/system.ts:535 (gate index.ts:192) | TestSystemLogStreams, TestSelfSignedTokens, TestInternalSecretHeader |
| 92 | GET | /api/system/quick | bearer | routes/system.ts:557 | TestSystemStats |
| 93 | GET | /api/system/ip | bearer | routes/system.ts:575 | TestSystemStats |
| 94 | POST | /api/system/purge | step-up | routes/system.ts:593 | TestSystemDangerousEndpointsRequireStepUp, TestSeeded/purge_without_docker |
| 95 | POST | /api/system/reboot | step-up | routes/system.ts:734 | TestSystemDangerousEndpointsRequireStepUp, TestFreshInstallFlow (rejection only) |
| 96 | POST | /api/system/reset | step-up | routes/system.ts:777 | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 97 | POST | /api/system/execute | bearer + password | routes/system.ts:817 | TestSystemExecute, TestInternalSecretHeader |
| 98 | GET | /api/system/version | bearer | routes/system.ts:983 | TestSystemStats, TestHealth |
| 99 | POST | /api/system/update-system | step-up | routes/system.ts:994 | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 100 | POST | /api/system/upgrade | step-up | routes/system.ts:1035 | TestSystemDangerousEndpointsRequireStepUp (rejection only) |
| 101 | GET (upgrade) | /ws/terminal | terminal | services/terminal.ts:95 | TestTerminalWebSocket, TestFreshInstallFlow (revoked terminal token) |

Cross-cutting behaviour (index.ts) — security headers (66), CORS (81), body parsers/limits (98-104),
restore-critical and maintenance gate (126), rate limiters (47, 57, 174-176), mount-level auth (179-222),
error handler (225), Express 404 — is covered by TestSecurityHeaders, TestCORS, TestRequestBodies,
TestUnknownAPIPath, Test401Shapes, TestCriticalRestoreGate, TestAuthenticatedRestore, TestRateLimitHeaders,
TestSetupRateLimit and TestAuthRateLimitExhaustion.
