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
