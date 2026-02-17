## [1.3.13](https://github.com/SSujitX/docklift/compare/v1.3.12...v1.3.13) (2026-02-17)


### Bug Fixes

* **auth:** allow query param tokens with purpose field for backward compatibility ([0a39654](https://github.com/SSujitX/docklift/commit/0a39654d6af1aab218f6a51ca99dad93ce2fd45d))
* **deployments:** prevent write errors when response stream ends ([b3b76ab](https://github.com/SSujitX/docklift/commit/b3b76abc9d0f77ce6d83265c396668248e13d56f))
* **domains:** enforce consistent domain validation across endpoints ([d19c13f](https://github.com/SSujitX/docklift/commit/d19c13fcdda98ef8b682d3656af00251c6ef5835))
* **files:** add project ID validation and improve path traversal protection ([90f5cd7](https://github.com/SSujitX/docklift/commit/90f5cd761e7f220e38363fab446dd51437c3640d))
* **projects:** handle upload directory creation and cleanup temp files ([287d884](https://github.com/SSujitX/docklift/commit/287d884533b115c1fd8586f2cf355d431d99b9c3))
* **security:** enhance CORS and restore endpoint security ([1205159](https://github.com/SSujitX/docklift/commit/1205159958baddd6d185e6f75c5315655a9e6c8a))
* update Next.js routes type import path ([4dd89eb](https://github.com/SSujitX/docklift/commit/4dd89eb71b62f69f101bba1309c9b94caf3023d3))


### Features

* **auth:** add setup token for fresh install and short-lived SSE token ([cb62d80](https://github.com/SSujitX/docklift/commit/cb62d8055e1f4744fc4b727268fc80213a297afa))
* **logs:** fetch short-lived SSE token for container logs ([f731c0e](https://github.com/SSujitX/docklift/commit/f731c0ed4fc3a1832ca9c481ae3ec8769410959a))
* **system-logs:** implement SSE token auth with exponential backoff ([acdcc80](https://github.com/SSujitX/docklift/commit/acdcc802c019b9149c6a5b8a5c1ae9525f2d501b))
* **system:** add audit logging and password verification for execute endpoint ([6bf8977](https://github.com/SSujitX/docklift/commit/6bf8977e58dab29d917ef5fa27dd0200180ddd32))
* **terminal:** add password verification for command execution ([b002974](https://github.com/SSujitX/docklift/commit/b002974374ee5dc194df75d3b1455a846b14b7ca))

## [1.3.12](https://github.com/SSujitX/docklift/compare/v1.3.11...v1.3.12) (2026-02-17)


### Features

* add deployments routes for project build, deploy, service management, and logging. ([06698e3](https://github.com/SSujitX/docklift/commit/06698e3826e7cfd007156ef1a12bc999acea8bd2))
* Document the automated release process in a new skill and update README to reflect semantic-release usage. ([64caccf](https://github.com/SSujitX/docklift/commit/64caccf8f3e73476433eb58b12d3f7f0a4c737ad))

## [1.3.11](https://github.com/SSujitX/docklift/compare/v1.3.10...v1.3.11) (2026-02-16)


### Bug Fixes

* **backup:** specify project name in docker compose up during restore ([1f5bd1c](https://github.com/SSujitX/docklift/commit/1f5bd1c39d1e560abb2069b3f1893e9eb920c10f))
* correct import path and icon props for Next.js build ([016fa09](https://github.com/SSujitX/docklift/commit/016fa0964e4e5d0f2a36c5c9bc0bce3509eefabb))
* **deployments:** pass branch parameter to pullRepo function ([8caa8e7](https://github.com/SSujitX/docklift/commit/8caa8e748df8a1e0ea1080eb65a14dbb10b66b3d))
* **docker:** prevent write-after-end crashes in container log streaming ([b9b3c52](https://github.com/SSujitX/docklift/commit/b9b3c52aa105e791420b4ed839de98f04132c26a))
* **frontend:** improve SSE URL handling in SystemLogsPanel for production environments ([a371253](https://github.com/SSujitX/docklift/commit/a3712534e5fb02cab9ed384e82e6c4d90959e83e))
* **frontend:** limit container logs to 5000 lines and fix SSE URL ([f3b761e](https://github.com/SSujitX/docklift/commit/f3b761e3c5f13c24540b053f424b817ffb325530))
* **frontend:** resolved LucideProps type error and added logs page ([4c650ca](https://github.com/SSujitX/docklift/commit/4c650ca71ebd865bb9a52e224d7282364987b16a))
* **frontend:** use relative SSE paths in production to avoid buffering ([c0d08cd](https://github.com/SSujitX/docklift/commit/c0d08cd797fdb84174b1ad1853e1802a06e87619))
* **git:** replace git pull with fetch+reset for reliable sync ([6c6224f](https://github.com/SSujitX/docklift/commit/6c6224f872c33d42a6b005b854c22db1992447f2))


### Features

* add container logs API endpoints for real-time streaming ([ab1083b](https://github.com/SSujitX/docklift/commit/ab1083bcf456f852ddc8be995c4e1cc71634322b))
* add logs API endpoint with authentication middleware ([9c8fd07](https://github.com/SSujitX/docklift/commit/9c8fd074945d2bf6c67325fb2e412de34efa934b))
* add logs page navigation to header ([61b0149](https://github.com/SSujitX/docklift/commit/61b014998c26c18a86a640958df43caae20aef44))
* add version checker component to root layout ([087a8fb](https://github.com/SSujitX/docklift/commit/087a8fb0d715733828b48c33d4c3c5d8ac02589f))
* **backup:** automate system reconciliation after restore ([b96511e](https://github.com/SSujitX/docklift/commit/b96511e7c9bd658a80014b0ef7caa30807e9d006))
* **docker:** add real-time container log streaming via SSE ([9b00f9f](https://github.com/SSujitX/docklift/commit/9b00f9f249e094b179308d39549d346de575effa))
* **frontend:** add system logs panel component for real-time service monitoring ([d5f4d1a](https://github.com/SSujitX/docklift/commit/d5f4d1afcc8ec4f7dfeaac4f812ffedb03930f6e))
* **frontend:** add version checker component for auto-refresh on deploy ([9adf817](https://github.com/SSujitX/docklift/commit/9adf81761f5c8165dd7bdcecaa25b111e6c25cf3))
* **frontend:** create logs page for real-time system logs monitoring ([cd914dd](https://github.com/SSujitX/docklift/commit/cd914dd80f53fb7b3bd007ea9e29193131a1533c))
* **health:** add version and instance ID to health endpoint ([77ecf3d](https://github.com/SSujitX/docklift/commit/77ecf3d80bd87b8d77bc0ab48eff884f95515760))
* **logs:** enhance log viewer with timestamps and improved UI ([2165389](https://github.com/SSujitX/docklift/commit/2165389af910d3043b454792a2fb8258151e2942))
* **logs:** increase default log tail lines to 5000 ([3f32d6b](https://github.com/SSujitX/docklift/commit/3f32d6b6a53f6ee9c4fb51031cc0f972d68b12ab))
* **logs:** introduce unified LogViewer component with search and enhanced UI ([ce551ef](https://github.com/SSujitX/docklift/commit/ce551ef284ccac6f43742daaf95346986f623d2d))
* **nginx:** add configuration for long-lived SSE log streams ([fbaf00a](https://github.com/SSujitX/docklift/commit/fbaf00aa12d0ab71b37608e50d6b9395e1a4e914))
* **nginx:** configure Nginx for Server-Sent Events by disabling buffering and caching ([edbbd62](https://github.com/SSujitX/docklift/commit/edbbd626a224e9f30f70bff97eab0ad63ac5ba27))
* **projects:** add real-time container logs viewer with ANSI support ([34a9f0d](https://github.com/SSujitX/docklift/commit/34a9f0d4d6242b65949a86c6fe15920b80e85a13))
* **system:** add real-time container logs endpoint via SSE ([f8d9aa1](https://github.com/SSujitX/docklift/commit/f8d9aa1630fb64009039c25634e3d5d1b38b3e1c))

## [1.3.10](https://github.com/SSujitX/docklift/compare/v1.3.9...v1.3.10) (2026-01-27)


### Bug Fixes

* **projects:** update GitHub URL regex to support dots in repo names ([ed99465](https://github.com/SSujitX/docklift/commit/ed99465c806c4a3a3dd06701d11a791334488cfb))

## [1.3.9](https://github.com/SSujitX/docklift/compare/v1.3.8...v1.3.9) (2026-01-27)


### Bug Fixes

* **deployments:** ensure consistent status updates for projects and services ([bcd6020](https://github.com/SSujitX/docklift/commit/bcd602061f67c965b79aed811ca4b1a940b51430))
* **deployments:** update project and services status consistently during operations ([e7ef986](https://github.com/SSujitX/docklift/commit/e7ef98637968d39312a90b1a9e438e229abf1363))
* **projects:** skip auto-sync during project builds ([694b9ec](https://github.com/SSujitX/docklift/commit/694b9ec89acacf84ea6da29738a4ced6d183be03))


### Features

* **deployment:** improve project status tracking and polling ([97779ea](https://github.com/SSujitX/docklift/commit/97779eaabe46438627ea0f52e3001998e9f42ae7))
* **deployments:** improve real-time deployment logs handling ([7c322ad](https://github.com/SSujitX/docklift/commit/7c322ad71745f3234fc3e286648bf1c771943096))
* **github:** fetch all repository pages for installations ([2c22365](https://github.com/SSujitX/docklift/commit/2c22365b5b0543b67077a90fa909c9934791f16a))
* Introduce agent skills for general development, database management, and Docker operations. ([01f1281](https://github.com/SSujitX/docklift/commit/01f128101d027cd74ff53da0d5f9f8995d50ae64))

# [1.4.0](https://github.com/SSujitX/docklift/compare/v1.3.8...v1.4.0) (2026-01-27)


### Bug Fixes

* **deployments:** ensure consistent status updates for projects and services ([bcd6020](https://github.com/SSujitX/docklift/commit/bcd602061f67c965b79aed811ca4b1a940b51430))
* **deployments:** update project and services status consistently during operations ([e7ef986](https://github.com/SSujitX/docklift/commit/e7ef98637968d39312a90b1a9e438e229abf1363))
* **projects:** skip auto-sync during project builds ([694b9ec](https://github.com/SSujitX/docklift/commit/694b9ec89acacf84ea6da29738a4ced6d183be03))


### Features

* **deployment:** improve project status tracking and polling ([97779ea](https://github.com/SSujitX/docklift/commit/97779eaabe46438627ea0f52e3001998e9f42ae7))
* **deployments:** improve real-time deployment logs handling ([7c322ad](https://github.com/SSujitX/docklift/commit/7c322ad71745f3234fc3e286648bf1c771943096))
* **github:** fetch all repository pages for installations ([2c22365](https://github.com/SSujitX/docklift/commit/2c22365b5b0543b67077a90fa909c9934791f16a))
* Introduce agent skills for general development, database management, and Docker operations. ([01f1281](https://github.com/SSujitX/docklift/commit/01f128101d027cd74ff53da0d5f9f8995d50ae64))
