# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| latest release on main | ✅ |
| older tags | ❌ (upgrade — `llmberth` projects regenerate cheaply) |

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's **private vulnerability reporting**:
<https://github.com/thy-bupt/llmberth/security/advisories/new>

This is the primary and preferred channel — it is private end-to-end and
lets us coordinate a fix and credit you in the advisory.

Include, where possible:

- affected component (CLI / generated app / template / CI) and version
  (`llmberth --version`, or the template_version from `.llmberth.yaml`);
- minimal reproduction (config, command, request);
- impact assessment from your perspective.

You will receive an acknowledgement within **72 hours** and a status update
at least every **7 days** until resolution. Fixes land on `main` first;
credits are given in the advisory unless you prefer otherwise.

## Scope

In scope:

- the `llmberth` CLI (this repository);
- the generated application template (`template/app/`) — auth, key
  handling, admin API, ledger, container/compose hardening;
- the CI pipelines of this repository and of generated projects.

Out of scope:

- vulnerabilities in the upstream LLM providers the generated app calls;
- misconfiguration of the operator's host (VPS, DNS, firewall) that
  `llmberth doctor --security` already warns about;
- social engineering of end users.

## Security design references

- `docs/STRIDE.md` — threat model per component (spoofing, tampering,
  repudiation, information disclosure, DoS, elevation).
- `README.md` — the security defaults every generated app ships with.
- `llmberth doctor --security` — the twelve-item self-audit that must pass
  before an app is considered deployment-ready.
