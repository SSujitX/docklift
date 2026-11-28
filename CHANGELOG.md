# [3.0.0](https://github.com/SSujitX/docklift/compare/v2.1.6...v3.0.0) (2026-11-28)


* feat!: Docklift 3.0 with the Go backend (BREAKING CHANGE) ([52bc105](https://github.com/SSujitX/docklift/commit/52bc105f969cc3b08e43024bdd9a6df956dcc34a))


### Bug Fixes

* **backup:** keep archive file modes on restore, make certificate keys owner-only ([c4dbd31](https://github.com/SSujitX/docklift/commit/c4dbd319cd6959e1f96903eae6ab50312c881523))
* **frontend:** restore Tailwind 3 look, drop removed containers, close terminal prompt on timeout ([f928c36](https://github.com/SSujitX/docklift/commit/f928c36982a4c254766b8117a42d5f005fec57f2))
* **install:** retry Docker install and show its error output ([3dd66a4](https://github.com/SSujitX/docklift/commit/3dd66a47a6da498e54c45e563cdbc190b182761e))
* **server:** harden auth, uploads and sessions; reattach proxy after upgrade ([87927cd](https://github.com/SSujitX/docklift/commit/87927cd12fc027073cfe83c57ca1050f0cc3be75))
* **server:** wire purge retention and harden system, logs and terminal ([d9c2c8e](https://github.com/SSujitX/docklift/commit/d9c2c8e051e2cdf26630332ffe723074681e731e))
* stop ignoring the Go logs route package ([b48a8bf](https://github.com/SSujitX/docklift/commit/b48a8bfe076d3807d5bd4731e76265ab9fd55155))
* **ui:** bust the cached favicon ([f588ae6](https://github.com/SSujitX/docklift/commit/f588ae636cfce669c07733e0acd87ad66bb60efb))


### Features

* **frontend:** API & MCP page for tokens and endpoints ([56e4ea3](https://github.com/SSujitX/docklift/commit/56e4ea305131644df2f5915ce58fd6de9af4d9c6))
* **frontend:** domain-only access switch in Settings ([dc20609](https://github.com/SSujitX/docklift/commit/dc20609c21e6a628b6d45db51d8893a1ef10ba2f))
* **scripts:** progress display, temporary build swap on small hosts, short failure output ([5691eed](https://github.com/SSujitX/docklift/commit/5691eedb680c49a346cc98c3eac5b3e0a684f5db))
* **server:** add backup and crash-safe restore ([69a8f30](https://github.com/SSujitX/docklift/commit/69a8f30eec65829fa6af4f6dc7ced00ecd190756))
* **server:** add command runner, Docker wrapper and deploy core ([2936466](https://github.com/SSujitX/docklift/commit/2936466d47f7c5ac809fa3802b8cd87814996180))
* **server:** add deployment pipeline, rollback and webhook queue ([2bf1986](https://github.com/SSujitX/docklift/commit/2bf1986e491cab102b47eb61833ce68e7f408adb))
* **server:** add git, path safety, unzip, files and ports routes ([a64ec8e](https://github.com/SSujitX/docklift/commit/a64ec8e5489d62b9c397244c668a3124928d4e57))
* **server:** add GitHub App integration and webhooks ([722aad9](https://github.com/SSujitX/docklift/commit/722aad90213107b6ed0139969ee4d6585608fb08))
* **server:** add Go backend foundation with auth and database layer ([fae5e7a](https://github.com/SSujitX/docklift/commit/fae5e7ae65e4d9604444ccd9fa0ea132799b3449))
* **server:** add managed database routes ([3dd7eb5](https://github.com/SSujitX/docklift/commit/3dd7eb53db03c57c81aaefb714c7828f8f4ca046))
* **server:** add nginx, certificate and DNS services ([3160847](https://github.com/SSujitX/docklift/commit/31608473fa256445ae25755d916fd7ecd00676c0))
* **server:** add panel domain and SSL routes ([6445fba](https://github.com/SSujitX/docklift/commit/6445fbac7b655f8925828686050bb644d784cfe7))
* **server:** add projects routes ([948b552](https://github.com/SSujitX/docklift/commit/948b5521a99464080e2cfdbeaa6b2bcba615f0c9))
* **server:** API tokens, OpenAPI endpoint map and MCP server ([aca8e98](https://github.com/SSujitX/docklift/commit/aca8e9884ac800a3362be90d8b178f9be42e2ccf))
* **server:** domain-only dashboard access with CLI escape hatch ([192e8c7](https://github.com/SSujitX/docklift/commit/192e8c70a5cf772080efd199b17da7c3fc988488))
* **server:** trust forwarded headers only from known proxies ([e1f2049](https://github.com/SSujitX/docklift/commit/e1f20494fcf775eacfec48777f83c4381fb49ae2))
* **server:** wire Docker, deploy, edge and git services into startup ([3aa0afb](https://github.com/SSujitX/docklift/commit/3aa0afbbeb5c02dd056439f10045a68ba80f14db))
* **server:** wire GitHub App and deploy queue, limit password guesses, share JS helpers ([3207fc7](https://github.com/SSujitX/docklift/commit/3207fc7ed4768918c10f3d8bd9ba6afb5abcbb1d))
* **ui:** new design tokens and global styles ([00ca002](https://github.com/SSujitX/docklift/commit/00ca0029685a0da28560747b8120709e39e572ae))
* **ui:** redesign app shell layout ([eb04339](https://github.com/SSujitX/docklift/commit/eb04339e3b025479f5ba9d0ab2b2f90ed4823548))
* **ui:** redesign AttachDatabasePanel ([ad0e5a1](https://github.com/SSujitX/docklift/commit/ad0e5a1c46fcbb05b03b80e81325b8be75a3cea3))
* **ui:** redesign BranchSelector ([ea53d0c](https://github.com/SSujitX/docklift/commit/ea53d0ca4582fb13feb7b623de11ef7f776f352a))
* **ui:** redesign button primitive ([bd493f9](https://github.com/SSujitX/docklift/commit/bd493f9082f8cbce2b4c54240ae511f2691b18c4))
* **ui:** redesign card primitive ([5581fc2](https://github.com/SSujitX/docklift/commit/5581fc252258a88324360a113b8d9adc813446ba))
* **ui:** redesign Dashboard page ([896bcbf](https://github.com/SSujitX/docklift/commit/896bcbf95720509a5f1dad9c1c7ec25ca2351f57))
* **ui:** redesign Databases page ([e6d2c1b](https://github.com/SSujitX/docklift/commit/e6d2c1b049eac2b8e5541bda88e9b52695f26ddf))
* **ui:** redesign Developer page ([10caee7](https://github.com/SSujitX/docklift/commit/10caee739a1e615474c4c188c400f0b7988dd637))
* **ui:** redesign dialog primitive ([09c6447](https://github.com/SSujitX/docklift/commit/09c644715e11a1c85be02bfaaa8c156f4edc2742))
* **ui:** redesign DnsGuideCard ([4e05dfd](https://github.com/SSujitX/docklift/commit/4e05dfdca1678ecc96ac7a72316ef70a94356b9f))
* **ui:** redesign EnvVarsManager ([a6e157d](https://github.com/SSujitX/docklift/commit/a6e157d699543924ee06f7314ad2e0e7cb7ae931))
* **ui:** redesign FileEditor ([77fa7e3](https://github.com/SSujitX/docklift/commit/77fa7e33f6292c04d370e5b2b1eb482b6851d676))
* **ui:** redesign FileTree ([fe5ab59](https://github.com/SSujitX/docklift/commit/fe5ab592346c263c81df9868b5fff7d11b3529c6))
* **ui:** redesign GitHubConnect ([bebd5ad](https://github.com/SSujitX/docklift/commit/bebd5ad97576836e44777875234dc07acb1632fa))
* **ui:** redesign input primitive ([874b929](https://github.com/SSujitX/docklift/commit/874b9294578c801835827acaeed20fcdf6e32e4c))
* **ui:** redesign Logs page ([0a96493](https://github.com/SSujitX/docklift/commit/0a96493ba07c8eefb3269a5fb94a56c6545bce1b))
* **ui:** redesign LogViewer ([31ddff3](https://github.com/SSujitX/docklift/commit/31ddff3634ee0b86f12708caf043bac87baff77d))
* **ui:** redesign ManagedDatabasePanel ([eb551e5](https://github.com/SSujitX/docklift/commit/eb551e51c2bf5a1fe500ef962b05ee8c8458206d))
* **ui:** redesign NewDatabase page ([693ae01](https://github.com/SSujitX/docklift/commit/693ae018419f41453a9217f86967233cb6402fe1))
* **ui:** redesign NewProject page ([ea5b88a](https://github.com/SSujitX/docklift/commit/ea5b88a97f64ddbf59e08d323644db6c3a2e7b2b))
* **ui:** redesign PanelAccess ([148dd42](https://github.com/SSujitX/docklift/commit/148dd425f271f98d8b77b9d5dad485eb4e06e2c7))
* **ui:** redesign PanelDomainCard ([d606301](https://github.com/SSujitX/docklift/commit/d6063010a3fd48f082abb3d5553f85ff8aad8622))
* **ui:** redesign Ports page ([b108d09](https://github.com/SSujitX/docklift/commit/b108d09b675c13f9eda793b102777d10e77096fb))
* **ui:** redesign ProjectActionBar ([494352c](https://github.com/SSujitX/docklift/commit/494352cebbe458f5eb19a7f80ef778774c0ae4d4))
* **ui:** redesign ProjectCard ([497992f](https://github.com/SSujitX/docklift/commit/497992f164ea04404731270446410e53f65d6c23))
* **ui:** redesign ProjectDetail page ([50e1801](https://github.com/SSujitX/docklift/commit/50e18013fa54052e146e9b4f332aaebebb8bbf4e))
* **ui:** redesign ProjectQuickActions ([f603717](https://github.com/SSujitX/docklift/commit/f60371746f9ece93b5907296f25032c87e0eb857))
* **ui:** redesign ProjectResourceCell ([5072a13](https://github.com/SSujitX/docklift/commit/5072a13d37336421bd44b31b24d6bbf6941cef2a))
* **ui:** redesign ScrollToTop ([0d4b4d9](https://github.com/SSujitX/docklift/commit/0d4b4d915d1b76c3c373c6304f39c2202ef35743))
* **ui:** redesign ServiceDomainCard ([9a38721](https://github.com/SSujitX/docklift/commit/9a387218699c00f30c7d8e46eed426554c7aa6d8))
* **ui:** redesign ServiceSwitcher ([c31c45c](https://github.com/SSujitX/docklift/commit/c31c45c28edbe3a5ef6115ce61c1b16c8fb3f854))
* **ui:** redesign Settings page ([48a6cfb](https://github.com/SSujitX/docklift/commit/48a6cfbf72db40cffbf460fea8c77903936ffcb4))
* **ui:** redesign Setup page ([8a9d321](https://github.com/SSujitX/docklift/commit/8a9d3217571d5c9abb72b18e6a19b1a1450630ae))
* **ui:** redesign shell AccountMenu ([6e1cd72](https://github.com/SSujitX/docklift/commit/6e1cd72bfd6a348fa8629381ebbf49ecf80b5d08))
* **ui:** redesign shell CommandPalette ([ca76c60](https://github.com/SSujitX/docklift/commit/ca76c60c2996fb6f11f0f6a39d929ae47461cddb))
* **ui:** redesign shell PageHeader ([3fdfe43](https://github.com/SSujitX/docklift/commit/3fdfe430f941afbb3f2daaf764f9a60953c402a9))
* **ui:** redesign shell Sidebar ([16186b7](https://github.com/SSujitX/docklift/commit/16186b7575ecfb12fdf046283a5531b1e3ed235a))
* **ui:** redesign shell SidebarStatus ([ae1ddc6](https://github.com/SSujitX/docklift/commit/ae1ddc6a34f12d3cae90d9cef00523925f5b491a))
* **ui:** redesign shell TopBar ([3bc0e77](https://github.com/SSujitX/docklift/commit/3bc0e77dfbb28a2fb658de2670a86ecdb7af9f3d))
* **ui:** redesign SignIn page ([0a43bd9](https://github.com/SSujitX/docklift/commit/0a43bd961b3ce8628b4405818504907240d94d41))
* **ui:** redesign sonner primitive ([c4e8d8b](https://github.com/SSujitX/docklift/commit/c4e8d8b30e6aaed0f82e9e7bed769568ce4229ca))
* **ui:** redesign SslStatusBadge ([f5ce479](https://github.com/SSujitX/docklift/commit/f5ce4797f5081f476167bb8fb69a014e089c047f))
* **ui:** redesign StatusBadge ([e00b5ff](https://github.com/SSujitX/docklift/commit/e00b5ffd1a906b0f8f91f5953b8c8652e81b7acc))
* **ui:** redesign SystemLogsPanel ([6ab4dfa](https://github.com/SSujitX/docklift/commit/6ab4dfaa35e1ad1f65c05b5b8daaa67bca62e1ef))
* **ui:** redesign SystemOverview ([330e379](https://github.com/SSujitX/docklift/commit/330e3796b17dcd2f2e2af01fb9d18eabbb10f3f3))
* **ui:** redesign tabs primitive ([3a3cd96](https://github.com/SSujitX/docklift/commit/3a3cd966bd47b7b68f4f583a98af69fc7d2b483c))
* **ui:** redesign Terminal ([f8b96f5](https://github.com/SSujitX/docklift/commit/f8b96f5f7486c634c5dcebd787c8dda98215f58d))
* **ui:** redesign Terminal page ([c484643](https://github.com/SSujitX/docklift/commit/c484643d49c7dc6590f10c0f8e3e32294085d665))
* **ui:** redesign TerminalView ([d948430](https://github.com/SSujitX/docklift/commit/d948430072c33236e6ac2683c38a1e20612f9dab))
* **ui:** redesign textarea primitive ([89866e0](https://github.com/SSujitX/docklift/commit/89866e097983d46a19de6af2a2ff24c8eb742c24))
* **ui:** redesign VersionChecker ([1541a75](https://github.com/SSujitX/docklift/commit/1541a7572132fd41072e1b9701f0545439cc154d))
* **ui:** update navigation labels ([493b6ca](https://github.com/SSujitX/docklift/commit/493b6ca926cd5d5bb5890dd8723c914d02173aae))
* **ui:** use the Docklift mark as the dashboard favicon ([e08b893](https://github.com/SSujitX/docklift/commit/e08b893854c28ae5d031e23820fb1dac9ac18966))
* **website:** add custom home page ([68abcba](https://github.com/SSujitX/docklift/commit/68abcbaa73fde32363d619706ab85b684e0d4644))
* **website:** add dashboard screenshots ([dd1aa23](https://github.com/SSujitX/docklift/commit/dd1aa238794f76da59e56ba24e9489339e02bf88))
* **website:** new favicon ([9de01b0](https://github.com/SSujitX/docklift/commit/9de01b0db2588527284016c802620f34d06e835b))
* **website:** new fonts and theme colour ([819c142](https://github.com/SSujitX/docklift/commit/819c142eca4ad58fb8479a6562dae4b91fb81bc3))
* **website:** new logo ([f56ec5f](https://github.com/SSujitX/docklift/commit/f56ec5fc1589de269c03f650ebc3482ab82b6cf7))
* **website:** redesign docs theme styles ([60f75b1](https://github.com/SSujitX/docklift/commit/60f75b1189767bea7236680d8a04af3b092da189))
* **website:** register home page component ([44c1f20](https://github.com/SSujitX/docklift/commit/44c1f209a6e47c71bffb07cd1559ca1e6406e041))
* **website:** restyle changelog preview ([374913e](https://github.com/SSujitX/docklift/commit/374913efcb326bef540d71b0e3b6d560df3f14b9))
* **website:** use the new home page ([e427a06](https://github.com/SSujitX/docklift/commit/e427a068893059dee6c431f1dc760cec2412ef9b))


### Performance Improvements

* **frontend:** cache GET data in memory, prefetch on hover, parallel loads ([dbe6714](https://github.com/SSujitX/docklift/commit/dbe6714b38ecea97d17d2ed925baf1c6e075e9db))


### BREAKING CHANGES

* the TypeScript backend is replaced by a single Go binary that also serves the dashboard. An install now runs 3 containers (docklift-backend, docklift-nginx-proxy, docklift-certbot) instead of 5; docklift-frontend and docklift-nginx no longer exist.

## [2.1.6](https://github.com/SSujitX/docklift/compare/v2.1.5...v2.1.6) (2026-11-26)


### Bug Fixes

* **deploy:** keep rollback image after stop/restart ([8b210f1](https://github.com/SSujitX/docklift/commit/8b210f1a3dff3e8b05939e2a86882c6905725311))
* **files:** add git metadata path check ([1e5dc5c](https://github.com/SSujitX/docklift/commit/1e5dc5c8e173a96990ba1e2aa345e6ab9f9f84ad))
* **files:** block editor access to .git ([1c4f25c](https://github.com/SSujitX/docklift/commit/1c4f25c0253696e3edd562e9a3b28765bc55fba6))
* **github:** add setup state token validator ([a804829](https://github.com/SSujitX/docklift/commit/a8048291054597a1114823d7a5f35af310e852fc))
* **github:** reject path-like setup state values ([6136057](https://github.com/SSujitX/docklift/commit/61360574902faba9693c31614d66167975ea7923))
* **images:** skip untagged rows when picking keep-2 ([01e4162](https://github.com/SSujitX/docklift/commit/01e4162a9f020490a7ee960318f19460f61c9e07))
* **purge:** keep rollback image after stop/restart ([b9e7ff3](https://github.com/SSujitX/docklift/commit/b9e7ff31a09735fe020cab7db6c3bf5b69fdec8d))

## [2.1.5](https://github.com/SSujitX/docklift/compare/v2.1.4...v2.1.5) (2026-08-10)


### Bug Fixes

* **ssl:** avoid HTTPS drop on domain reorder ([dd6ca3a](https://github.com/SSujitX/docklift/commit/dd6ca3a78bd9b722786b32aa32f2a5bc16704fa6))
* **ssl:** keep partial HTTPS while expanding SANs ([4f29711](https://github.com/SSujitX/docklift/commit/4f29711d3f1ab3e7aa75382d069defa3ff104c85))
* **ssl:** resolve cert lineage by SAN health ([c090e78](https://github.com/SSujitX/docklift/commit/c090e78da06b09ee29b2fdb61474ee6e6032866f))
* **ssl:** use covering cert for panel domains ([48f8f8d](https://github.com/SSujitX/docklift/commit/48f8f8d80fe5c53d8efa5c9e1231daf846c295df))

## [2.1.4](https://github.com/SSujitX/docklift/compare/v2.1.3...v2.1.4) (2026-07-31)


### Bug Fixes

* **system:** clarify purge removes unused docklift images ([97852de](https://github.com/SSujitX/docklift/commit/97852de984f48dd56d1c82b74e76d825e38b51c8))
* **terminal:** clarify purge clears BuildKit cache ([f357d9a](https://github.com/SSujitX/docklift/commit/f357d9a5fb9f519718740d6b47415f07e1de489c))


### Features

* **db:** backfill commit_sha and image_tags columns ([cdde56e](https://github.com/SSujitX/docklift/commit/cdde56e75169597f5b4b080e6fb484d5d6408779))
* **db:** migrate deployment image_tags columns ([270a59b](https://github.com/SSujitX/docklift/commit/270a59be520b753662fb801e6706b74c8348b4b3))
* **db:** store deploy commit sha and image tags ([8ff6f15](https://github.com/SSujitX/docklift/commit/8ff6f15ed10ff0edfbb309f59be3aa3dd32124c1))
* **deploy:** add keep-2 image and BuildKit helpers ([404d760](https://github.com/SSujitX/docklift/commit/404d7601aabfa49d5f869b8f9402d657e311e039))
* **deploy:** keep-2 prune and restore previous API ([d64d84c](https://github.com/SSujitX/docklift/commit/d64d84c3ce1ee9f078883c1b8a4312a658cb2207))
* **frontend:** add deployment commit_sha and image_tags ([ed54e53](https://github.com/SSujitX/docklift/commit/ed54e534f91fdae26726c14b15138fdf93c10616))
* **frontend:** add Restore previous on deployments ([802fb59](https://github.com/SSujitX/docklift/commit/802fb598359144304abff404f7515616c7ef9eb4))
* **git:** expose HEAD sha and hard reset helpers ([cd91146](https://github.com/SSujitX/docklift/commit/cd91146f395cee4960a343ad8f0085741d1995d3))
* **system:** purge unused docklift images and BuildKit ([b7a9df2](https://github.com/SSujitX/docklift/commit/b7a9df24614227e9e359a3b43c7ca96e4d8cc3d9))

## [2.1.3](https://github.com/SSujitX/docklift/compare/v2.1.2...v2.1.3) (2026-07-26)


### Features

* **terminal:** implement shell WS auth deferral during sidebar upgrade confirmation and enhance connection handling ([abccd0f](https://github.com/SSujitX/docklift/commit/abccd0ffa0430f5d58061e3672b43de89bdf1b6a))

## [2.1.2](https://github.com/SSujitX/docklift/compare/v2.1.1...v2.1.2) (2026-07-26)


### Features

* **github:** include app name and slug in status response ([bc623ac](https://github.com/SSujitX/docklift/commit/bc623ac8a91a2d8df1e5aa8863dca9319da63067))
* **settings:** display GitHub app name and slug in integration section ([c063d4d](https://github.com/SSujitX/docklift/commit/c063d4d88ac4c0e1ec13075b3ff4b672c8fad1c2))

## [2.1.1](https://github.com/SSujitX/docklift/compare/v2.1.0...v2.1.1) (2026-07-26)

# [2.1.0](https://github.com/SSujitX/docklift/compare/v2.0.14...v2.1.0) (2026-07-26)


### Bug Fixes

* **shell:** keep sidebar active on nested routes *force minor* ([b71c5cf](https://github.com/SSujitX/docklift/commit/b71c5cfcd3854c3bca4d9a939c278562684a49d3))

## [2.0.14](https://github.com/SSujitX/docklift/compare/v2.0.13...v2.0.14) (2026-07-26)

## [2.0.13](https://github.com/SSujitX/docklift/compare/v2.0.12...v2.0.13) (2026-07-26)


### Bug Fixes

* **shell:** size collapsed new-project like logo ([4df7e52](https://github.com/SSujitX/docklift/commit/4df7e52edac8c095b7177dd10934001f0689f22c))


### Features

* **domain:** paste urls to bare hostnames ([c8ba282](https://github.com/SSujitX/docklift/commit/c8ba2823c7478b7d76d7e7273b944ad3dffda134))
* **domains:** add panel domain card ([cb37563](https://github.com/SSujitX/docklift/commit/cb37563afc4bdb1cd9a839e2455e9a99cac664ca))
* **domains:** add panel ssl events and dns-check ([b562a39](https://github.com/SSujitX/docklift/commit/b562a39590dfe7ab0f147220fa1352632d83363d))
* **domains:** clean pasted hostnames on add ([e1e9c4e](https://github.com/SSujitX/docklift/commit/e1e9c4e76d112106e1b9eec7d1f11409fa90c3b3))
* **domains:** simplify dns guide without accordion ([8870208](https://github.com/SSujitX/docklift/commit/887020854790d233d1bdcf213f21640dfc16579f))
* **frontend:** use singular domain tab label ([723978a](https://github.com/SSujitX/docklift/commit/723978ab11f35a86799ad27654a6b1d246c38ee5))
* **settings:** match project domain panel ux ([3951cbe](https://github.com/SSujitX/docklift/commit/3951cbe1a92896a9f52480a14580d246ae324887))
* **shell:** avatar menu with copyable email ([7017cdf](https://github.com/SSujitX/docklift/commit/7017cdf85e97d8d0e5ac77f7dcd09f359b9dd2fa))
* **website:** add changelog nav and disable html ([a0ffcb9](https://github.com/SSujitX/docklift/commit/a0ffcb9202ef9cc54514b3ed1e373f1bc3699cef))
* **website:** add homepage changelog preview ([555674a](https://github.com/SSujitX/docklift/commit/555674a8dc2d7252da4ff3367afbed69830a090c))
* **website:** load changelog releases for homepage ([1ae2620](https://github.com/SSujitX/docklift/commit/1ae2620ebe29b3543f4726fdbc5ccab8bc9e7514))
* **website:** register changelog preview component ([4056b4f](https://github.com/SSujitX/docklift/commit/4056b4f467610eada09287217b5b4974d711a49a))
* **website:** show changelog preview on home ([da1c536](https://github.com/SSujitX/docklift/commit/da1c5365f8970b76e24392081d7dba46d9f9132c))
* **website:** sync changelog from repo root ([0397293](https://github.com/SSujitX/docklift/commit/03972937ded8c7308acba5a70d0b1fe07db29649))

## [2.0.12](https://github.com/SSujitX/docklift/compare/v2.0.11...v2.0.12) (2026-07-26)


### Bug Fixes

* **deploy:** cancellable timed async image pull ([ccebcf4](https://github.com/SSujitX/docklift/commit/ccebcf4d8e80fbeb7e620a97641bc3e8e908e100))
* **deploy:** own deploy lock by deployment id ([3f2d9d5](https://github.com/SSujitX/docklift/commit/3f2d9d549f0d78a3deae00da65ebc99bc5754ec3))
* **deploy:** serialize cancel with ownership gates ([8302e85](https://github.com/SSujitX/docklift/commit/8302e85cae36ccf8e0e5220a04b4d62742565cd4))
* **deploy:** undo proxy links if cancel wins success-tail ([190d60a](https://github.com/SSujitX/docklift/commit/190d60a7af2d777034b5efcd7747f354a343792a))
* **frontend:** minor file tree polish ([6ff9293](https://github.com/SSujitX/docklift/commit/6ff9293d904889283409393a9f3df64b8dec347b))
* **git:** fail closed when branch or tag missing ([bccadb6](https://github.com/SSujitX/docklift/commit/bccadb6c27ac3ecb85344287ba26903ced6ca39e))
* **github:** match tag pins on webhook refs ([48de2cc](https://github.com/SSujitX/docklift/commit/48de2cc0727db981d160376d8a9c20be5eaf146d))
* **git:** verify tags on remote before reset ([ca68e55](https://github.com/SSujitX/docklift/commit/ca68e55bdbc15ea8114ff2910554f8b85d29c135))
* **nginx:** abort domain writes when deploy lock lost ([3250898](https://github.com/SSujitX/docklift/commit/3250898a80a7e8d089c42502291b081597dd0ab8))


### Features

* **db:** fetch live engine tags from docker hub ([713580e](https://github.com/SSujitX/docklift/commit/713580ee51afdd6753a16f545107f2c8be4a2d52))
* **db:** persist selected version and mount path ([69af7b3](https://github.com/SSujitX/docklift/commit/69af7b35a8743c5a235bd41c4db4808b7947f2fc))
* **db:** version-aware mounts and legacy image pins ([053a523](https://github.com/SSujitX/docklift/commit/053a52375f4fa96ae2e8b67ca4cd7dfde69669f2))
* **frontend:** add account menu ([0b76f2e](https://github.com/SSujitX/docklift/commit/0b76f2ea2a6e0c9bb24245ff251ed7c1506a55a5))
* **frontend:** add project quick actions ([87653ba](https://github.com/SSujitX/docklift/commit/87653babfb49b27065e04ab3b5d4de60c9eb6f1a))
* **frontend:** add project resource cell ([465a9b3](https://github.com/SSujitX/docklift/commit/465a9b3a9a17b5f75da9853b66ce7ab5e272ebf8))
* **frontend:** adjust app shell chrome ([e9a0990](https://github.com/SSujitX/docklift/commit/e9a09906a92151c115de9d00c321d978c1fb0875))
* **frontend:** align new project create flow ([1b7a9fb](https://github.com/SSujitX/docklift/commit/1b7a9fbb383a388c7d9a13f83501cdfd5831fceb))
* **frontend:** allow full-height log viewer ([3e2aaab](https://github.com/SSujitX/docklift/commit/3e2aaabaaa06577910958d7607cf07f121b50ee2))
* **frontend:** compact project action bar ([954033d](https://github.com/SSujitX/docklift/commit/954033d59bc080c31e33b307044ab8d67628ec23))
* **frontend:** database tabs and deploy log select ([814e74f](https://github.com/SSujitX/docklift/commit/814e74fc8990679ae00c188f01d9696c03e62a5d))
* **frontend:** denser operate sidebar ([482d22c](https://github.com/SSujitX/docklift/commit/482d22cc6f975a112b33234354ce5538b78888be))
* **frontend:** fill viewport for system logs ([a98a535](https://github.com/SSujitX/docklift/commit/a98a53574be91f2920258d32ae1c0342e23e0a23))
* **frontend:** fill viewport for terminal ([1356eea](https://github.com/SSujitX/docklift/commit/1356eead9e919e2e3016f2aba025af08db7646b1))
* **frontend:** harden file editor save flow ([141146d](https://github.com/SSujitX/docklift/commit/141146d02a02d4abdd2b09fa8c0bba0511247861))
* **frontend:** improve attach database panel ([8790694](https://github.com/SSujitX/docklift/commit/87906941883cf7025d8c189d85319dc4ca952235))
* **frontend:** include databases in palette ([c9693c5](https://github.com/SSujitX/docklift/commit/c9693c5a2f0f403c9ae3e88007cb2b2f998a5982))
* **frontend:** one-viewport logs page ([5d23071](https://github.com/SSujitX/docklift/commit/5d230710275829e331edf6918049acd18da8c442))
* **frontend:** one-viewport ports page ([2475f6e](https://github.com/SSujitX/docklift/commit/2475f6ec00f6af0cc6503cdef3a64390c65bc728))
* **frontend:** one-viewport terminal page ([0cefca2](https://github.com/SSujitX/docklift/commit/0cefca2f16896b9ad9001b1bbfd33ec7bf56be68))
* **frontend:** polish service switcher ([dae9af1](https://github.com/SSujitX/docklift/commit/dae9af1e89a7f534e0d23ea772661d26a3ff6acb))
* **frontend:** polish sidebar status ([451519b](https://github.com/SSujitX/docklift/commit/451519b1c3a45c292b692303693774ac0f8b351b))
* **frontend:** projects-style databases list ([d2a32c7](https://github.com/SSujitX/docklift/commit/d2a32c7f7109e629b5d4db6704066bca7bce7717))
* **frontend:** raise toast provider ([9921351](https://github.com/SSujitX/docklift/commit/9921351d660c346963b9ade7168e4943250a6061))
* **frontend:** responsive projects dashboard ([4b26d22](https://github.com/SSujitX/docklift/commit/4b26d2298ca1680aa338d8cc6fb8e8dc58109d2d))
* **frontend:** route docs externally and operate pages ([ce268da](https://github.com/SSujitX/docklift/commit/ce268da7177766744472bfb4f9fd5f02efd0f124))
* **frontend:** settings nav from shared ids ([495acc2](https://github.com/SSujitX/docklift/commit/495acc2a8a7909450a4eb6910eb980532bcf286e))
* **frontend:** share settings nav ids ([76d342f](https://github.com/SSujitX/docklift/commit/76d342f4e9508af90d3e2115f07dbee3d1077fca))
* **frontend:** strengthen global toasts ([2a3603b](https://github.com/SSujitX/docklift/commit/2a3603ba987c764482bf06bf8d19f8cefb31b593))
* **frontend:** support tag refs in branch selector ([c9cc81a](https://github.com/SSujitX/docklift/commit/c9cc81a8b10831c979e3916483323a9e59d95eca))
* **frontend:** update shell navigation ([c94c800](https://github.com/SSujitX/docklift/commit/c94c8007b714300f920d297e0970c28b33e42cde))
* **frontend:** version picker on new database ([fbcd5f9](https://github.com/SSujitX/docklift/commit/fbcd5f99c2048105a3e8e041881dedf7270b7620))
* **frontend:** wire account menu in top bar ([e9385c5](https://github.com/SSujitX/docklift/commit/e9385c5a0ae8dc537604570772cb018ea390b0c7))
* **github:** improve branch and tag listing ([c1247f4](https://github.com/SSujitX/docklift/commit/c1247f4d0b8d7bf31195bbef851c36e05dfb8051))
* **ports:** expose private running projects ([9be11d3](https://github.com/SSujitX/docklift/commit/9be11d3abcfe6d5a8111f31312ce8b496212b1af))

## [2.0.11](https://github.com/SSujitX/docklift/compare/v2.0.10...v2.0.11) (2026-07-25)


### Features

* **compose:** support command and skip PORT env ([a4598eb](https://github.com/SSujitX/docklift/commit/a4598ebdc1fb0da7846397937e67da40f2f21b6c))
* **db:** add engine field and database links ([88418aa](https://github.com/SSujitX/docklift/commit/88418aac58974b54078d88a49130dc63e6fe4cff))
* **db:** catalog official database engines ([48e9802](https://github.com/SSujitX/docklift/commit/48e980257bad4ff51d33d3b7d23fc9a420a089ac))
* **db:** link cleanup and credential env guards ([fd4e20e](https://github.com/SSujitX/docklift/commit/fd4e20e56cebc36a4e8168850251ccc461cbe9bd))
* **db:** link inject and network attach helpers ([d9df90f](https://github.com/SSujitX/docklift/commit/d9df90fc5f5a9bdcca25cd6f985b18ceff82f502))
* **db:** managed database create and link API ([08bdfce](https://github.com/SSujitX/docklift/commit/08bdfced8cec4cf10a264a57c5173e40a47a4d2c))
* **db:** migrate managed databases schema ([de3e5fc](https://github.com/SSujitX/docklift/commit/de3e5fcac642040481affb805ff1d3e502e2df7a))
* **db:** mount databases router ([4385532](https://github.com/SSujitX/docklift/commit/4385532efc9161497a12ab98715685f13aad3795))
* **db:** repair db_engine and link indexes ([e8f876a](https://github.com/SSujitX/docklift/commit/e8f876a624ca8e34338dc138de3acaf292dbb814))
* **db:** unique app env key per database link ([885dfe7](https://github.com/SSujitX/docklift/commit/885dfe7ab9277a4233914f6e63805918ad3f9a16))
* **deploy:** pull and run managed database images ([f8c8b10](https://github.com/SSujitX/docklift/commit/f8c8b10c4c7e47df39c6d24327fce0c00f5708a0))
* **docker:** attach containers to project networks ([7137abe](https://github.com/SSujitX/docklift/commit/7137abe106f9af55ffee1724a3e9c15cce7b2193))
* **frontend:** add databases new route ([dec97de](https://github.com/SSujitX/docklift/commit/dec97debcc61b05fa7635ea6a1f9fa06d8bb9c29))
* **frontend:** add managed database project fields ([f9a17ae](https://github.com/SSujitX/docklift/commit/f9a17aee0f3c0029178ba243f2542a4588c55147))
* **frontend:** add new database page ([4434505](https://github.com/SSujitX/docklift/commit/4434505f6a8989451abb2009b3664587581271b4))
* **frontend:** attach database to app panel ([17dd4ff](https://github.com/SSujitX/docklift/commit/17dd4ff530e0da6a9f5fe13c86a11d948a2cc5c4))
* **frontend:** database connection and link panel ([cb7857a](https://github.com/SSujitX/docklift/commit/cb7857ab4f3a19d0572156537ef0260ed8bed336))
* **frontend:** route databases list to create flow ([168707b](https://github.com/SSujitX/docklift/commit/168707b122d0ba466d821191ac48ea97a9332da2))
* **frontend:** wire managed database panels ([2307840](https://github.com/SSujitX/docklift/commit/2307840361dc1808cd63d4edc440703fdeeb8dab))

## [2.0.10](https://github.com/SSujitX/docklift/compare/v2.0.9...v2.0.10) (2026-07-25)


### Features

* **LogViewer:** add copy functionality for logs with user feedback and handling for large log buffers ([966a2ea](https://github.com/SSujitX/docklift/commit/966a2ea4118416ad7c587dd2e88c8b424de82cf4))

## [2.0.9](https://github.com/SSujitX/docklift/compare/v2.0.8...v2.0.9) (2026-07-25)


### Features

* **auth:** enhance password prompt for terminal actions with customizable options ([4aba31a](https://github.com/SSujitX/docklift/commit/4aba31a77aae5ecc7fe818f10d5443fcb8d43595))
* **auth:** implement step-up password requirement for system actions ([52d57a4](https://github.com/SSujitX/docklift/commit/52d57a4c27a2526cfa69e637d50477a5641f8d88))
* **auth:** update WebTerminal component to include step-up password requirement for host actions and enhance upgrade dialog with password confirmation ([14eac7c](https://github.com/SSujitX/docklift/commit/14eac7c67ba7b17029f14c1e20a9cc772848ef2a))

## [2.0.8](https://github.com/SSujitX/docklift/compare/v2.0.7...v2.0.8) (2026-07-25)


### Bug Fixes

* **ui:** update LogViewer height for better responsiveness ([f28ef18](https://github.com/SSujitX/docklift/commit/f28ef184af70fb1664478b47f30c87a91f94d195))


### Features

* **ui:** enhance ServiceSwitcher with scroll functionality and improved service selection ([6636a63](https://github.com/SSujitX/docklift/commit/6636a635c956c4b78b0ff8a3583de2a56350e0ea))

## [2.0.7](https://github.com/SSujitX/docklift/compare/v2.0.6...v2.0.7) (2026-07-25)


### Features

* **ui:** enhance ProjectActionBar with improved styling and new danger action ([262452f](https://github.com/SSujitX/docklift/commit/262452fe2f96a3bdfe69f4b57fc84c6a6429d2b8))

## [2.0.6](https://github.com/SSujitX/docklift/compare/v2.0.5...v2.0.6) (2026-07-25)


### Bug Fixes

* **db:** drop legacy env unique after scope migrate ([65093a0](https://github.com/SSujitX/docklift/commit/65093a08a360f30af225fc3abbe1a2c11e41df6a))


### Features

* **api:** accept service-scoped env CRUD ([5b82a2a](https://github.com/SSujitX/docklift/commit/5b82a2a7e3955339684454adabe331fa049ea730))
* **compose:** pass per-service env into compose ([32025fa](https://github.com/SSujitX/docklift/commit/32025fa3d7e916676f19cabc356d60c00c2694eb))
* **db:** migrate env to service-scope unique ([b860238](https://github.com/SSujitX/docklift/commit/b86023865ddedcdd554aea0417b4daeaff5b379e))
* **db:** scope env vars by service name ([6fc6738](https://github.com/SSujitX/docklift/commit/6fc673862290fef5cdc6537b1e246cadbfd32774))
* **deploy:** inject merged service env on deploy ([fe6225b](https://github.com/SSujitX/docklift/commit/fe6225b3ea4eebaaa6ed414a05f5cc98fde9472e))
* **env:** merge shared and service-scoped vars ([c7c6634](https://github.com/SSujitX/docklift/commit/c7c6634168d0436cfabfb0996c56792eee0aa676))
* **env:** scope env manager by service ([10d7c04](https://github.com/SSujitX/docklift/commit/10d7c042dcf17c583f16a136feaa6a4cda82d02f))
* **types:** add env service_name field ([1acef2b](https://github.com/SSujitX/docklift/commit/1acef2bbb0931d0df09ed5e4ccbf0801c43858fe))
* **ui:** add all-services action bar ([915f874](https://github.com/SSujitX/docklift/commit/915f8742dce382794ed7da564ca684ae70d463fe))
* **ui:** add all-services workspace rail ([a917b1d](https://github.com/SSujitX/docklift/commit/a917b1d67743e0491a5caed5f50fcbf5005ebcda))
* **ui:** add bare button variant ([61b49a1](https://github.com/SSujitX/docklift/commit/61b49a10e8a228aa3d5333734d394db6d8a4d5d1))
* **ui:** label dashboard project lifecycle actions ([262702c](https://github.com/SSujitX/docklift/commit/262702ce2c7e2c800a3ddfa0e1e1355ce582df09))
* **ui:** split all-services vs service workspace ([b2786f7](https://github.com/SSujitX/docklift/commit/b2786f7cee8ecde68341686deee9a69cde06b735))

## [2.0.5](https://github.com/SSujitX/docklift/compare/v2.0.4...v2.0.5) (2026-07-25)


### Features

* **sidebar:** route upgrade through terminal confirm ([107f971](https://github.com/SSujitX/docklift/commit/107f971f900c040321d3a6f6e7a4049d6f70a28b))
* **terminal:** quiet host strip and upgrade wait dialogs ([b9c8c69](https://github.com/SSujitX/docklift/commit/b9c8c69effba7fe914944c968c522f5196a4e0c4))
* **ui:** allow hiding dialog close button ([ef38ddb](https://github.com/SSujitX/docklift/commit/ef38ddbc91b995335d72e810cdb7648babbbd7a4))

## [2.0.4](https://github.com/SSujitX/docklift/compare/v2.0.3...v2.0.4) (2026-07-25)


### Features

* **install:** pin release via v= or env ([47fdac6](https://github.com/SSujitX/docklift/commit/47fdac619bfe9e1372e5ff4f1e7ee99343658655))

## [2.0.3](https://github.com/SSujitX/docklift/compare/v2.0.2...v2.0.3) (2026-07-25)


### Bug Fixes

* **system:** stop caching stale no-update checks ([d92f05d](https://github.com/SSujitX/docklift/commit/d92f05d620953cc59a93cd095e0b2dbe13c4290b))
* **ui:** refresh version banner without github spam ([d02911d](https://github.com/SSujitX/docklift/commit/d02911d203c3ef62a2aa19c4493238fe0e009df0))

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
