## [0.20.0](https://github.com/holistics/anfra/compare/v0.19.0...v0.20.0) (2026-11-11)

### Features

* docs and skills for new publish flows ([08f4c5b](https://github.com/holistics/anfra/commit/08f4c5b5b8645de01877749258bd04cfd5811042))
* **cli:** offer an e-commerce sample project in anfra init ([59b7a61](https://github.com/holistics/anfra/commit/59b7a61e5a1d9b3e08c8b61e9cffa5361d742b80))
* **cli:** tolerate sub pages for org url ([9ebd872](https://github.com/holistics/anfra/commit/9ebd872d9dd7e9f3b760e8a3136f42a19cb76b23))
* **cli:** warnings for when anfra publish might not be in step with git ([761c676](https://github.com/holistics/anfra/commit/761c67633a44607abf9fbfc39f0ac25f99882f79))
## [0.19.0](https://github.com/holistics/anfra/compare/v0.18.0...v0.19.0) (2026-11-09)

### Features

* **cli:** check for updates hourly, not twice a day ([b376f22](https://github.com/holistics/anfra/commit/b376f22fc99bcdd3c81c46aa7045bd8d2ca25185))
* **skills:** handle a Data App's errors by scope, and read them in anfra.log ([58c2c0d](https://github.com/holistics/anfra/commit/58c2c0d2331bef64ff52cf15f700c77c709b4e47))
* **skills:** add skill and links to docs ([d1ab32a](https://github.com/holistics/anfra/commit/d1ab32a326229a62ce379443c6c8a8dfd07594d7))
* **skills:** update new AML features - new relationship types, ai_context prop ([451fe69](https://github.com/holistics/anfra/commit/451fe69460b2093d77ff2d7a2e58ad74c29ac401))

### Bug Fixes

* **skills:** read a Data App's log from its last mount, and its stack lines ([d70e50a](https://github.com/holistics/anfra/commit/d70e50ae87c6517275ea543d94f829edf0b5a7f8))
* **skills:** name the SDK's stack frames anfra-sdk.js ([9204594](https://github.com/holistics/anfra/commit/9204594e2865b2e37ea5d8814d3e2ac980fbcb35))
* **skills:** read a Data App's log lines by event ([3a538d8](https://github.com/holistics/anfra/commit/3a538d8d07925396bb060739e88105a23910bf0b))
* **skills:** read a Data App's errors by their new kinds ([24ede33](https://github.com/holistics/anfra/commit/24ede3302a6f2d474dc662278a670d3441568e3c))
* **skills:** read request_id, and an empty log, in a Data App's log ([66f98c1](https://github.com/holistics/anfra/commit/66f98c18e0e588e7851dfd144b2b895a4f5e2482))
* **skills:** read a Data App's log first when the user reports a problem ([e802f3c](https://github.com/holistics/anfra/commit/e802f3c664a546b5ca0a6293b674d11856a2b1ec))
* **skills:** name validation_failed for what a control or cross-filter adds wrong ([4e9d01c](https://github.com/holistics/anfra/commit/4e9d01c86b199f938ed5622a5b4f3e8b1b224299))
## [0.17.0](https://github.com/holistics/anfra/compare/v0.16.0...v0.17.0) (2026-11-04)

### Features

* **skills:** minor updates ([2b096ca](https://github.com/holistics/anfra/commit/2b096cab76c952b8a778ffbae9c3d88d17ac0173))
* **skills:** name a relationship in with_relationships ([64c884f](https://github.com/holistics/anfra/commit/64c884f570dda064ff927b4270a7378795683505))
* **skills:** name the point's measure in query.menu ([4100913](https://github.com/holistics/anfra/commit/41009133a928917fd129f19c7c52e8eee2058e16))
* **skills:** read a filter's value, guard late libraries, find server errors ([0cc1220](https://github.com/holistics/anfra/commit/0cc1220fb1b95b6462a51ec99f3f434c0627bc18))
* **skills:** say what requestId and a throwing subscribe() listener do ([03af9b2](https://github.com/holistics/anfra/commit/03af9b2322d5dc2480b4dfa9e32988408554c3c0))
* **skills:** teach a Data App's errors by code and scope ([92a98b9](https://github.com/holistics/anfra/commit/92a98b99268d8d31f54d486e3b71a56fdb9f5f9c))

### Bug Fixes

* **skills:** clarify wording in the feedback corrections ([9caa1c8](https://github.com/holistics/anfra/commit/9caa1c87c3d0cb64555e4f6d19efc737a09ff58e))
* **skills:** find anfra serve's log with anfra status ([6f5b4ca](https://github.com/holistics/anfra/commit/6f5b4caa548c8ba622853bcd398d69a139cdfb02))
* **skills:** fix knowledge and examples for filters on aliases ([c34cfcf](https://github.com/holistics/anfra/commit/c34cfcfa4edb19344328ad00576b3529bb074783))
* **skills:** say an inactive relationship is off, not optional ([ef4e55f](https://github.com/holistics/anfra/commit/ef4e55f5edb0e2c0848382d887eef50de8b847dd))
## [0.15.0](https://github.com/holistics/anfra/compare/v0.14.2...v0.15.0) (2026-10-30)

### ⚠ BREAKING CHANGES

* **skills:** prefix skill names with anfra- to prevent collision with global agent skills

### Features

* **skills:** prefer writing AQL by self ([9334ad8](https://github.com/holistics/anfra/commit/9334ad8a05e7176953eff1a3a9ffade03a881dd2))
* **skills:** prefix skill names with anfra- to prevent collision with global agent skills ([bc70675](https://github.com/holistics/anfra/commit/bc706756ea13952e5513713792efe8eace5b5c81))
* **skills:** stamp versions in skills ([c489a9b](https://github.com/holistics/anfra/commit/c489a9bfd3866f785f47d58f8c009be871290828))
* **skills:** update installation README ([bd5a582](https://github.com/holistics/anfra/commit/bd5a58298bfe0d68e664d38a6acb245f931bf45e))
* **skills:** use new agent check commands ([53f07d2](https://github.com/holistics/anfra/commit/53f07d264f6186246b4161109d3ab39502dd0adb))

### Build

* move skills build to anfra repo ([225363f](https://github.com/holistics/anfra/commit/225363f6af7f3e49ce6f8fff5c497bb9d5ab1cef))
## [0.4.5](https://github.com/holistics/anfra-skills/compare/anfra-v0.4.4...anfra-v0.4.5) (2026-10-28)

### Features

* **plugins.anfra:** follow the host's light or dark mode with Anfra.theme ([e428bc3](https://github.com/holistics/anfra-skills/commit/e428bc34a43cf67ae8e5c09c0dcfbb2db0a2a431))
* **plugins.anfra:** open the host's menu on a chart's point with query.menu ([60fc394](https://github.com/holistics/anfra-skills/commit/60fc394fb058b1acfa2f8a96630959946bade2f2))

### Bug Fixes

* **plugins.anfra:** drop the Inspect panel's tabs from build-data-app ([b07d41f](https://github.com/holistics/anfra-skills/commit/b07d41f6cc085d7d86e24b1c17631d006ad756f4))

## [0.4.4](https://github.com/holistics/anfra-skills/compare/anfra-v0.4.3...anfra-v0.4.4) (2026-10-28)

### Bug Fixes

* **plugins.anfra:** say anfra query answers one 10,000-row page ([8b6a806](https://github.com/holistics/anfra-skills/commit/8b6a8064f416c21c79a95bef9a9430e15936aff4))
* **plugins.anfra:** say Data App queries are uncapped and what a paged result holds ([05f4c72](https://github.com/holistics/anfra-skills/commit/05f4c72d66089dcb3a5430afd1adad86947e62b2))

## [0.4.3](https://github.com/holistics/anfra-skills/compare/anfra-v0.4.2...anfra-v0.4.3) (2026-10-21)

### Features

* **plugins.anfra:** load Data App libraries from jsDelivr and cdnjs only, ES modules by +esm ([45c9ac3](https://github.com/holistics/anfra-skills/commit/45c9ac3156e3639b2f7998851cea11764b27b914))
* **plugins.anfra:** teach Data Apps' content policy and SRI hashes ([061aaf0](https://github.com/holistics/anfra-skills/commit/061aaf0126d05f418655e47db07b61e2edc6132e))

## [0.4.2](https://github.com/holistics/anfra-skills/compare/anfra-v0.4.1...anfra-v0.4.2) (2026-10-21)

### Bug Fixes

* **plugins.anfra:** require Data Apps to be one self-contained HTML file ([133e496](https://github.com/holistics/anfra-skills/commit/133e49667ce435092d6370efe25f671da80229e3))

## [0.4.0](https://github.com/holistics/anfra-skills/compare/anfra-v0.3.2...anfra-v0.4.0) (2026-10-19)

### Features

* describe the plugin system and give each built-in plugin its own reference ([f1c8cc8](https://github.com/holistics/anfra-skills/commit/f1c8cc8b0537704be523a9aa24df56016ade98a7))
* teach Anfra.plugins in place of Anfra.features ([20aa602](https://github.com/holistics/anfra-skills/commit/20aa602cbef12bd6a5ecb44b9df7d399945e0f79))

## [0.3.1](https://github.com/holistics/anfra-skills/compare/anfra-v0.3.0...anfra-v0.3.1) (2026-10-13)

### Features

* **plugins.anfra:** export a query's result in build-data-app ([891050d](https://github.com/holistics/anfra-skills/commit/891050d3b7201a67ccd25103c0832db956a2793b))

### Bug Fixes

* **plugins.anfra:** add export buttons only when the design agrees ([4388238](https://github.com/holistics/anfra-skills/commit/43882387c9ada4cb6d0e41704b2ab654f8af8609))
* **plugins.anfra:** keep export feedback out of the layout ([bb4db43](https://github.com/holistics/anfra-skills/commit/bb4db434c3c8bec49591ae35fd85966b5680c436))

## [0.3.0](https://github.com/holistics/anfra-skills/compare/anfra-v0.2.1...anfra-v0.3.0) (2026-10-12)

### ⚠ BREAKING CHANGES

* **plugins.anfra:** new semantic types and data handling

### Features

* **plugins.anfra:** new semantic types and data handling ([32c1d03](https://github.com/holistics/anfra-skills/commit/32c1d03f560129bc5f3274d609a432ed36af7103))

## [0.2.1](https://github.com/holistics/anfra-skills/compare/anfra-v0.2.0...anfra-v0.2.1) (2026-10-12)

## 0.2.0 (2026-10-08)

### ⚠ BREAKING CHANGES

* rename plugin to anfra

### Miscellaneous Chores

* rename plugin to anfra ([187ed7e](https://github.com/holistics/anfra-skills/commit/187ed7e041d84d720bd52fc5e9091c6f31c536ce))

## [0.1.1](https://github.com/holistics/anfra-skills/compare/anfra-development-v0.1.0...anfra-development-v0.1.1) (2026-10-07)

### Features

* **plugins.anfra-development:** adapt to new cli serve args ([8fe1cad](https://github.com/holistics/anfra-skills/commit/8fe1cad6a202ea680641bb18b6007c54eea565ed))

## [0.1.0](https://github.com/holistics/anfra-skills/compare/anfra-development-v0.0.3...anfra-development-v0.1.0) (2026-10-07)

### Features

* **plugins.anfra-development:** codex plugin format ([2c75e18](https://github.com/holistics/anfra-skills/commit/2c75e18485bff3547c6e53527813df0a16430af9))
* **plugins.anfra-development:** setup-repo and build-data-app skills ([610b248](https://github.com/holistics/anfra-skills/commit/610b24893c6725013615fdd47eab2289a327d26e))
* **plugins.anfra-development:** sql queries for studying data sources in aml ([6677a50](https://github.com/holistics/anfra-skills/commit/6677a506fccd6f1357209f3d4b73f639321cb243))
* **plugins.anfra-development:** update new CLI commands ([45caba2](https://github.com/holistics/anfra-skills/commit/45caba2ab64f6fd38cc4a3f1518b48daa3ed8c3b))

## 0.0.3 (2026-07-06)

### Features

* **plugins.anfra-development:** knowledge for aql having ([c615d13](https://github.com/holistics/anfra-skills/commit/c615d132f12edb573515f71a9b5ec8754d288e7c))

## 0.0.2 (2026-07-06)

### Features

* **plugins.anfra-development:** add aml validation hook ([0926734](https://github.com/holistics/anfra-skills/commit/09267347b6b563f2e27cfce586bcfca70f749b43))
* **plugins.anfra-development:** aql-writer agent ([43437c5](https://github.com/holistics/anfra-skills/commit/43437c5bd4dee2245d736caf1ba86e458d3bdb21))
* **plugins.anfra-development:** inject aqlearn and example index into aql-writer ([7abdb2b](https://github.com/holistics/anfra-skills/commit/7abdb2b78a163d2cf7a045b5045a7363ac1679fd))

### Bug Fixes

* **plugins.anfra-development:** prevent AI failing many times before reading references ([82d1396](https://github.com/holistics/anfra-skills/commit/82d139697dac0b61c0b7b2a372a6ab73e689ef17))

## 0.0.1 (2026-07-03)

### Features

* **plugins.anfra-development:** init with aml and aql skills ([434e7d8](https://github.com/holistics/anfra-skills/commit/434e7d88a2e4ef2cc7d33eaf995bfca4e4c312a9))
