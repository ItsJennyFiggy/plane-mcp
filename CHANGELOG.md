# Changelog

## [1.16.0] — Unreleased

### Features

* **tools:** add `create_project_label`, `update_project_label`, `delete_project_label`, `provision_standard_labels` tools (planner+full only) (AGENT-176)
* **plane:** add `CreateLabel`, `UpdateLabel`, `DeleteLabel` client methods (AGENT-176)
* **tools:** add `mcp.ToolAnnotations` (readOnlyHint/destructiveHint/idempotentHint/openWorldHint) to all label-related tools (AGENT-176)

### Breaking changes

* **tools:** rename label tools for disambiguation — `add_label` → `add_label_to_work_item`, `remove_label` → `remove_label_from_work_item`, `list_labels` → `list_project_labels` (AGENT-176)

## [1.15.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.14.0...v1.15.0) (2026-06-29)


### Features

* **transport:** add Streamable HTTP transport to plane-mcp (AGENT-130) ([#76](https://github.com/ItsJennyFiggy/plane-mcp/issues/76)) ([fe33aa9](https://github.com/ItsJennyFiggy/plane-mcp/commit/fe33aa9b7ef3bc49fb089f02f1588808e53dec60))

## [1.14.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.13.0...v1.14.0) (2026-06-24)


### Features

* **tools:** add list_modules + set_module tools (AGENT-125) ([#75](https://github.com/ItsJennyFiggy/plane-mcp/issues/75)) ([df6ded7](https://github.com/ItsJennyFiggy/plane-mcp/commit/df6ded787978121ec72290c5ce28f2de6483680c))
* **tools:** remove add_label/remove_label from the worker profile ([#72](https://github.com/ItsJennyFiggy/plane-mcp/issues/72)) ([2ac1c44](https://github.com/ItsJennyFiggy/plane-mcp/commit/2ac1c44c9f724c558c9396c2bb6a9399f079078e))

## [1.14.0] — Unreleased

### Features

* **tools:** add list_modules + set_module tools (module read/assign surface) (AGENT-125)
* **tools:** add module parameter to update_work_item (AGENT-125)

## [1.13.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.12.2...v1.13.0) (2026-06-18)


### Features

* **tools:** add reviewer tool-scoping profile (read + comment-back only) ([#68](https://github.com/ItsJennyFiggy/plane-mcp/issues/68)) ([6e9f2da](https://github.com/ItsJennyFiggy/plane-mcp/commit/6e9f2da436389baaf0d0ed25f7ed0b481f75c817))

## [1.12.2](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.12.1...v1.12.2) (2026-06-18)


### Bug Fixes

* **server:** add graceful shutdown via signal-aware context ([#66](https://github.com/ItsJennyFiggy/plane-mcp/issues/66)) ([c2abd73](https://github.com/ItsJennyFiggy/plane-mcp/commit/c2abd739add124e27f8ced942c7da5906b23d94e))

## [1.12.1](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.12.0...v1.12.1) (2026-06-17)


### Bug Fixes

* **ci:** remove non-existent ko-build/ko-action step ([#64](https://github.com/ItsJennyFiggy/plane-mcp/issues/64)) ([9d33305](https://github.com/ItsJennyFiggy/plane-mcp/commit/9d33305a721095de6ba01b721ce4ec6b596b7c73))

## [1.12.0](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.11.1...v1.12.0) (2026-06-17)


### Features

* add add_comment tool and make report_progress state optional (AGENT-52) ([#52](https://github.com/ItsJennyFiggy/plane-mcp/issues/52)) ([534135b](https://github.com/ItsJennyFiggy/plane-mcp/commit/534135bed2ab1013da9f10ca93c0b24f7a3654d7))
* add update_work_item tool (AGENT-51) ([#47](https://github.com/ItsJennyFiggy/plane-mcp/issues/47)) ([3bf67bb](https://github.com/ItsJennyFiggy/plane-mcp/commit/3bf67bb1f2a45e125186740f1a42454955e59bb3))
* **agent-41:** add relations management tools (set_relation, remove_relation, list_relations) ([#49](https://github.com/ItsJennyFiggy/plane-mcp/issues/49)) ([f67387a](https://github.com/ItsJennyFiggy/plane-mcp/commit/f67387a51e56753562cfa42736689fff8a848e73))
* **agent-68:** add parent/sub-issue management tools (set_parent, clear_parent, list_children) ([#50](https://github.com/ItsJennyFiggy/plane-mcp/issues/50)) ([706f58e](https://github.com/ItsJennyFiggy/plane-mcp/commit/706f58e2f657ca3f578df2fd4ff0024b2d3abb10))
* **ci:** modernize release workflows with goreleaser, ko, and golangci-lint ([#62](https://github.com/ItsJennyFiggy/plane-mcp/issues/62)) ([8a82d19](https://github.com/ItsJennyFiggy/plane-mcp/commit/8a82d19e380e8bb5b5ec462296c8517b8f6fa2a4))
* implement move_work_item tool (AGENT-48) ([#51](https://github.com/ItsJennyFiggy/plane-mcp/issues/51)) ([553ad17](https://github.com/ItsJennyFiggy/plane-mcp/commit/553ad173e1413c9561a0f54788bd8487cbbfcd9f))
* **tools:** add FlexibleDetail type and schema for get_work_item detail parameter ([#60](https://github.com/ItsJennyFiggy/plane-mcp/issues/60)) ([1ae0559](https://github.com/ItsJennyFiggy/plane-mcp/commit/1ae05598ee8224d9cf1f11ebf3a28134ba73c2ae))


### Bug Fixes

* apply state_group filter client-side in list_work_items ([#53](https://github.com/ItsJennyFiggy/plane-mcp/issues/53)) ([76b5b7b](https://github.com/ItsJennyFiggy/plane-mcp/commit/76b5b7b7460decff7a81ab5d5cb37f0aa7841fc0))
* **plane-mcp:** AGENT-78/79/80/81 — v1.12 pre-release bug fixes ([#63](https://github.com/ItsJennyFiggy/plane-mcp/issues/63)) ([63d0cf2](https://github.com/ItsJennyFiggy/plane-mcp/commit/63d0cf2aba8491c592a4f900aed32a6f94554c11))
* post-merge cleanup and robustness fixes (Group 2) ([#57](https://github.com/ItsJennyFiggy/plane-mcp/issues/57)) ([110f8a2](https://github.com/ItsJennyFiggy/plane-mcp/commit/110f8a2380796467ddbc7e180fdadd687133c078))
* post-merge code review fixes (Batch C) ([#54](https://github.com/ItsJennyFiggy/plane-mcp/issues/54)) ([def0f8e](https://github.com/ItsJennyFiggy/plane-mcp/commit/def0f8efa47f98ef1c667095b5618ebebbf18ca7))
* post-merge safety and stability fixes (Group 1) ([#56](https://github.com/ItsJennyFiggy/plane-mcp/issues/56)) ([fa67973](https://github.com/ItsJennyFiggy/plane-mcp/commit/fa67973d43ccaf9b944e16399c749f0da41a0495))

## [1.11.1](https://github.com/ItsJennyFiggy/plane-mcp/compare/v1.11.0...v1.11.1) (2026-06-16)


### Bug Fixes

* **tools:** fix search_work_items sequence_id type mismatch (AGENT-61) ([#45](https://github.com/ItsJennyFiggy/plane-mcp/issues/45)) ([d5fcb6e](https://github.com/ItsJennyFiggy/plane-mcp/commit/d5fcb6eec189a3f2e8fcd486f84b36890b9bad74))

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
