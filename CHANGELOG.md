## [2.0.2](https://github.com/SSujitX/docklift/compare/v2.0.1...v2.0.2) (2026-07-25)


### Bug Fixes

* **github:** list repos across all installs ([1a8c27c](https://github.com/SSujitX/docklift/commit/1a8c27cdd629b43252154f48eec28f468a225968))
* **ui:** drop fake IP:null public endpoints ([d4d2ea1](https://github.com/SSujitX/docklift/commit/d4d2ea1d2ce8f5ad5fba4ffcb1924c007b70c218))
* **ui:** fix domains empty-state port copy ([ea7c85d](https://github.com/SSujitX/docklift/commit/ea7c85da32e0abead736dd79ce5777abfe16146d))


### Features

* **ui:** show all github accounts on new project ([17a513b](https://github.com/SSujitX/docklift/commit/17a513b0975f895a73ac6815904fe7b79c21a684))

## [2.0.1](https://github.com/SSujitX/docklift/compare/v2.0.0...v2.0.1) (2026-07-25)


### Bug Fixes

* **build:** tighten secret and path preflight ([99eb20d](https://github.com/SSujitX/docklift/commit/99eb20d5b453e2abd79df4c476a14a74445a8026))
* **compose:** soft defaults without hard caps ([6776480](https://github.com/SSujitX/docklift/commit/67764806b463d666fba1669647f602e93132d8bd))
* **compose:** use real DEPLOYMENTS_PATH and BACKUP_PATH in dev ([3a0ea45](https://github.com/SSujitX/docklift/commit/3a0ea45ddca7230e14b43885f35623a5ff5a6421))
* **cors:** restore same-origin checks for CORS and terminal WS ([ab8d93b](https://github.com/SSujitX/docklift/commit/ab8d93bd5756009eb4df033af9b446556d897964))
* **Dashboard:** use authFetch for project list ([8ca62fe](https://github.com/SSujitX/docklift/commit/8ca62fec9fcc4e1b386a91b89f222dc71bbf5663))
* **Databases:** use authFetch for project list ([9468d8e](https://github.com/SSujitX/docklift/commit/9468d8ed57227172448f1565fb58bb395c554794))
* **deploy:** safe cancel teardown and secret preflight ([cfb6bbb](https://github.com/SSujitX/docklift/commit/cfb6bbbde138a7a7c988df01e5f1c00a61f264d1))
* **dns:** clarify ownership check errors ([a292552](https://github.com/SSujitX/docklift/commit/a292552f4f68908b4e5de4b19de0aa6ef1d2f1f1))
* **docker:** boot via ensureDb not db push ([7098443](https://github.com/SSujitX/docklift/commit/709844396d29edb78f28013a7900f85dfadad3b4))
* **domains:** validate panel port and block duplicate hosts ([2980540](https://github.com/SSujitX/docklift/commit/2980540ebb85c7752805cd3615ebc240b4def769))
* **EnvVarsManager:** use authFetch for env CRUD ([45cd8ff](https://github.com/SSujitX/docklift/commit/45cd8ff0dab09c35e4d201d2d64d0ff47cc0c7e6))
* explain stale dependency lockfiles ([652c5cc](https://github.com/SSujitX/docklift/commit/652c5cc9cd5cf1c2cc4430f44388e3f54dfa6ceb))
* **FileEditor:** use authFetch for file save ([4092d2b](https://github.com/SSujitX/docklift/commit/4092d2b847d1bcb409de1f7a1eca5d1e8dbb4d0b))
* **github:** add credentials option to fetch request for GitHub manifest ([996fdc8](https://github.com/SSujitX/docklift/commit/996fdc87bed6ae612398b2c0b12c2743bd83f5e6))
* **github:** harden multi-state webhook setup ([efae7b6](https://github.com/SSujitX/docklift/commit/efae7b6fde0bd529dea3a871be5adc76b924ca19))
* **github:** multi-state setup and skip deleted branch pushes ([9738720](https://github.com/SSujitX/docklift/commit/9738720dd376b21ba511ce3bb9d32c433ea8a41a))
* **install:** align dev install onboarding ([ff4523c](https://github.com/SSujitX/docklift/commit/ff4523cd790119ecc59f8e86863210f91c8e508d))
* **install:** print public dashboard onboarding ([b6283c1](https://github.com/SSujitX/docklift/commit/b6283c1ba448fd0c0b6da9ef0868bb5263951955))
* **logs:** name the two nginx containers by their role ([eac21ee](https://github.com/SSujitX/docklift/commit/eac21eec087472e8f928db807904f3037c80efa6))
* **LogViewer:** correct search highlight matching ([19b7dba](https://github.com/SSujitX/docklift/commit/19b7dba8568c81f7719ceb1385b28809aef9d9e3))
* **NewProject:** guard repos response and authFetch create ([f05048c](https://github.com/SSujitX/docklift/commit/f05048c913a1a6b12c45f4ed5df35d8ac8aefabf))
* **nginx:** surface reload failures instead of fake success ([f28c7a4](https://github.com/SSujitX/docklift/commit/f28c7a40b3c08d706239d809d2d637e49d8cc476))
* **ports:** refuse delete when port still in use ([82ab6ec](https://github.com/SSujitX/docklift/commit/82ab6ec1f6b4c9bbdefcca9f51b5daf4b037a963))
* **Ports:** use authFetch for ports API ([4d1025e](https://github.com/SSujitX/docklift/commit/4d1025e1197bdf68817506336959eb9a3a0c376f))
* **ProjectCard:** honor HTTP errors on actions ([8ec7727](https://github.com/SSujitX/docklift/commit/8ec7727779b4dae8de6ac88b549f62e14e98fe16))
* **ProjectDetail:** abort stale history and stream error checks ([0e3b4e6](https://github.com/SSujitX/docklift/commit/0e3b4e68768544d10f68288035c48d9557c9d780))
* **projects:** disconnect proxy before delete ([532ddb3](https://github.com/SSujitX/docklift/commit/532ddb328656ee11e74ab5d37d27fe5e5f14595b))
* **ServiceDomainCard:** serial domain mutations and authFetch ([f63c40c](https://github.com/SSujitX/docklift/commit/f63c40ca942cf7639271540e8b5827590babf1a7))
* **Settings:** authFetch and stream failure handling ([bf9eda7](https://github.com/SSujitX/docklift/commit/bf9eda7d509bbe28c17fa74fbad26b4a3219e361))
* **ssl:** make www domains explicit ([2375c1c](https://github.com/SSujitX/docklift/commit/2375c1cf7187d156a95d8ee4262b6b52de3d6ce7))
* **ssl:** show actionable certificate errors ([828e88c](https://github.com/SSujitX/docklift/commit/828e88c3a23cb0b35b38c828454eacee7567fa43))
* **status:** report mixed services as degraded ([654a2ed](https://github.com/SSujitX/docklift/commit/654a2ed32f58881bfbfa552bf5dafe793f2ed1fb))
* **system:** include certbot in core container lists ([a00d58b](https://github.com/SSujitX/docklift/commit/a00d58b4eafadfca0ded3d0f339f35757e20ab94))
* **system:** narrow prune and upgrade host exec ([f2eab65](https://github.com/SSujitX/docklift/commit/f2eab65c5a72f580003ba278b2e52e5043a0bd0d))
* **SystemOverview:** use authFetch for system APIs ([bb2418a](https://github.com/SSujitX/docklift/commit/bb2418a66ae7d6fccdb60789b0d55785ad1ffe47))
* **system:** re-auth execute against JWT user ([e7f9288](https://github.com/SSujitX/docklift/commit/e7f92884c14e42ea0412f74403119ca81357ddb9))
* **TerminalView:** settle password cancel and authFetch ([b0a8b44](https://github.com/SSujitX/docklift/commit/b0a8b446bf06ab18bcba2c39b8ca126985931102))
* **ui:** clarify github connect states ([cfa0d0a](https://github.com/SSujitX/docklift/commit/cfa0d0a669b50bac312d18f152224dd78b65922d))
* **ui:** databases page auth and copy ([40f1011](https://github.com/SSujitX/docklift/commit/40f10112b6426b1d65ab391e0ffec4be7c7c4d77))
* **ui:** narrow purge to dangling images ([8eb605e](https://github.com/SSujitX/docklift/commit/8eb605e4c247d45eb80491267be01355755a773f))
* **ui:** ports page opt-in host port copy ([4d84ddd](https://github.com/SSujitX/docklift/commit/4d84dddb443152ec24be77db1ab1c4cd90bbd2fe))
* **ui:** project detail degraded and safety ([fb6e762](https://github.com/SSujitX/docklift/commit/fb6e762762addca404ee560a137a9cfda822dc2b))
* **ui:** reflect degraded on project cards ([c60f672](https://github.com/SSujitX/docklift/commit/c60f6725f7c5167f66396651563d565ecd73a6a1))
* **ui:** step-up and restore critical copy ([b8e142f](https://github.com/SSujitX/docklift/commit/b8e142f9a23916deed1081eea048016591104cbd))
* **ui:** unique env var editing rules ([055e553](https://github.com/SSujitX/docklift/commit/055e5535b7cd77b5521fb8da8442d89b55e9ee69))
* **uninstall:** limit cleanup to docklift resources ([adc0fd9](https://github.com/SSujitX/docklift/commit/adc0fd95fbad5a2c1c61fdd166f7f5e0c677f085))
* **uninstall:** remove all DockLift resources, spare other workloads ([686b0d1](https://github.com/SSujitX/docklift/commit/686b0d198629be0543557a1d82dfcffaa7757aac))
* **upgrade:** verify stop before db snapshot ([574f26b](https://github.com/SSujitX/docklift/commit/574f26be151683c18c74495281366389c3b8427c))


### Features

* add automatic builds and persistent storage ([968e324](https://github.com/SSujitX/docklift/commit/968e324f92cb4f51ec54b5d388773098e34761d1))
* **api:** seal middleware and setup rate limits ([38f515c](https://github.com/SSujitX/docklift/commit/38f515c62d8b3f19c6bd87e0b369bef446e19e29))
* **AppShell:** focus trap mobile drawer ([a8b9ac6](https://github.com/SSujitX/docklift/commit/a8b9ac688df8f08d569f010d19915ec3a7338652))
* **auth:** add password step-up helper ([4c82c27](https://github.com/SSujitX/docklift/commit/4c82c2795e1dfac259a3847ed49ae8368fb07e2a))
* **auth:** authFetch clears session on 401 ([ef8c2c2](https://github.com/SSujitX/docklift/commit/ef8c2c281c084c887a46a9a73129b850487928d6))
* **auth:** claim bootstrap and issue pwdv on session tokens ([d67d209](https://github.com/SSujitX/docklift/commit/d67d2092144743b8af2916b58869442ec65dd9b4))
* **AuthProvider:** register 401 logout handler ([886cebf](https://github.com/SSujitX/docklift/commit/886cebf4da35c2df6833a1ed77f27316969e2d60))
* **auth:** require pwdv claim to match passwordChangedAt ([1900f3a](https://github.com/SSujitX/docklift/commit/1900f3a4a0f761dfc2ab311a5c5ced8b21cbadcc))
* **backup:** consistent SQLite snapshot and honest restore status ([03ff7d7](https://github.com/SSujitX/docklift/commit/03ff7d787170b1982227e5b3c512a3611a75639a))
* **backup:** fail-closed restore and critical clear ([860c18a](https://github.com/SSujitX/docklift/commit/860c18a94356d4c2c5b2a08a2376136ccb56f208))
* **bootstrap:** atomic claim and recover stale registration claims ([3e54e99](https://github.com/SSujitX/docklift/commit/3e54e9995cb2a6e301fb0f3d2b1d48b7902f0655))
* **certs:** improve SSL activity events and renew handling ([dc25eed](https://github.com/SSujitX/docklift/commit/dc25eedd83ca3c9a29f1ca256db40427741318ea))
* **CommandPalette:** focus trap and authFetch ([2adf044](https://github.com/SSujitX/docklift/commit/2adf0448105ea888af710e11920b7cfb5252c438))
* **compose:** dedupe colliding scanned service names ([2ecf7ab](https://github.com/SSujitX/docklift/commit/2ecf7abcbc2a70daaf0d9dd37f18fd0b3cc81e41))
* **compose:** set BACKUP_PATH and DATA_PATH in production ([f885547](https://github.com/SSujitX/docklift/commit/f885547921de1b908ffda3d75625258e9f20bf72))
* **compose:** verify teardown by exact labels ([8329ca7](https://github.com/SSujitX/docklift/commit/8329ca76a2a688bf8e310acbe98cd252083b2ff5))
* **config:** add backupPath configuration to specify backup directory location ([7b728d8](https://github.com/SSujitX/docklift/commit/7b728d85bb3fb6d82267dbf2644f28fc47e0b1f8))
* **db:** add checked-in init migration ([418b303](https://github.com/SSujitX/docklift/commit/418b303632843a1efc64815566f67b088b8989d2))
* **db:** add host-port and secret env fields ([d326d52](https://github.com/SSujitX/docklift/commit/d326d5250956e65ccf3bcb2bfb800c18a336b702))
* **db:** ensure migrate deploy on boot ([4186e3f](https://github.com/SSujitX/docklift/commit/4186e3ffe4bbc33441f6f9082ff64663906c1ed4))
* **deploymentRecovery:** reconcile stale builds on boot ([45372f4](https://github.com/SSujitX/docklift/commit/45372f4ecc24a2616c3d9cec7b6a32b71a968157))
* **deployments:** add pagination support and metadata option for listing deployments ([19a1877](https://github.com/SSujitX/docklift/commit/19a18773e20f10eded9d436439ba27dbb66e6c54))
* **deployments:** harden cancel, scrub, ports, and status sync ([b4ac11c](https://github.com/SSujitX/docklift/commit/b4ac11c55ece4d9cef55c4359777141afe657d62))
* **deploymentState:** track in-flight project deploys ([2a823b8](https://github.com/SSujitX/docklift/commit/2a823b8708d5c3ab4084d6978b12e5408fbb604e))
* **dnsCheck:** DNS preflight helpers for custom domains ([f9ca685](https://github.com/SSujitX/docklift/commit/f9ca68503b6f81ccf4a10838e9bb1050098aedf6))
* **DnsGuideCard:** introduce DnsGuideCard component for DNS setup guidance, including A-record examples and Cloudflare notes ([eba4d58](https://github.com/SSujitX/docklift/commit/eba4d58e31b19478e46dfd2fb564ba00aa578d22))
* **docker:** connect proxy to project networks ([8a4ec96](https://github.com/SSujitX/docklift/commit/8a4ec962351c920d0f9c00f5524c4f35330f9488))
* **domain:** add domain input normalization and validation functions for improved hostname handling ([88ba0c7](https://github.com/SSujitX/docklift/commit/88ba0c7de32af49262e993c050283d7a5640901f))
* **domainOwnership:** reject duplicate hostnames ([1b52d39](https://github.com/SSujitX/docklift/commit/1b52d3935ec3b1d007d195fe1c6652950fa4ec09))
* **env:** dedupe env variables on boot ([117309d](https://github.com/SSujitX/docklift/commit/117309df5d6746739646dc786935e1df442fbe2e))
* **focusTrap:** trap and restore focus for overlays ([52f5851](https://github.com/SSujitX/docklift/commit/52f5851af89601e12ef9ae58f583f006cca86a2c))
* **frontend:** add degraded project status type ([07e1441](https://github.com/SSujitX/docklift/commit/07e1441233911b242012eb252978c6ab26960c72))
* **fsCopy:** implement cross-platform recursive directory copy and atomic swap functions ([846ae38](https://github.com/SSujitX/docklift/commit/846ae38def113405004e7780f1687ee2077b62c8))
* **index:** maintenance gate and boot deployment recovery ([fe63c10](https://github.com/SSujitX/docklift/commit/fe63c10dfa1e237c50e12f154713fb8e70035af2))
* **logs:** add certbot stream and follow the tail reliably ([c678a7a](https://github.com/SSujitX/docklift/commit/c678a7a29b89c6641c4ceb091a95c31dd1c3adf0))
* **maintenance:** add process-wide maintenance gate to manage API access during restore operations ([1db75ee](https://github.com/SSujitX/docklift/commit/1db75ee5287a28ac2a94c833dd3682cfb710e8b4))
* **naming:** add path hash helpers for service and storage keys ([20004b9](https://github.com/SSujitX/docklift/commit/20004b9084b63cf19d6b29eb4cb738be37df5103))
* **naming:** add project network name helper ([bf5b5e7](https://github.com/SSujitX/docklift/commit/bf5b5e76b7d4c6e80d1bd94d9e193c0f014a9725))
* **networking_proxy:** enhance SSL issuance process with DNS preflight checks and activity logging ([ce35705](https://github.com/SSujitX/docklift/commit/ce357055eaf5f5bad4a0d532b8130da83eb3a446))
* **nginx:** enhance SSL event logging during Nginx configuration updates for better error handling and user feedback ([7dc21ca](https://github.com/SSujitX/docklift/commit/7dc21ca3bb573f6dd81fde930be7d3119381d812))
* **portAllocation:** allocate host ports atomically ([acccfd6](https://github.com/SSujitX/docklift/commit/acccfd6d4f94b8324702ec11241cd88e4aaf188a))
* **ProjectDetail:** implement deployment history pagination and enhance real-time log tracking ([5247cfe](https://github.com/SSujitX/docklift/commit/5247cfe523b786540d0bf3c8a8eb2b78726456e7))
* **ProjectDetail:** integrate DNS management components and streamline domain handling ([434605f](https://github.com/SSujitX/docklift/commit/434605f5a12913c08ddbdeaf41897867fc4de82f))
* **projects:** rollback create failures and block delete while deploying ([8bb04a9](https://github.com/SSujitX/docklift/commit/8bb04a914bb2442b064e8a52901b7eded498aea9))
* **projectStatusSync:** sync status from all service containers ([5556a8e](https://github.com/SSujitX/docklift/commit/5556a8e785abc690753e678ea4a56c84272870d3))
* **restore:** add commit-or-rollback policy ([31cefc2](https://github.com/SSujitX/docklift/commit/31cefc20a9efe5c5b71640d0c06eced07d71a30d))
* **restore:** add restore lock helper ([8ef8c9a](https://github.com/SSujitX/docklift/commit/8ef8c9a8e835931d115efe28ca09eb5948349b50))
* **restore:** add setup-token restore auth ([7987dde](https://github.com/SSujitX/docklift/commit/7987dde8688f029edb773406108c9a887d11e77b))
* **restore:** persist critical seal marker ([37c4363](https://github.com/SSujitX/docklift/commit/37c43630f7099dbd4bb62756750a27f76871fd11))
* **runCompose:** safe docker spawn with error and close handlers ([eb62c3d](https://github.com/SSujitX/docklift/commit/eb62c3d1c81ce997edcfc0a455a6b57383587ed9))
* **ServiceDomainCard:** add ServiceDomainCard component for managing service domains, SSL status, and DNS checks ([a29f5df](https://github.com/SSujitX/docklift/commit/a29f5df89fcb09a1c2918b0ffbd06a7b569cbcd9))
* **shell:** add expandedOnHover prop to Sidebar for improved user experience ([0d288a2](https://github.com/SSujitX/docklift/commit/0d288a26a3cdee81a3e9a383372b310284d6cd1f))
* **shell:** enhance sidebar interaction with hover state ([9572ddb](https://github.com/SSujitX/docklift/commit/9572ddbf487e0c7637f4a8256efdfbad92813586))
* **sslHelp:** implement SSL error handling with user guidance for common certbot issues ([7a4e335](https://github.com/SSujitX/docklift/commit/7a4e3351bf46dee1bad7dc815056ea30dfea80ea))
* **streamProgress:** implement function to consume text streaming responses and handle errors ([d19f89d](https://github.com/SSujitX/docklift/commit/d19f89d86d2364c9ae8dd434e3842d4f1b8904d4))
* **types:** add SslEvent and DomainDnsCheck interfaces for enhanced event logging and DNS status tracking ([b0e79bf](https://github.com/SSujitX/docklift/commit/b0e79bf1ac8d7d2a7746ab5e72753e31deed2eba))
* **ui:** rebuild dashboard around a left sidebar shell ([55a0622](https://github.com/SSujitX/docklift/commit/55a062212fbc1014fe55d2963fa5783d4500cf23))
* **ui:** require password for restore dialogs ([e75dfa6](https://github.com/SSujitX/docklift/commit/e75dfa64bb2f789000219367123d434eb272dc7e))
* **ui:** show degraded status badge ([ba25e27](https://github.com/SSujitX/docklift/commit/ba25e27be186e075bd80070910a9aa9899c68b82))

# [2.0.0](https://github.com/SSujitX/docklift/compare/v1.3.21...v2.0.0) (2026-07-24)


### Bug Fixes

* **backup:** redeploy from project root after restore ([ede06d2](https://github.com/SSujitX/docklift/commit/ede06d28f9ea44c221d56305b6e996779c68fa34))
* **frontend:** harden SSE usage and adaptive polling ([1f84a4f](https://github.com/SSujitX/docklift/commit/1f84a4fdccf0d2d433a9e65ba5b645a76803967a))
* **scripts:** align install flow with certbot and public :8080 ([2e839e3](https://github.com/SSujitX/docklift/commit/2e839e3e0b61ebb2273ccfa250af682ff81ae071))
* **security:** scrub git remotes and require webhook secret ([33ca1d4](https://github.com/SSujitX/docklift/commit/33ca1d4962949d535b37244a1e7bc00895ff4c0d))


### Features

* **auth:** split bearer and SSE auth middleware ([f8e03ef](https://github.com/SSujitX/docklift/commit/f8e03ef89f2950030bf6a25e19e8c775a982b1bf))
* **deploy:** cancel builds and fix status lifecycle ([262dd5c](https://github.com/SSujitX/docklift/commit/262dd5c149357af94e3c1ffacc23852d71fdb2e1))
* **frontend:** migrate dashboard from Next.js to Vite ([39801ff](https://github.com/SSujitX/docklift/commit/39801ff79e104bd7f8b414c059e465858a3b13c0))
* **security:** harden auth, CORS, uploads, and GitHub CSRF ([cde084b](https://github.com/SSujitX/docklift/commit/cde084b7efde25a06843cade9aa10388296808d0))
* **ssl:** add Let's Encrypt ACME and HTTPS vhosts ([79b1b9e](https://github.com/SSujitX/docklift/commit/79b1b9ea79e67ce9ccff9fc4d949a9b20f5c20b8))
* Vite frontend and SSL *force major* ([3108c66](https://github.com/SSujitX/docklift/commit/3108c6646dc9f4e53664e6d760ad7798e40faf17))
* Vite frontend and SSL *force major* ([d08a4c8](https://github.com/SSujitX/docklift/commit/d08a4c8cddb0a72f708eff64cca560218e9604fb))

## [1.3.21](https://github.com/SSujitX/docklift/compare/v1.3.20...v1.3.21) (2026-05-12)


### Features

* **docker:** re-enable IPv6 support and configure subnets for docklift_network in Docker Compose files ([4a1431a](https://github.com/SSujitX/docklift/commit/4a1431ad5564c0eea15b0fe15b11993855ca4234))

## [1.3.20](https://github.com/SSujitX/docklift/compare/v1.3.19...v1.3.20) (2026-05-12)


### Features

* **docker:** remove IPv6 support and subnet configurations from Docker Compose files ([16748ca](https://github.com/SSujitX/docklift/commit/16748ca13c99f77581397e3b4acb55c2fda413a4))

## [1.3.19](https://github.com/SSujitX/docklift/compare/v1.3.18...v1.3.19) (2026-05-10)


### Features

* **deployments:** update service domains on stop and cancel actions ([479a4c7](https://github.com/SSujitX/docklift/commit/479a4c7520502fea7ced02469e24b07130c8ee13))

## [1.3.18](https://github.com/SSujitX/docklift/compare/v1.3.17...v1.3.18) (2026-05-10)


### Bug Fixes

* **dashboard:** prevent polling interval from resetting on buildingCount change ([0cbf936](https://github.com/SSujitX/docklift/commit/0cbf936cd63802dcae1efd177138266886c773e2))
* **deployments:** improve security by scrubbing GitHub tokens and preventing command injection ([4aa5cd7](https://github.com/SSujitX/docklift/commit/4aa5cd798fc1fbcaece8f524fe3b723622294e26))
* **github:** verify webhook signature before database lookups ([747683d](https://github.com/SSujitX/docklift/commit/747683d2945031153d6db80f58bde35070bd3fa9))
* **settings:** download backup using fetch instead of window.open ([e7259fa](https://github.com/SSujitX/docklift/commit/e7259fa887d221caf17e06ee0dbc24323469cb2b))
* **terminal:** add input validation to prevent command injection ([86714ab](https://github.com/SSujitX/docklift/commit/86714ab5b47f8578bcb269780fa2e7fcd9269072))
* **TerminalView:** prevent memory leak by cleaning up contextmenu listener ([5f95ef7](https://github.com/SSujitX/docklift/commit/5f95ef767d0b50d1b95ffc65b72c3fc25d460251))


### Features

* add system routes for monitoring, server controls, and command execution. ([65918e0](https://github.com/SSujitX/docklift/commit/65918e0af0ce7e9cb1bf64751783a664902638e3))
* **docker:** enable IPv6 support and configure subnets for docklift_network ([4e0c943](https://github.com/SSujitX/docklift/commit/4e0c943413f14ef96e0ae5d575671190decd5fe6))
* enable IPv6 support and configure subnets for docklift_network ([c786ccd](https://github.com/SSujitX/docklift/commit/c786ccd768dd13f7c13284eaea642f878c4f28ee))
* Establish core application structure for project deployment, including a frontend dashboard and backend Docker services. ([d38f3e3](https://github.com/SSujitX/docklift/commit/d38f3e35e0e98716c2e4e63c22462dc08cfee624))
* Implement core application features including settings, GitHub integration, domain management, and system backups. ([004d26c](https://github.com/SSujitX/docklift/commit/004d26c9abc21896e0227c2e49d2509cc4685e81))
* **setup:** add setup token for unauthenticated backup restore ([e461826](https://github.com/SSujitX/docklift/commit/e4618268af6ff8d7b39ef3b85965dd1cd3f4e8dd))
* update dependencies in package.json and bun.lock ([36d849c](https://github.com/SSujitX/docklift/commit/36d849cf5d07833411687aab7e5c6feadee6e4f3))
* update package dependencies and add overrides for brace-expansion ([d46b83d](https://github.com/SSujitX/docklift/commit/d46b83d419796d6afba92c74e7954685859a99a4))

## [1.3.17](https://github.com/SSujitX/docklift/compare/v1.3.16...v1.3.17) (2026-02-17)


### Bug Fixes

* **nginx:** use dynamic Connection header for WebSocket proxying ([1ebd6c5](https://github.com/SSujitX/docklift/commit/1ebd6c5bcdf84dd5507740d82e82ed560df55808))


### Features

* **nginx-proxy:** route unmatched domains to docklift dashboard ([3c5404d](https://github.com/SSujitX/docklift/commit/3c5404dc72d04f1a39be7d04f14995ee33ba9f72))
* **nginx:** add WebSocket upgrade support via connection header mapping ([7aa4c26](https://github.com/SSujitX/docklift/commit/7aa4c26468c5a4e5553f942e1e07c4f94c8c101b))

## [1.3.16](https://github.com/SSujitX/docklift/compare/v1.3.15...v1.3.16) (2026-02-17)


### Bug Fixes

* **nginx:** conditionally set Connection header for WebSocket upgrade ([1ddb1c7](https://github.com/SSujitX/docklift/commit/1ddb1c7f18a3d5116435305133bbec63af6ddf3a))

## [1.3.15](https://github.com/SSujitX/docklift/compare/v1.3.14...v1.3.15) (2026-02-17)


### Bug Fixes

* **TerminalView:** handle missing API_URL for WebSocket connection ([cb6ab60](https://github.com/SSujitX/docklift/commit/cb6ab601b5afb20c2e87d64148a1954bb1f0793f))
* update import path for Next.js routes type definitions ([4f4dbcf](https://github.com/SSujitX/docklift/commit/4f4dbcfb4093d6eca37d8257a284f2e7ace279c9))


### Features

* **nginx:** add WebSocket proxy configuration for terminal ([8c4354d](https://github.com/SSujitX/docklift/commit/8c4354dfd13965b6b2063969bc9e2bea4ad6f66e))
* **terminal:** add clipboard support with copy/paste shortcuts ([105171d](https://github.com/SSujitX/docklift/commit/105171deca1caf14ace2c9a076a372aa7035dd6e))
* **terminal:** add colorful prompt and set initial working directory ([5781ca4](https://github.com/SSujitX/docklift/commit/5781ca42dae9acb7cb317e58cc990bdfb62a8815))
* **terminal:** add fullscreen toggle functionality ([ae6735b](https://github.com/SSujitX/docklift/commit/ae6735b9365f43ba214d85ba39ccbfc764c418d6))
* **terminal:** add WebSocket-based interactive PTY shell service ([6f1a5dd](https://github.com/SSujitX/docklift/commit/6f1a5dd6a007f972b67f169b94bc8c0b9b78bd5b))
* **terminal:** attach WebSocket server to HTTP server ([d787879](https://github.com/SSujitX/docklift/commit/d787879b47d9b088cb8ec5edd0c08d564ebf6895))
* **terminal:** replace node-pty with script-based shell for zero native dependencies ([5b6544e](https://github.com/SSujitX/docklift/commit/5b6544e0733e729fb968ffe33e6056a82e3e5e5a))
* **terminal:** replace static command logs with interactive xterm.js shell ([3125827](https://github.com/SSujitX/docklift/commit/3125827231da4458e23d1108404fa264bb18eb5c))

## [1.3.14](https://github.com/SSujitX/docklift/compare/v1.3.13...v1.3.14) (2026-02-17)

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
