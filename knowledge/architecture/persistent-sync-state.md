---
type: Decision
title: Persistent Hub Sync State Across CLI Invocations
description: "okf hub push, pull, and sync persist the last-synced head and tree in a per-device .okf-sync-state.json so each new process keeps a correct CAS expected head and 3-way reconcile base."
tags: [sync, hub, reconcile, cas, state]
generated: { by: agent/cli, at: "2026-10-07T19:22:20Z" }
status: stable
---

# Persistent Hub Sync State Across CLI Invocations

## Context
The sync engine keeps the last commit agreed with the hub (`cachedHead`) and that commit's tree (`cachedTree`). Before this decision they lived only in memory, so every `okf hub` process started as if it had never synced:

* A second `push` from the same device sent no expected head and was rejected with `409 Conflict`.
* `sync` reconciled against an empty base, so every local edit looked like a same-file collision: the remote version was restored and the edit moved to `<file>.conflict-local.md`.
* Every file was re-encrypted on each push, defeating `plaintext_hash` change detection.

Engine tests did not catch this because they reuse one in-memory engine across operations; the CLI never does.

## Decision
`HubPush`, `HubPull`, and `HubSync` enable persistence via `Engine.LoadState(<bundle>/.okf-sync-state.json)`. After each successful operation the engine writes the vault ID, head, and serialized tree to that file.

* **Atomic and private**: written to a temp file and renamed; owner-only permissions.
* **Never uploaded**: dotfiles are skipped by `ScanBundle`, like `.okf-vault.json`.
* **Vault-scoped**: state recorded for a different vault ID (or an unknown format version) is ignored.
* **Fail-soft**: if the remote operation succeeds but the state cannot be written, the engine returns the result with a `StateSaveError`; the CLI prints a warning instead of reporting failure. The next run falls back to a full reconcile.
* **Opt-in for library callers**: `NewEngine` is unchanged; engines without `LoadState` keep in-memory behavior.

## Zero-Knowledge Impact
None. The file stays on the device and records only paths and hashes of files already present there in plaintext.

# Related Concepts
- [Zero-Knowledge Vault Cryptography and Blind Sync Architecture](zero-knowledge-vault-sync.md): Restores the CAS expected head and 3-way reconcile base that the sync protocol assumes across CLI invocations
