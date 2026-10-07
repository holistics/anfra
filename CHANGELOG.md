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
