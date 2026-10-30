## [0.14.2](https://github.com/holistics/anfra/compare/v0.14.1...v0.14.2) (2026-10-30)

### Features

* **appserve:** wear @holistics/anfra-design's tokens, fonts and type ([0530e9a](https://github.com/holistics/anfra/commit/0530e9a8d3624df9ccd53a151c2a80888d559db8))
* **design:** add @holistics/anfra-design, one design language for core and Cloud ([77b2790](https://github.com/holistics/anfra/commit/77b27904fcd9ba2ca00253faf4aa045e0026a430))
* **sdk.common:** give plugin UI anfra-design's tokens, and one mapping to them ([5125a4a](https://github.com/holistics/anfra/commit/5125a4a40e8f3232a1f9117b9dd95bdaf9845015))
* **sdk.host:** move pick into the panel's strip as a panel command ([7a4312f](https://github.com/holistics/anfra/commit/7a4312fe37a8074ac7df531a1252e2bda9adba25))
* **sdk:** draw inspect's states with the theme's hover, sunken, selected and status tokens ([a8ef627](https://github.com/holistics/anfra/commit/a8ef62791dbc7b7d41bb0d3bf73e81280fcb878f))

### Bug Fixes

* **appserve:** draw the palette's on mark in the accent's text colour ([8220549](https://github.com/holistics/anfra/commit/8220549013891db669c2f55e120da8d287e46658))
* **design:** give type.css every spacing scale DESIGN.md names ([04ac184](https://github.com/holistics/anfra/commit/04ac18415710ff256360072a988d029d92bf6ed9))
* **sdk.app:** fall back to anfra-design's light colours in the app's menus and modals ([9dc2e61](https://github.com/holistics/anfra/commit/9dc2e61c8a200f13c00e81437b071ebc6f6aa885))
* **sdk.app:** round the plugin layer's surfaces at xl, the theme radius plus 4px ([a7ab76f](https://github.com/holistics/anfra/commit/a7ab76f7fe830b0a1865291b960c55a000658455))
* **sdk.common:** mix the newer theme tokens from a host's older ones ([b66aaf6](https://github.com/holistics/anfra/commit/b66aaf6ac3e88e14670db50c78c42369d18203da))

### Security

* bump vitest to 4.1.12 for @vitest/mocker (W44) ([d351b47](https://github.com/holistics/anfra/commit/d351b474a00f50b0444f56e831568eff26f82d25))
## [0.14.1](https://github.com/holistics/anfra/compare/v0.14.0...v0.14.1) (2026-10-28)

### Bug Fixes

* **engine:** bump to support relative path in semantic catalog ([2e27130](https://github.com/holistics/anfra/commit/2e2713080c1d41800173423c4ee6f1eaaf4bb8c3))
## [0.14.0](https://github.com/holistics/anfra/compare/v0.13.5...v0.14.0) (2026-10-28)

### ⚠ BREAKING CHANGES

* **sdk:** inspect logs the frame's calls, and gets a rich Queries tab
* **sdk:** the inspect.watch, inspect.highlight, inspect.locate
and inspect.pick calls and the inspect.snapshot and inspect.picked
events are gone; use mounted.panel instead.

### Features

* **appserve:** a command palette over the shell's and the plugins' commands ([587a914](https://github.com/holistics/anfra/commit/587a914c65d3994115efc11984b1b236e8783cfe))
* **appserve:** draw the plugins' commands and panel tabs, dropping the Vue inspector ([463281a](https://github.com/holistics/anfra/commit/463281a7226a28070c30b5c74c40694ea6a64609))
* **appserve:** group the panel's tabs that don't fit behind a More tabs menu ([28dc60d](https://github.com/holistics/anfra/commit/28dc60d962f538b8069a53cf012a3652bd4bf233))
* **appserve:** scan-search for Inspect, and an icon before each panel tab's title ([40ea3f3](https://github.com/holistics/anfra/commit/40ea3f36a1535ca0ca1e73aae3b5b7be9767783d))
* **appserve:** the palette in the sidebar, Reload first, and a Download menu for render's commands ([fdedde4](https://github.com/holistics/anfra/commit/fdedde4ae1505f9095976bcb07574a2f14b3e848))
* **sdk.host:** add command and tab contributions from plugins and the host ([e5b53ee](https://github.com/holistics/anfra/commit/e5b53ee13e7dc98c9a285f9b2ce3a39d39206a5b))
* **sdk.host:** give a tab its mount, and a mount its reader and datasets ([6cdf0f1](https://github.com/holistics/anfra/commit/6cdf0f13ad3c22cd57a5cf9eeca9ce82089db79c))
* **sdk:** a menu on blocks and on query rows, and modals and popovers in the frame ([7af77df](https://github.com/holistics/anfra/commit/7af77df2853f411d1412f40f8d7033dcb833321a))
* **sdk:** give every mount a panel, with the frame's snapshot, pick and highlight ([aac3027](https://github.com/holistics/anfra/commit/aac3027d9906075a9aa61de550f6f314f6438c30))
* **sdk:** hover tooltips in inspect's tabs, in place of click popovers and native titles ([91c0ce3](https://github.com/holistics/anfra/commit/91c0ce370784117728083cc62d89b4ee6ce4611c))
* **sdk:** inspect logs the frame's calls, and gets a rich Queries tab ([4178c56](https://github.com/holistics/anfra/commit/4178c560d80ac4a62c70cb8c1b7de770209212a9))
* **sdk:** inspect's Data tab, each query's latest rows, with a CSV download ([b078641](https://github.com/holistics/anfra/commit/b07864177f11b26bb5a785af2bd8935be0fe4d3a))
* **sdk:** inspect's Declaration tab: counts, checks, the code and the wiring ([5fb21bb](https://github.com/holistics/anfra/commit/5fb21bb12ed96e5714f33fcd3141f6dbea248663))
* **sdk:** inspect's Overview tab, where the panel opens: trust, issues, changes ([003878d](https://github.com/holistics/anfra/commit/003878de79e3daee57eb79834becda688df7295a))
* **sdk:** inspect's Structure and Data tabs and its Inspect command ([7ed89d8](https://github.com/holistics/anfra/commit/7ed89d8b798b59a27ba149e9792b3c4635922b5c))
* **sdk:** inspect's tabs as tiles, with type icons and one type scale ([5b721a9](https://github.com/holistics/anfra/commit/5b721a94608de000ffcf99357c1632ccf9ff7e4f))
* **sdk:** lineage's graph of the page and its layered layout ([5e078bf](https://github.com/holistics/anfra/commit/5e078bf2eb38641b4bcf0608708e6e6def22d807))
* **sdk:** lineage's Lineage tab, a canvas of where the page's numbers come from ([2a170e2](https://github.com/holistics/anfra/commit/2a170e20218b89e0324c34953186fdb5c6de0b89))
* **sdk:** put Lineage after Queries, and draw it as a fork ([b5bad3f](https://github.com/holistics/anfra/commit/b5bad3f42f79f9813b2f0aedd248380840decbdb))
* **sdk:** render offers Download as PNG and PDF as commands, which appserve draws ([6297dba](https://github.com/holistics/anfra/commit/6297dbaa1e0c6ca7a070de244bcaf99cacbd75ae))
* **sdk:** run commands by shortcut, forwarded from the frame ([51105e0](https://github.com/holistics/anfra/commit/51105e03db19ebc25f6ea14c5e120478ea9d3c0c))
* **sdk:** show a declaration as its JSON, with its AQL highlighted across its lines ([97a9981](https://github.com/holistics/anfra/commit/97a9981ea0e267d1a61f4c454dcaf12337d31f71))
* **sdk:** show a query in Lineage from its row in Queries and its card in Data ([fa0fdcf](https://github.com/holistics/anfra/commit/fa0fdcf2e01d7534868a0be468e4e0b5751ea7be))
* **sdk:** show a query or a node in Lineage from inspect's tabs ([d89e3fa](https://github.com/holistics/anfra/commit/d89e3fad9517b421ffcf0ab0e1bace3f21236c0b))
* **sdk:** show a query's signature as the Declaration shows its JSON ([1c0d9cf](https://github.com/holistics/anfra/commit/1c0d9cf77887b95daaeace85ffb5358e12d22db6))
* **sdk:** theme tokens for plugin tabs, the plugin layer and Anfra.theme ([4c5f0a9](https://github.com/holistics/anfra/commit/4c5f0a90df8fbcf0011e6ee91d493e0d202f79d5))

### Bug Fixes

* **appserve:** drop a Data App's mount when it goes, and redraw the panel for the next ([25babb3](https://github.com/holistics/anfra/commit/25babb381ca476d5f8540af22b2ccf7f05021c1a))
* **appserve:** keep commands for the page's menu out of the toolbar ([9b9aab4](https://github.com/holistics/anfra/commit/9b9aab4ce1012f960e4ea37217df805e3e92dbf7))
* **appserve:** let the panel's tab strip scroll when its tabs don't fit ([d4e2703](https://github.com/holistics/anfra/commit/d4e27037bd810b15afeb85939c37292d1b0d44c5))
* **appserve:** open the More tabs menu leftward, so the window doesn't cut it off ([8a981c9](https://github.com/holistics/anfra/commit/8a981c9942fb37215e695ddb42c6ada87878e213))
* **sdk.app:** keep the browser's menu when no frame item applies ([b9a1fc6](https://github.com/holistics/anfra/commit/b9a1fc66024be1261628fa855dd4efcb89c85d65))
* **sdk.host:** check a snapshot's every node, and open pick mode at a tab ([344714a](https://github.com/holistics/anfra/commit/344714a1e37aa4c5215c97117d3b8b6871edf503))
* **sdk.host:** hold a forwarded shortcut to the frame's calls ([ba547aa](https://github.com/holistics/anfra/commit/ba547aac5139eddf338271cc67c7e9f8176619a9))
* **sdk.host:** keep the policy ahead of a definition's scripts however it starts ([9d7825d](https://github.com/holistics/anfra/commit/9d7825d8dc0b44f7f34c8a29a4a55318a64366b7))
* **sdk:** always show a section's chevron ([2375e44](https://github.com/holistics/anfra/commit/2375e448cc71d9f4b2427da59d9bbb6121af84f6))
* **sdk:** centre the lineage legend's button ([029a8ea](https://github.com/holistics/anfra/commit/029a8eabb6bbfde864e5f0978e6b48547c3ae567))
* **sdk:** draw the frame's menu, modal and popover in the top layer, and keep a modal's page inert ([89caf2c](https://github.com/holistics/anfra/commit/89caf2ccb6fa2f1ad0b1c479070fecae90398ce2))
* **sdk:** give a popover a close button, and focus so Escape closes it ([387a095](https://github.com/holistics/anfra/commit/387a0954ca86e76e0c09f78bfe08ec6977c88f2c))
* **sdk:** inspect's tabs at 13px, as before ([fe44852](https://github.com/holistics/anfra/commit/fe4485278f81caf0a8f189c660927b8cd17f4cc0))
* **sdk:** keep Download disabled while the frame draws ([acd0515](https://github.com/holistics/anfra/commit/acd0515ec16129c053e5828aed2901e4a81f8577))
* **sdk:** keep the page's scrollbar room under a modal, and open popovers on a side ([702b6bd](https://github.com/holistics/anfra/commit/702b6bda870c9d39fe3dc68b3f1e83473e115cbe))
* **sdk:** mute only Structure's labels, not Queries' app sections ([663528a](https://github.com/holistics/anfra/commit/663528aec7e1014bb8347d402f7b4b7ef56dad1e))
* **sdk:** one highlight in the menu, moved by the pointer and the arrows ([e66dc6e](https://github.com/holistics/anfra/commit/e66dc6e48c66b20872b3dde2756eabbbb95b9246))
* **sdk:** open lineage's Show menu in the top layer, inside the window ([8276a15](https://github.com/holistics/anfra/commit/8276a15cbba0877fc57d6c594deb0abbf84aee2d))
* **sdk:** put inspect's popovers in the top layer, kept inside the window ([0afdcd1](https://github.com/holistics/anfra/commit/0afdcd1bb7ab40cb34962b36875e631f3a2beb4c))
* **sdk:** set apart the queries and controls a declaration check names ([e3862a1](https://github.com/holistics/anfra/commit/e3862a1be2b70b6aa8a51156e994bcf8223c54ba))
* **sdk:** space Data's tables apart ([411e99d](https://github.com/holistics/anfra/commit/411e99d8cecd284126b080d8dd53ec992cdceef5))
## [0.13.5](https://github.com/holistics/anfra/compare/v0.13.4...v0.13.5) (2026-10-28)

### Features

* **engine.sidecar:** bump anfra-node to support lookml and improve search ([f3cefa9](https://github.com/holistics/anfra/commit/f3cefa9f6032c6ac77aef56f146136903dead46f))
* **engine:** improve search command output in CLI ([851ba1c](https://github.com/holistics/anfra/commit/851ba1cc2677ade479ec0c0c9d52a65fff572d49))
* **engine:** make search accept offset and limit ([f010016](https://github.com/holistics/anfra/commit/f0100165af84e7fe0934471d6b9bbc6ce241f5c1))
## [0.13.4](https://github.com/holistics/anfra/compare/v0.13.3...v0.13.4) (2026-10-26)

### Features

* dedup nested error messages ([f620318](https://github.com/holistics/anfra/commit/f6203185fa6245f6e854c0e24406f8c5ab11e4ed))
* expose and log internal errors (toggleable by env flag) ([e2755f7](https://github.com/holistics/anfra/commit/e2755f74c4b9beafc0e92e21bf749421471fba2f))

## [0.13.3](https://github.com/holistics/anfra/compare/v0.13.2...v0.13.3) (2026-10-22)

### Bug Fixes

* **sdk.app:** take a host's messages from its origin, not only the parent ([9525df4](https://github.com/holistics/anfra/commit/9525df44ddb11efe587503c50308657b6291f331))
## [0.13.2](https://github.com/holistics/anfra/compare/v0.13.1...v0.13.2) (2026-10-21)

### Bug Fixes

* **engine:** fix engine version when being imported ([7ab0774](https://github.com/holistics/anfra/commit/7ab0774daabf4cb01ced5428f445d6d913c09c30))

### Performance Improvements

* **cli:** reduce motherduck cli calls ([af754e0](https://github.com/holistics/anfra/commit/af754e02c47a245aab65acd95ef9b32e6373f979))
## [0.13.1](https://github.com/holistics/anfra/compare/v0.13.0...v0.13.1) (2026-10-21)

### Features

* **cli:** read an org's allowed Data App origins from anfra Cloud ([efe50a0](https://github.com/holistics/anfra/commit/efe50a0f1bcbd67de53552f13462ef7c325a75b5))
* **cli:** warn on publish of Data App origins the org does not allow ([032966f](https://github.com/holistics/anfra/commit/032966f1e7a38f2c95a150e8ffb37cbb3a184148))
* **sdk.host:** load Data App libraries from jsDelivr and cdnjs only ([2667bc0](https://github.com/holistics/anfra/commit/2667bc0d9f188f74efbf9f28c21ff271ce9dc8c5))
## [0.13.0](https://github.com/holistics/anfra/compare/v0.12.0...v0.13.0) (2026-10-21)

### Features

* **cli:** support uploading duckdb to motherduck during publish ([00e8e4a](https://github.com/holistics/anfra/commit/00e8e4a2a4441dbf1429c07cc0929ed2b3f708df))
## [0.12.0](https://github.com/holistics/anfra/compare/v0.11.1...v0.12.0) (2026-10-21)

### ⚠ BREAKING CHANGES

* **sdk.host:** MountOptions.baseHref is removed and no <base> is
injected: a Data App is one self-contained file.

### Features

* bump anfra-node to 0.9.0 and improve its error handling ([b120367](https://github.com/holistics/anfra/commit/b120367c169bca1db74d0009c4d8e1b635a331f2))
* **cli:** add durable oauth login and credential management ([88adbf1](https://github.com/holistics/anfra/commit/88adbf116edc877a8a1a6162bb6be24daa60f5fd))
* **cli:** add interactive organization picker and auth progress ([bcf5a35](https://github.com/holistics/anfra/commit/bcf5a35efa402448c7921026922917a469d532ad))
* **cli:** align cloud contract and canonical organization urls ([cfd35d6](https://github.com/holistics/anfra/commit/cfd35d6c40a2e2eaa4a49f93f354f118a9a54009))
* **cli:** warn about Data App references a host cannot serve ([04e7f2b](https://github.com/holistics/anfra/commit/04e7f2baedeaa4ccfdee2bbd9eb15ff912122335))
* **cli:** wire auth commands and bearer-authenticated publishing ([a04f0f2](https://github.com/holistics/anfra/commit/a04f0f2463820f9724a35427a30ac5dce8654373))
* formalize background routines ([bcfecd4](https://github.com/holistics/anfra/commit/bcfecd4e1c3e6ba90bf12e6fd27c1cd317fbbca0))
* improve publish cliui ([0c5cc97](https://github.com/holistics/anfra/commit/0c5cc97d068385e082792f1e7a191c9c4bb47c16))
* **sdk.host:** run Data App frames under a content policy ([9654654](https://github.com/holistics/anfra/commit/96546542b82357460fde62d8be4bb530f46c029d))
* **shared:** allow hosts to set error response headers ([2b3efcb](https://github.com/holistics/anfra/commit/2b3efcb77fcc2625db010cb4556434fd7a5bedc8))
* **shared:** check Data App origins and definitions in repoformat ([3976499](https://github.com/holistics/anfra/commit/39764998eb893e62b0419a52b9eff8bac54ad5a7))
* **shared:** expose optional oauth scope requirements in discovery ([804a645](https://github.com/holistics/anfra/commit/804a6457fa4b63ad23ea9ebd5f8449a25978ff67))
* **shared:** name Data Apps in repoformat, for every host ([e53d725](https://github.com/holistics/anfra/commit/e53d725e8200ebcf29152c7e6f4b1f41b9e46c7e))

### Bug Fixes

* **cli:** streamline organization authentication and terminal cleanup ([eb4f148](https://github.com/holistics/anfra/commit/eb4f148dd81dddd7611a1b42e36cf6f7763adab2))
* **sdk.host:** keep a definition's html and head attributes under the policy ([6b2d07d](https://github.com/holistics/anfra/commit/6b2d07d486e432a5e4304ff74a4f4d564e50576d))
* **shared:** read a tag's attributes only when it has some ([5574eea](https://github.com/holistics/anfra/commit/5574eeacaed08556d8f8663eb15f564d4a4c035d))

### Build

* **cli:** validate and embed the optional default cloud url ([13e6cb7](https://github.com/holistics/anfra/commit/13e6cb72ab8ba3fc393e788044a0daaa75bd9a6d))

### Security

* bump brace-expansion and handlebars (W42) ([ea31570](https://github.com/holistics/anfra/commit/ea315706601d2ba8b32b04525a34934e9c4f0932))
* bump tsup to 9 to clear esbuild 0.27.7 (W42) ([9838b7c](https://github.com/holistics/anfra/commit/9838b7ce8fc84ca455ee9b280d7ae0ea557228de))
## [0.11.1](https://github.com/holistics/anfra/compare/v0.11.0...v0.11.1) (2026-10-19)

### ⚠ BREAKING CHANGES

* **engine:** revamp to authorize on typed input
* refactor and introduce executor

### Features

* add query result row cap ([cbfda62](https://github.com/holistics/anfra/commit/cbfda62db26af98485c8606dba0aafb582b0ae4a))
* let host provide the run timeout ([7044896](https://github.com/holistics/anfra/commit/7044896a3e45cd98ec10067e40cb09fcc2404f17))
* pass tenant id to canal-query ([621f23d](https://github.com/holistics/anfra/commit/621f23d50a43205783b21320fbee3860419939a1))
* report compiled AQL for observer ([2682f41](https://github.com/holistics/anfra/commit/2682f411a65581278e4032e9dd72203b29525c59))
* report query run to observers ([e1e2f06](https://github.com/holistics/anfra/commit/e1e2f06d7eb6c62d70b34c21b0e1aedbb885a980))
* stream query result ([8ab3da0](https://github.com/holistics/anfra/commit/8ab3da06a5e406718a9ab824c135e8c0bf3c05cd))
* streaming query ([6c9be5d](https://github.com/holistics/anfra/commit/6c9be5df447846162d81a3a384b4b33f031871a3))

### Bug Fixes

* only show query_failed (422) when error scope is 'User' ([e3f8146](https://github.com/holistics/anfra/commit/e3f814628cde360979efa56cc72e1c7e4f75b70e))
* **sdk.app:** keep Anfra.features as a deprecated view of the installed plugins ([d4760c5](https://github.com/holistics/anfra/commit/d4760c509dc3b04ed8384c588d965cc96d36eaf2))
* **shared:** execute every execution, even when the client left before start ([814e429](https://github.com/holistics/anfra/commit/814e42931ba4009591db73b253b57cd84cd79692))

### Miscellaneous Chores

* **engine:** revamp to authorize on typed input ([10f5e1a](https://github.com/holistics/anfra/commit/10f5e1a3c103b8a9b059c1489c253d4e7c17f5bb))
* refactor and introduce executor ([3f59172](https://github.com/holistics/anfra/commit/3f5917290968463d302816464a85a3aff5438eef))
## [0.11.0](https://github.com/holistics/anfra/compare/v0.10.2...v0.11.0) (2026-10-19)

### ⚠ BREAKING CHANGES

* **sdk:** mountDataApp and serveBridge are replaced by
createHost(...).mount(...), and the Backend option by corePlugin.
Anfra.features is removed: a plugin's API is absent where it is not
installed, so check Anfra.plugins.has(name). The bridge's messages
changed.

### Features

* **appserve:** mount data apps through the plugin host runtime ([269d305](https://github.com/holistics/anfra/commit/269d305e504d6baea0288c3138acbac3432c2545))
* **sdk.host:** carry who started each call, and let a plugin call as its origin ([07813ed](https://github.com/holistics/anfra/commit/07813ed07d9f7977f5d11eb0955c725b7c5b8386))
* **sdk:** add build and testing entrypoints for plugin authors ([11b2e9e](https://github.com/holistics/anfra/commit/11b2e9e68b49905c259fd5af3943692699f6faf2))
* **sdk:** build data app features as plugins on a host runtime ([39caea2](https://github.com/holistics/anfra/commit/39caea2f5b0d47aaedb60d9d13d25b07cf23b574))
* **sdk:** type plugin members from one augmentable registry ([19e848d](https://github.com/holistics/anfra/commit/19e848db064146bf7b2fc51585e171eeb6b36c99))

### Bug Fixes

* **sdk.host:** check what crosses from the frame, and every plugin ([183a773](https://github.com/holistics/anfra/commit/183a773e95798680ee545d46022ce2d953bf98a3))
## [0.10.2](https://github.com/holistics/anfra/compare/v0.10.1...v0.10.2) (2026-10-16)

### Bug Fixes

* bump canal-query to 2.19.1 to avoid duckdb stuck loop ([48c1cd2](https://github.com/holistics/anfra/commit/48c1cd28b11b73b235b3a7e1c3e3caa449ed28c4))
## [0.10.1](https://github.com/holistics/anfra/compare/v0.10.0...v0.10.1) (2026-10-16)

### Features

* disable AQL v2 compiler ([94d1bf2](https://github.com/holistics/anfra/commit/94d1bf265d50843c747aa164e6422c18b2ec66e7))
## [0.10.0](https://github.com/holistics/anfra/compare/v0.9.1...v0.10.0) (2026-10-16)

### Features

* separate envs for sidecars and support serving pprof ([2516521](https://github.com/holistics/anfra/commit/25165215797dd90058f91395ff3decfd69c78813))
* support data source settings ([6ddb716](https://github.com/holistics/anfra/commit/6ddb7168bbeb98577f0db25b90deb1d6e0b1729a))
## [0.9.1](https://github.com/holistics/anfra/compare/v0.9.0...v0.9.1) (2026-10-15)

### Bug Fixes

* fix incorrect anfra-node version ([d3fd713](https://github.com/holistics/anfra/commit/d3fd71327e4f7d4bcfd2a95b56f0ac5c5d73d282))
## [0.9.0](https://github.com/holistics/anfra/compare/v0.8.0...v0.9.0) (2026-10-14)

### Features

* **engine:** add ingest context for lineage and search ([5c6510f](https://github.com/holistics/anfra/commit/5c6510f62db0ee533992f49228933c8074559102))
* **engine:** better types ([fa960a0](https://github.com/holistics/anfra/commit/fa960a0154f2da9d4bca5a4d35592563c8d019f0))
* lineage sdk ([4e3f213](https://github.com/holistics/anfra/commit/4e3f21333aeafd19ab98d9d12767f71348fe9e90))
* minor improvements and refactoring ([d916b32](https://github.com/holistics/anfra/commit/d916b32cb72aad047f27feb06b5b60389533f07c))
* **sdk:** fix copywritings ([58e49a8](https://github.com/holistics/anfra/commit/58e49a89b93b13050bf24cfcdf1db10ad6392fc5))
## [0.8.0](https://github.com/holistics/anfra/compare/anfra-v0.7.1...anfra-v0.8.0) (2026-10-13)

### Features

* **sdk:** export a query's result from a Data App ([5ae069e](https://github.com/holistics/anfra/commit/5ae069e6255658d7a730227fb4d6d8e328757b2f))
## [0.7.2](https://github.com/holistics/anfra/compare/anfra-v0.7.1...anfra-v0.7.2) (2026-10-13)

### Features

* **sdk:** export a query's result from a Data App ([5ae069e](https://github.com/holistics/anfra/commit/5ae069e6255658d7a730227fb4d6d8e328757b2f))
## [0.7.1](https://github.com/holistics/anfra/compare/anfra-v0.7.0...anfra-v0.7.1) (2026-10-12)

### Features

* **appserve:** row actions and a pinned detail pane in the Structure tab ([a6f0cd7](https://github.com/holistics/anfra/commit/a6f0cd703f57b6cd317e4c62be3ab5dbe594ae51))

### Bug Fixes

* **appserve:** scope the Structure tab's styles to its own tree ([5a0e298](https://github.com/holistics/anfra/commit/5a0e298819803cbafdcd1cd716633622f6c8f741))
## [0.7.0](https://github.com/holistics/anfra/compare/anfra-v0.6.1...anfra-v0.7.0) (2026-10-12)

### ⚠ BREAKING CHANGES

* revamp query result data handling
* use json v2 to fix null arrays and avoid similar mistakes

### Features

* add query.export function ([050878d](https://github.com/holistics/anfra/commit/050878de72a168c002056d7879a0683c41530860))
* **cli:** dotted form for CLI object args ([023977f](https://github.com/holistics/anfra/commit/023977fcc571e30f0bf114cf6de7512da767c51d))
* revamp query result data handling ([57a4629](https://github.com/holistics/anfra/commit/57a46294dcb8396c9306a6218f410baa3d9c0e70))

### Bug Fixes

* use json v2 to fix null arrays and avoid similar mistakes ([49d55a8](https://github.com/holistics/anfra/commit/49d55a811db0684f1832f2c762df0c93ef15df3d))
## [0.6.1](https://github.com/holistics/anfra/compare/anfra-v0.6.0...anfra-v0.6.1) (2026-10-12)

## [0.6.0](https://github.com/holistics/anfra/compare/anfra-v0.5.0...anfra-v0.6.0) (2026-10-12)

### Features

* **appserve:** structure tab in the inspect panel ([e580f6e](https://github.com/holistics/anfra/commit/e580f6e83ab8912ffd19e0625ff6abc6f0fe799c))
* **sdk:** read a Data App's structure from its markup, show and locate it ([c2fd61c](https://github.com/holistics/anfra/commit/c2fd61cb08e898e42da21a6478f5cda9c1324ca2))

### Bug Fixes

* **appserve:** pick mode, the toolbar and copying a handle ([8745c5f](https://github.com/holistics/anfra/commit/8745c5fd3e25211d885d375ae4896b2835c06a36))
* **sdk,appserve:** plainer words in the structure's messages ([b056afd](https://github.com/holistics/anfra/commit/b056afde258dfe7b4a7058d4db263be249dd6acb))
* **sdk.app:** declaring an entity or a mapping notifies subscribers ([9e8ccd0](https://github.com/holistics/anfra/commit/9e8ccd074d4245ff65c4623c41e63c7fc1eda1b0))
* **sdk.app:** set the overlay label's parts apart ([333dbff](https://github.com/holistics/anfra/commit/333dbffc2247a75a5bb0b9a99ae8d32ed3baae4c))

### Build

* publish @holistics/anfra-sdk ([df8071d](https://github.com/holistics/anfra/commit/df8071de74815aeed41d32b5966fbd2b5b6b5198))
## [0.5.0](https://github.com/holistics/anfra/compare/anfra-v0.4.3...anfra-v0.5.0) (2026-10-08)

### ⚠ BREAKING CHANGES

* env flags only allow 0 or 1 for affordance. add flag for HIDE_ERROR_CAUSES

### Features

* env flags only allow 0 or 1 for affordance. add flag for HIDE_ERROR_CAUSES ([91933d5](https://github.com/holistics/anfra/commit/91933d55f2954ca50f266d6cc5be8fe27dc4ecf2))
## [0.4.3](https://github.com/holistics/anfra/compare/anfra-v0.4.2...anfra-v0.4.3) (2026-10-08)

### Features

* show update notice for serve, hide for errors ([2ef54ea](https://github.com/holistics/anfra/commit/2ef54eadd64b0916456e25561c14fc934c6bc283))
* show update notice in appserve frontend ([9977f58](https://github.com/holistics/anfra/commit/9977f580f348efcd0d68f0c6cbf64a0ed3a12e56))

### Bug Fixes

* **cli:** update skill plugin name ([610c71e](https://github.com/holistics/anfra/commit/610c71e21b775d4a940c39759c731f3a902e040f))

### Build

* pre-create home folder to prevent others claiming the ownership and permission ([8cd7daa](https://github.com/holistics/anfra/commit/8cd7daa6ad73107c790b274a7142ecffcd76250e))
## [0.4.2](https://github.com/holistics/anfra/compare/anfra-v0.4.1...anfra-v0.4.2) (2026-10-08)

### Bug Fixes

* use tini as the init process to work around sidecar watchdog check ([ec7055a](https://github.com/holistics/anfra/commit/ec7055ad169181078a9d464f19e3a742e2bb8a49))
## [0.4.1](https://github.com/holistics/anfra/compare/anfra-v0.4.0...anfra-v0.4.1) (2026-10-08)

### Build

* release Docker image ([23b819e](https://github.com/holistics/anfra/commit/23b819e61ce9fc7c5950a75e627f87b6612f5aa3))
## [0.4.0](https://github.com/holistics/anfra/compare/anfra-v0.3.0...anfra-v0.4.0) (2026-10-07)

### ⚠ BREAKING CHANGES

* move to apikit, remove socket listener in favor of serve.json
* revamp commands for better typing and affordance

### Features

* add the engine package, this module's public API ([ea3de82](https://github.com/holistics/anfra/commit/ea3de82f844017ab0c5eba183e68c6f5d78ba0cb))
* **appserve:** frontend ([1affde6](https://github.com/holistics/anfra/commit/1affde6b2f6906852ed75622ac61b134f7cafec7))
* **cli:** skills install ([82f9656](https://github.com/holistics/anfra/commit/82f965683b969d1cc304b326ae958e4b55250fd6))
* **engine.query:** support query input (transforms) ([051aeb2](https://github.com/holistics/anfra/commit/051aeb24b8cd1652d49a8b29828fe5e4be86cba1))
* **engine:** add show command ([845e401](https://github.com/holistics/anfra/commit/845e4010b268288673c08cf12e73fa7c06dbac7f))
* **engine:** support querying with SQL ([3bb0732](https://github.com/holistics/anfra/commit/3bb073278bb7f3666abe88db469dded59cad33aa))
* ingest and search ([3401694](https://github.com/holistics/anfra/commit/340169481b142bc0236400067fa2a43d09eca9d9))
* mcp and discovery ([a973b4b](https://github.com/holistics/anfra/commit/a973b4bf548ec79a9c82943b2029da4eed54cca3))
* prepare for anfra cloud ([83d8c7f](https://github.com/holistics/anfra/commit/83d8c7fec08db3001ab4942c7ba321bd50cf9301))
* **sdk:** init anfra SDK ([cb96e0f](https://github.com/holistics/anfra/commit/cb96e0fe2951638f1a917f678d328d74877bd609))
* **shared:** add shared package ([20548be](https://github.com/holistics/anfra/commit/20548be698ee23fa7ebbe0d4d60f86b4dadf0e52))
* telemetry ([a832e27](https://github.com/holistics/anfra/commit/a832e275982883149f67f3bf8a43e65a9635960b))

### Performance Improvements

* batch dataset fetch and retain aml cache in memory ([c13a4b8](https://github.com/holistics/anfra/commit/c13a4b8cd94a0a9fcf9c8ab900f4b7bd6cf61d96))

### Miscellaneous Chores

* move to apikit, remove socket listener in favor of serve.json ([07c20b8](https://github.com/holistics/anfra/commit/07c20b86f332eefbf0b2c0da890b6ac30c8be07c))
* revamp commands for better typing and affordance ([00a2722](https://github.com/holistics/anfra/commit/00a272201116006a19f58c764a37485cb15daf85))

### Security

* bump fast-uri to 3.1.8 and js-yaml to 4.3.2 (W39-2026) ([d107880](https://github.com/holistics/anfra/commit/d107880d97581db376553f8211ad6d5336495f6f))
* **deps:** bump js-yaml to 4.3.1 (ENG-3075) ([e1e6687](https://github.com/holistics/anfra/commit/e1e66876a68d4bf4c64e03f33b3e242437973c0d)), closes [#23](https://github.com/holistics/anfra/issues/23)
* **deps:** bump x/crypto, x/sys, Go toolchain, and npm transitives ([06ea3ff](https://github.com/holistics/anfra/commit/06ea3ff3d10a033192e6c9f1b3e580f3f2f75010))
## [0.3.0](https://github.com/holistics/anfra/compare/anfra-v0.2.0...anfra-v0.3.0) (2026-07-23)

### Build

* bump canal-query 2.13.0 ([63048f3](https://github.com/holistics/anfra/commit/63048f35cdf0021c4fc15677108afc12d0992e82))
* compress download ([25cea57](https://github.com/holistics/anfra/commit/25cea57460cc341ffb5bb8c17305a90322c8507c))
## [0.2.0](https://github.com/holistics/anfra/compare/anfra-v0.1.0...anfra-v0.2.0) (2026-07-22)

### Features

* auto update mechanism ([258a94c](https://github.com/holistics/anfra/commit/258a94cf8ef08dccec2cebbae6f699c9224f43ae))

### Build

* install.sh script ([24de3d9](https://github.com/holistics/anfra/commit/24de3d98b96c23b458284c23a71f3af2038a3d0f))

## [0.1.0](https://github.com/holistics/anfra/compare/c567bd9d4eb4601ca1480de59d02c363ee6f19cc...anfra-v0.1.0) (2026-07-21)

### Features

* implement serve ([1e2fb63](https://github.com/holistics/anfra/commit/1e2fb63b1634ae7052f5ef6b6bfbec2a1deae04a))
* patch aql limit with truncate_rows ([2fd2a90](https://github.com/holistics/anfra/commit/2fd2a90a9075b8732903c72c8c80bef3821dafa4))
* query command ([c567bd9](https://github.com/holistics/anfra/commit/c567bd9d4eb4601ca1480de59d02c363ee6f19cc))
* remove validate --aql, add arg shorthands ([70bf1e2](https://github.com/holistics/anfra/commit/70bf1e29f05153b9ffab7933e4040d9807502e4c))
* validate command ([fa0ce05](https://github.com/holistics/anfra/commit/fa0ce05db16a01b8b853e03cbc63bd3b3f7628d2))

### Bug Fixes

* proper validation for AQL ([d633c21](https://github.com/holistics/anfra/commit/d633c21bbe94a274d7e8e66f71f67c32f368a132))

### Build

* bump anfra_node 0.0.3 ([c9ecb63](https://github.com/holistics/anfra/commit/c9ecb63527ec6b78779afb4fc82c6b7b985b338f))

### Performance Improvements

* enable canal query connection pool when doing serve ([a9a8f25](https://github.com/holistics/anfra/commit/a9a8f250e7a73ce6fb06c98175fc2fd3ea10cb78))
