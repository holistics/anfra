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
