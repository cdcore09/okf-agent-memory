---
type: Decision
title: Hub Vault Credentials From Environment
description: "okf hub push, pull, and sync accept the vault password and Secret Key from OKF_HUB_PASSWORD and OKF_HUB_SECRET_KEY, with flags taking precedence and no config-file fallback."
tags: [hub, sync, security, credentials, env]
generated: { by: agent/cli, at: "2026-10-07T19:51:41Z" }
status: stable
---

# Hub Vault Credentials From Environment

## Context
The vault master password and Secret Key were accepted only as `-password` and `-secret-key` flags. Command-line arguments are visible to other local processes and often land in shell history, which is a poor fit for the two secrets that unlock a zero-knowledge vault, especially for agents and daemons that run sync unattended.

## Decision
* `ResolvePassword` and `ResolveSecretKey` resolve the flag first, then `OKF_HUB_PASSWORD` and `OKF_HUB_SECRET_KEY`.
* There is **no** `.okf-vault.json` fallback, unlike the hub auth token: storing key material next to the bundle would defeat the vault's purpose.
* The documented recommendation is to inject the variables from a secret manager rather than pass flags.

# Related Concepts
- [Zero-Knowledge Vault Cryptography and Blind Sync Architecture](../architecture/zero-knowledge-vault-sync.md): Extends the hub credential resolution rules (alongside OKF_HUB_TOKEN) to the vault password and Secret Key
