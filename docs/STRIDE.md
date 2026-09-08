# STRIDE Threat Model — llmberth

Scope: the `llmberth` CLI, the application it generates (`template/app/`),
and the runtime stack they assemble (app + Postgres + Caddy + containers).
Trust boundaries: operator laptop ↔ VPS, public internet ↔ Caddy, Caddy ↔
app, app ↔ Postgres, CLI ↔ loopback admin API, operator ↔ git remote.

Legend: ✅ mitigated by default · ⚠️ partially mitigated (documented
residual risk) · ❌ accepted / out of scope (with rationale).

## 1. Generated app — public API (`/v1/*`, via Caddy in prod)

| Threat | Vector | Status | Mitigation / residual |
|---|---|---|---|
| S — Spoofing | Forged `lbt_live_` keys | ✅ | Keys are 256-bit random; only SHA-256 hash + 8-char prefix stored; constant-time compare; unknown/revoked keys rejected (cache invalidation makes revocation instant). |
| S | Replayed stolen keys | ⚠️ | TLS protects transit; per-key rate limits and budgets cap blast radius. No mTLS/key rotation yet (backlog). |
| T — Tampering | Request/response mutation in transit | ✅ | TLS 1.2+ via Caddy (prod, automatic Let's Encrypt); dev binds loopback only. |
| T | Ledger falsification from the app process | ⚠️ | Ledger rows are append-only at the app layer; prod DB role hardening (no UPDATE/DELETE grants) is on the roadmap. Single-tenant Postgres, so app-role compromise is game over regardless. |
| R — Repudiation | "I never sent that request" | ✅ | `usage_ledger` records key, model, tokens, cost, timestamp per request; `audit_log` records every admin action (create/revoke). |
| I — Info disclosure | Message bodies leaked via logs | ✅ | Bodies are never logged; DB persistence is opt-in (`LOG_MESSAGES=1`, off by default) — `doctor --security` item 9 fails the audit when it is on. |
| I | Prompt/response content read by the operator | ❌ | Accepted: a self-hosted tool is operated by the data owner; the alternative (E2E crypto to the model) is impossible — the model must read plaintext. |
| D — DoS | Token/flood exhaustion | ✅ | Per-key rate limit (429 `rate_limited`), per-key + global monthly budgets (429 `budget_exhausted`), input size caps, upstream timeout. |
| E — Elevation | Public API → admin API | ✅ | Separate listener; admin requires `ADMIN_TOKEN` bearer (constant-time); loopback-only binding in dev; **no host mapping at all** in prod (loopback by construction). Prefixes differ (`lbt_live_` vs admin token), so one cannot be confused for the other. |

## 2. Admin API (loopback)

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| S | Forged admin token | ✅ | 256-bit hex token, constant-time compare, fail closed when unset. |
| I | Token exfiltration | ⚠️ | Stored 0600 in `.llmberth-admin-token` (gitignored; `doctor --security` verifies .env/token tracking). Residual: any process running as the same user can read it — same trust level as the operator. |
| E | Container-network peer reaches admin | ✅ | Listener binds all interfaces only inside the container; the **host** publishes admin to `127.0.0.1` in dev and **not at all** in prod. Compose network contains only app+postgres (+Caddy in prod, which has no admin route). |
| T | Revocation bypass via cache | ✅ | Revoke writes `revoked_at`, clears the in-memory cache entry, and appends to `audit_log`; the very next request is rejected (acceptance-tested). |

## 3. Upstream provider calls (SSRF)

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| I/E | SSRF via attacker-influenced `LLM_BASE_URL` | ✅ | Allowlist, fail closed: `LLM_ALLOWED_HOSTS` seeded from the provider preset + loopback; requests to non-allowlisted hosts are refused at startup and per configuration change. |
| I | Key leakage to wrong host | ✅ | The API key is only sent to the allowlisted base_url; provider keys live in the OS keychain / encrypted file (§5.1), overlaid at runtime, never persisted in `.env` by the CLI. |

## 4. Supply chain

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| T | Compromised base image | ✅ | All base images pinned by digest (`doctor --security` item 5); CI runs govulncheck + gitleaks (+ trivy) on the CLI **and** generated projects. |
| T | Malicious dependency | ✅ | Minimal dependency set; govulncheck gates callable vulns (jose2go JWE DoS caught and fixed in-repo). |
| T | Leaked credentials in git history | ✅ | gitleaks full-history scan in CI; §5.8 forbids credential literals (doctor item 2 re-checks generated projects). |
| E | Compromised CI | ⚠️ | Least-privilege workflow permissions; no secrets in CI beyond the stock `GITHUB_TOKEN`. Residual: GH Actions trust. |

## 5. Container & runtime

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| E | Container escape / root in container | ✅ | distroless + `USER nonroot`, `read_only: true` rootfs + tmpfs scratch, digest-pinned images. |
| D | Resource exhaustion on host | ⚠️ | Budgets/rate limits at the app layer; no cgroup caps in compose yet (backlog). |
| I | Postgres exposure | ✅ | Dev: `127.0.0.1` only; prod: no host mapping. Password via env, never committed. |

## 6. CLI (operator side)

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| S | Rogue admin API impersonation | ⚠️ | CLI talks to `127.0.0.1:<admin_port>` with the bearer token; a local attacker with port control could impersonate the API. Accepted: that attacker already controls the operator's user session. |
| T | Template tampering after generation | ✅ | Generated projects are git repos with an initial commit; `doctor --security` re-checks tracked-state invariants. |
| I | Secrets in shell history / CI logs | ✅ | `keys upstream set` reads hidden input; tokens/keys never echoed; CI redacts (gitleaks `--redact`). |

## 7. LLM-specific risks

| Threat | Vector | Status | Mitigation |
|---|---|---|---|
| T | Prompt injection driving the model | ⚠️ | Baseline detection hook (pluggable middleware, `HOOKS_DISABLED=baseline` to opt out) flags common injection patterns and rejects the request — heuristic, not a guarantee; the hard guardrails (input caps, budgets) bound the damage. |
| D | Cost-bombing via crafted prompts | ✅ | Token budgets are the control (per-key + global), enforced before upstream calls. |
| I | System-prompt extraction | ⚠️ | Baseline hook flags "reveal instructions" patterns; ultimately the model's alignment is the second line — document operator prompts accordingly. |

## Residual-risk summary

1. Single-instance premise: rate limits/budgets are in-process; multi-instance
   deployments need a shared limiter (documented, backlog).
2. Prod DB role hardening (separate migrate role, revoke UPDATE/DELETE on
   append-only tables) is planned; today the app role is the only role.
3. Key rotation and mTLS are backlog items.
4. The threat model assumes the operator is trusted; llmberth defends the
   *boundary* (internet ↔ app ↔ upstream), not the operator against themselves.
