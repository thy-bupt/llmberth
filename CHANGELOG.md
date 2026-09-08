# Changelog

## 1.0.0 (2026-09-08)


### Features

* llmberth v0.1 core — CLI scaffold, generated app template, golden test ([467cd91](https://github.com/thy-bupt/llmberth/commit/467cd91fbdfa2ce8454ffb593efa7e20e1065ea9))
* M1 close-out — generated-app CI, e2e script, README v1, naming report ([119cd89](https://github.com/thy-bupt/llmberth/commit/119cd89e15c90f3f51254fcdc4328d26b215fc7f))
* M2 v0.2 — lifecycle commands, keys workflow, doctor ([d56bc38](https://github.com/thy-bupt/llmberth/commit/d56bc3857dbd96b95b7fd68218927f81beee8535))
* M3 cp1-4 — usage CLI, budget alerts, keychain upstream keys ([1846a20](https://github.com/thy-bupt/llmberth/commit/1846a202d709f79564e1a4a1d441e9100167404b))
* M3 cp5-8 — TUI operator console (dashboard/keys/logs) ([c60937f](https://github.com/thy-bupt/llmberth/commit/c60937f208eecd46a505caf506466428d47344b1))
* M4 cp1 — prod profile with Caddy TLS (automatic HTTPS) ([9ccce84](https://github.com/thy-bupt/llmberth/commit/9ccce840827d0a137c3b3ce91fa4b697ec52be8a))
* M4 cp2 — doctor --security, twelve-item audit (plan §5.7) ([461b39e](https://github.com/thy-bupt/llmberth/commit/461b39e79156c57575d9662a7d64cd7043c8fc34))
* M4 cp3-cp6 — security policy, threat model, trivy, injection hook ([305cf8d](https://github.com/thy-bupt/llmberth/commit/305cf8d3eba6680cf999c1e21fbf8310e8888a2f))


### Bug Fixes

* CI first-run — golden walk race, gitleaks full-history scan ([0e3e6a2](https://github.com/thy-bupt/llmberth/commit/0e3e6a2a251228f7d500dce8a4ef915458dd0f4f))
* CI second-run — jose2go DoS vulnerabilities, gitleaks placeholder rule ([18dc7ba](https://github.com/thy-bupt/llmberth/commit/18dc7baa1b3b8a1a47781225f9d3cbb3fb9d5f62))
* e2e reliability — stale-container cleanup, loud down failures ([97e0616](https://github.com/thy-bupt/llmberth/commit/97e0616194ef764b86e236f3067c90e83839a308))
* first-run build path (fresh project, no go.sum) — Dockerfile/dev/Makefile ([26c32e8](https://github.com/thy-bupt/llmberth/commit/26c32e87c208562f4469f9870d6ff1b05e8a5569))
* gitleaks config — allowlist map form + demo-file path exclusions ([9de3a57](https://github.com/thy-bupt/llmberth/commit/9de3a57cf5a3c5fba8bb36e6eeccd9b4a6f33cbd))
* pin toolchain go1.25.13 — stdlib fixes for GO-2026-6218/GO-2026-6090 ([71e5bac](https://github.com/thy-bupt/llmberth/commit/71e5bac197032343f62286657d4c2da90e6e461f))
* pin trivy-action to v0.36.0 (0.28.0 tag does not exist) ([8b14382](https://github.com/thy-bupt/llmberth/commit/8b1438258016c70fddfb761ed210eedd0199140f))
* propagate toolchain pin to generated projects (template go.mod) ([444a4ac](https://github.com/thy-bupt/llmberth/commit/444a4ac896e41fc63d93d569fb13c5168a6d23df))
* release-please v4 config — drop removed package-name input ([1537878](https://github.com/thy-bupt/llmberth/commit/1537878a67039756a935e46566fab00ad642a3fe))
* track env templates dropped by gitignore; goldens detect missing files ([91389a5](https://github.com/thy-bupt/llmberth/commit/91389a5f9dd188ae1622b3148e17c4c13df71a1e))
* trivy CI gate — upgrade x/text (CVE-2026-56852), pin trivy-action v0.36.0 ([8767e4e](https://github.com/thy-bupt/llmberth/commit/8767e4ecddf521a270378d554ef5a6082c42f646))
