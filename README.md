# llmberth

> Self-hosted LLM app **generate + operate** toolkit: one command generates a
> production-grade Go backend that you fully own — and the same binary
> manages its whole life.

**vs LiteLLM and every hosted gateway:** llmberth does not proxy your
traffic. It *generates your own code* — a complete, OpenAI-compatible Go
backend with zero runtime dependency on llmberth. Fork it, bend it, ship it;
the source, the data and the keys never leave your infrastructure.

**Zero telemetry.** Nothing phones home. Ever.

```console
$ llmberth init my-app --provider=openai --ui=single-page
Generating "my-app" (provider=openai, ui=single-page, db=postgres)
Wrote 35 files. Template version 0.1.0.

$ cd my-app && docker compose up -d --wait   # postgres + app, loopback-only ports

$ curl -s http://127.0.0.1:8090/admin/keys \
    -H "Authorization: Bearer $(cat .llmberth-admin-token)" -d '{"name":"first"}'
{"key":"lbt_live_…","warning":"store this key now — it is never displayed again"}

$ curl -s http://127.0.0.1:8080/v1/chat/completions \
    -H "Authorization: Bearer lbt_live_…" -H "Content-Type: application/json" \
    -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}'
{"choices":[{"message":{"role":"assistant","content":"…"}}],"usage":{…}}
```

<!-- TODO(M1 close): replace the console block above with an animated CLI
     demo (GIF/asciinema). TUI demo lands with M3. -->

## What you get, out of the box

Every generated app ships with the LLM production parts most templates leave
as security homework:

- **OpenAI-compatible API** — `POST /v1/chat/completions` (stream & non-stream);
  any OpenAI SDK connects by pointing `base_url` at it.
- **Token ledger + cost estimates** — every request accounted (`usage_ledger`),
  costs estimated from a replaceable price table, exposed via
  `X-LLM-Cost-Usd` (always labeled *estimate*).
- **API keys** — `lbt_live_…`, hashed at rest (SHA-256 + constant-time
  compare, 8-char prefix only), per-key **rate limit** and **budget**,
  **instant revocation** through the loopback admin API.
- **Audit log** — every management action appended, append-only.
- **Log discipline** — message bodies are never logged; DB persistence is
  opt-in (`LOG_MESSAGES=1`).
- **SSRF guard** — upstream `base_url` must be in a host allowlist (fail closed).
- **Bundled chat UI** (optional) — strict CSP, no inline JS.
- **Providers** — OpenAI / DeepSeek / Qwen / vLLM / Ollama via one
  OpenAI-compatible client with configurable `base_url`; a built-in **fake
  provider** powers demos and tests with no network and no keys.

## The life of a project (single binary)

```console
llmberth init <name>       # generate: source is yours from commit one
llmberth dev               # hot-reload dev stack            (M2, v0.2)
llmberth up / stop / status
llmberth logs [-f]
llmberth keys add|list|revoke
llmberth usage [--by key|model]
llmberth doctor [--security]   # 12-point security audit     (M4, v0.4)
```

v0.1 (current) delivers `init` plus the full generated app; lifecycle
commands land in v0.2, the TUI in v0.3, Caddy TLS + `doctor --security` in
v0.4. See [PLAN.md](../PLAN.md) for the milestone map.

## Architecture

```
client (any OpenAI SDK / bundled UI)
   │  Authorization: Bearer lbt_live_…
   ▼
app (chi) ── auth ▸ rate limit ▸ budget ▸ provider ──▶ upstream LLM
   │                    (base_url allowlist)
   ├─ Postgres (internal network; ledger, keys, audit, history)
   └─ admin API — loopback only, bearer-token, fail closed (CLI channel)
```

Generated project layout: single Go module, embedded migrations
(golang-migrate), `docker compose` with **all ports published to 127.0.0.1
only**, distroless + non-root container image, Makefile, and its own CI
(lint + fake-provider tests + govulncheck + compose validation).

## Requirements

- Go 1.25+ (to build/extend the generated app)
- Docker with compose v2
- An upstream LLM key — or none: `--provider=fake` for a zero-dependency demo

## Development

```console
make build   # build the llmberth CLI
make test    # unit tests
make golden  # golden generation test: snapshot diff + build + test + compose
make e2e     # full local acceptance: init → up → key → chat → revoke → 401
```

## Security

Secrets discipline: the CLI and templates never embed credential literals;
`.env` files are gitignored, keys are hashed at rest, admin tokens live in a
0600 file. `docs/SECURITY.md` (responsible disclosure) and the STRIDE
threat model land with v0.4.

## License

Apache-2.0. © The llmberth Authors.
