# Changelog

## [1.11.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.10.0...v1.11.0) (2026-06-16)


### Features

* **tools:** add get_last_comment tool (AGENT-47) ([#42](https://github.com/ItsJennyFiggy/plane-mcp/issues/42)) ([f2fa8b5](https://github.com/ItsJennyFiggy/plane-mcp/commit/f2fa8b58f822dd73b0b29ca2b71713ce9eff66bc))
* **tools:** add list_comments tool (AGENT-46) ([#41](https://github.com/ItsJennyFiggy/plane-mcp/issues/41)) ([4613a87](https://github.com/ItsJennyFiggy/plane-mcp/commit/4613a87fddb9049fc1d66e71b1bb2b0b4d7db9d9))
* **tools:** add list_states tool (AGENT-39) ([#36](https://github.com/ItsJennyFiggy/plane-mcp/issues/36)) ([be8024c](https://github.com/ItsJennyFiggy/plane-mcp/commit/be8024cc49aa26bd390b0d300bf88a1253f10a12))
* **tools:** add list_work_items tool (AGENT-49) ([#38](https://github.com/ItsJennyFiggy/plane-mcp/issues/38)) ([475c6b6](https://github.com/ItsJennyFiggy/plane-mcp/commit/475c6b65a1c6c46a17853958f31745f9e0682a51))
* **tools:** add search_work_items tool (AGENT-42) ([#40](https://github.com/ItsJennyFiggy/plane-mcp/issues/40)) ([f86dbcc](https://github.com/ItsJennyFiggy/plane-mcp/commit/f86dbcc138515cc91f71b9a7a0e9b09ffc0ce4ea))
* **tools:** annotate plane-mcp tools with ToolAnnotations (AGENT-43) ([#44](https://github.com/ItsJennyFiggy/plane-mcp/issues/44)) ([15c85f0](https://github.com/ItsJennyFiggy/plane-mcp/commit/15c85f0e3e79380359ec0f78a7f9d3a5e5b45162))


### Bug Fixes

* **tools:** convert HTML descriptions in get_work_item (AGENT-50) ([#43](https://github.com/ItsJennyFiggy/plane-mcp/issues/43)) ([92767b7](https://github.com/ItsJennyFiggy/plane-mcp/commit/92767b7ad7d37418a645ae8ffb16100476502019))

## [1.10.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.9.0...v1.10.0) (2026-06-15)


### Features

* **tools:** add assign_work_item tool for assignee management ([#34](https://github.com/ItsJennyFiggy/plane-mcp/issues/34)) ([3cb68bc](https://github.com/ItsJennyFiggy/plane-mcp/commit/3cb68bc881a37f961c5bddae0afa177e6c88d609))

## [1.9.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.8.1...v1.9.0) (2026-06-15)


### Features

* add list_projects tool ([#32](https://github.com/ItsJennyFiggy/plane-mcp/issues/32)) ([47c1128](https://github.com/ItsJennyFiggy/plane-mcp/commit/47c11287acf663c576650e41b90153dbfbd090f0))
* **create_task:** add optional parent parameter for sub-issue creation ([#31](https://github.com/ItsJennyFiggy/plane-mcp/issues/31)) ([718fe75](https://github.com/ItsJennyFiggy/plane-mcp/commit/718fe75532bdf6a39263642d3f4d3a774c53e9b0))


### Bug Fixes

* **find_my_work:** make project and state_group optional ([#30](https://github.com/ItsJennyFiggy/plane-mcp/issues/30)) ([1aff8a3](https://github.com/ItsJennyFiggy/plane-mcp/commit/1aff8a371a9a041858579c045cbc644b3f448f59))

## [1.8.1](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.8.0...v1.8.1) (2026-06-15)


### Performance Improvements

* **docker:** speed up GHCR image build with .dockerignore and layer caching ([#27](https://github.com/ItsJennyFiggy/plane-mcp/issues/27)) ([9446ade](https://github.com/ItsJennyFiggy/plane-mcp/commit/9446adea780cdc308fd1b3918b49e96ded77929a))

## [1.8.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.7.0...v1.8.0) (2026-06-15)


### Features

* **tools:** add add_label and remove_label MCP tools ([#25](https://github.com/ItsJennyFiggy/plane-mcp/issues/25)) ([2df1574](https://github.com/ItsJennyFiggy/plane-mcp/commit/2df1574458045d4bbbe4cf85590b786e606ee13e))

## [1.7.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.6.2...v1.7.0) (2026-06-14)


### Features

* **tools:** add list_labels tool for label discovery ([#23](https://github.com/ItsJennyFiggy/plane-mcp/issues/23)) ([0639386](https://github.com/ItsJennyFiggy/plane-mcp/commit/06393865b3cd1c01149eb3717b1e1dc7434c3f3d))

## [1.6.2](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.6.1...v1.6.2) (2026-06-14)


### Bug Fixes

* **create_task:** use 'labels' field name matching Plane API serializer ([#21](https://github.com/ItsJennyFiggy/plane-mcp/issues/21)) ([925cd08](https://github.com/ItsJennyFiggy/plane-mcp/commit/925cd08a0423f0652093d86770d96dd567a7ed19))

## [1.6.1](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.6.0...v1.6.1) (2026-06-14)


### Bug Fixes

* **create_task:** make assignees/labels optional and accept stringified arrays ([#19](https://github.com/ItsJennyFiggy/plane-mcp/issues/19)) ([fdffd36](https://github.com/ItsJennyFiggy/plane-mcp/commit/fdffd368e245dee885a921265bdfb499e6ad1983))

## [1.6.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.5.0...v1.6.0) (2026-06-14)


### Features

* **create_task:** module assignment + Markdown descriptions (AGENT-18, AGENT-19) ([#17](https://github.com/ItsJennyFiggy/plane-mcp/issues/17)) ([3301853](https://github.com/ItsJennyFiggy/plane-mcp/commit/3301853504e402e19a52a83f4124d0b073a55fdd))

## [1.5.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.4.1...v1.5.0) (2026-06-13)


### Features

* **formatter:** preserve table, strikethrough, and tasklist in work-item descriptions ([#14](https://github.com/ItsJennyFiggy/plane-mcp/issues/14)) ([0d2fe7b](https://github.com/ItsJennyFiggy/plane-mcp/commit/0d2fe7b3b08dd4d1a2bd83c4d3e055501b7c0ee8))

## [1.4.1](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.4.0...v1.4.1) (2026-06-13)


### Bug Fixes

* **plane:** use /api/v1/users/me/ instead of workspace-scoped /me/ endpoint ([#12](https://github.com/ItsJennyFiggy/plane-mcp/issues/12)) ([d3148aa](https://github.com/ItsJennyFiggy/plane-mcp/commit/d3148aac62dc03f2cec7fa14ec4176f48c7f8325))

## [1.4.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.3.0...v1.4.0) (2026-06-13)


### Features

* **tools:** 5 semantic MCP tools + profile gate (AGENT-10 + AGENT-11) ([#10](https://github.com/ItsJennyFiggy/plane-mcp/issues/10)) ([c651fe7](https://github.com/ItsJennyFiggy/plane-mcp/commit/c651fe7b721d290e9d260f6671abd7ae15623b10))

## [1.3.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.2.0...v1.3.0) (2026-06-13)


### Features

* **plane:** add 6 new client methods and GetCallerID resolver (AGENT-10) ([#8](https://github.com/ItsJennyFiggy/plane-mcp/issues/8)) ([0fd16f9](https://github.com/ItsJennyFiggy/plane-mcp/commit/0fd16f9832407930d6428fedd876e998d3d73d9f))

## [1.2.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.1.0...v1.2.0) (2026-06-13)


### Features

* implement token-efficient output contract (AGENT-9) ([#6](https://github.com/ItsJennyFiggy/plane-mcp/issues/6)) ([4571872](https://github.com/ItsJennyFiggy/plane-mcp/commit/45718728ddc04f9b41cbd7ea8b7c387a80e1293b))

## [1.1.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.0.0...v1.1.0) (2026-06-13)


### Features

* **plane:** implement REST client, auth, and Name-UUID resolver ([#4](https://github.com/ItsJennyFiggy/plane-mcp/issues/4)) ([0b6d8b2](https://github.com/ItsJennyFiggy/plane-mcp/commit/0b6d8b23633aa9fdce8fe80e33d629056acd74b9))

## 1.0.0 (2026-06-13)


### Features

* **server:** implement Go MCP server skeleton with stdio transport and config parsing ([#2](https://github.com/ItsJennyFiggy/plane-mcp/issues/2)) ([5c1f2c8](https://github.com/ItsJennyFiggy/plane-mcp/commit/5c1f2c80ed0a6df88ced45b24b6db3206503b2e0))

## [1.0.1](https://github.com/ItsJennyFiggy/template-go/compare/v1.0.0...v1.0.1) (2026-06-11)


### Bug Fixes

* **ghcr:** lowercase repository name in docker tags ([#3](https://github.com/ItsJennyFiggy/template-go/issues/3)) ([ccfca23](https://github.com/ItsJennyFiggy/template-go/commit/ccfca23f9c59aee11941b519d39e4d02f7df92e1))

## 1.0.0 (2026-06-11)


### Features

* **scaffold:** bootstrap Go stdlib template and workflows ([#1](https://github.com/ItsJennyFiggy/template-go/issues/1)) ([26a300e](https://github.com/ItsJennyFiggy/template-go/commit/26a300e8d6d8596c466242dd079ff34f714bcade))
