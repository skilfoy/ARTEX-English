# ARTEX English, operator distribution

This branch is `next`. It is the line to run. [`main`](https://github.com/skilfoy/ARTEX-English/tree/main) is the natural-English archive of upstream v0.3.14 and has a publicly reported console authentication weakness. Do not expose `main`.

## What this tag is

`v0.7.0-en.0` is [BaaRai/ARTEX v0.7.0](https://github.com/BaaRai/ARTEX/releases/tag/v0.7.0) (`22ae88afabf66e47e73644a0528d8b427c4c80b8`) plus the files this distribution adds. The agent, the evidence model, the scope gates, and the console are theirs. AGPL-3.0 still applies, including if you offer the program as a network service.

This repository is not Autumn-27, not ThreatBook RedAgents, and not the BaaRai project. Do not send them bugs that exist only in `infra/` or `detections/`.

## What was replayed from the English archive

* Cloud installer for one virtual machine on AWS, Google Cloud, or Azure: [`infra/`](infra/README.md). The VM builds this tree. It does not pull a prebuilt ARTEX image. `git_ref` defaults to `next`.
* `admin_cidr` is required and cannot be `0.0.0.0/0`. Postgres, port 8787, and port 8788 are not published. Caddy is the only public listener, and the firewall starts limited to that admin network.
* The hosted mock preview stays the Vercel project on the archive. BaaRai already supports `NEXT_PUBLIC_MOCK=1` in this tree, so a frontend-only demo does not need the old branch.

## What was added here

* [`detections/`](detections/README.md): Sigma, Suricata, YARA, and a CSV of image names for the console process and for the prebuilt images reported backdoored in November 2026. A unit test checks that the pack still names them.
* [`scripts/check_supply_chain.py`](scripts/check_supply_chain.py), run from [`.github/workflows/distribution.yml`](.github/workflows/distribution.yml). Those image names may appear in the detection pack and in this document. They may not appear anywhere else.
* [`docker-compose.cloud.yml`](docker-compose.cloud.yml) for the VM: source build, `no-new-privileges`, update checks off, trusted proxy set to the Compose networks.

## Base images

Postgres and Caddy are pinned to a multi-arch index digest in [`infra/image-pins.txt`](infra/image-pins.txt). The same Postgres digest is used by local Compose, the cloud Compose file, `install.sh`, and CI. [`scripts/check_image_pins.py`](scripts/check_image_pins.py) fails if any of those pull the tag without that digest.

Resolved 2026-11-26 from the Docker Hub registry. These are index digests, so amd64 and arm64 both resolve. Do not replace one with the digest of a single architecture.

To bump:

1. Read the tag's manifest list, not one platform. `crane digest postgres:16-alpine` and `crane digest caddy:2-alpine` print the index digest.
2. Replace the digest in `infra/image-pins.txt` and in every file named by the checker.
3. Run `python3 scripts/check_image_pins.py`.

## MCP

This tree does not listen for MCP. There is no MCP port to publish.

* `mcphttp` is a client. An HTTP or SSE server is an outbound connection the operator saves in the console. `GET` and `POST /api/mcp` sit behind the console JWT. `/api/health` and `/api/ready` are the only API routes that answer without a token, and an anonymous health response is only `{"ok": true}`.
* Adding or changing an MCP server is an approval-gated tool (`create_mcp`, `update_mcp`). A stdio entry starts a command on this host. An HTTP entry sends the saved headers to that URL.
* The built-in browser server is stdio (`playwright-mcp`), seeded disabled. It is not a socket.
* The console flag default is `127.0.0.1:8787`. The container command is `-addr :8787`, so the process listens on all interfaces inside the container. Local Compose publishes that only on `127.0.0.1`. Cloud Compose does not publish `8787` or `8788`. Caddy on 80 and 443 is the only published listener, and Terraform starts it limited to `admin_cidr`.
* `install.sh` publishes its optional Postgres container on `127.0.0.1:5432` only.
* Do not add a host port for Postgres, the console, the recording proxy, or an MCP URL.

## What this tag does not do

* It does not add attack skills, scope removal, or a new agent loop.
* It does not turn update checks on. `ARTEX_UPDATE_CHECK` stays opt-in, and the cloud file sets it to `0`. An update banner that is pointed at another repository would replace this binary with someone else's.
* It does not publish a Docker Hub image. The inherited release workflow pushes an image only if the `DOCKERHUB_ENABLED` repository variable is set. Leave it unset.
* Maintainer GPG signatures are not configured on this account. The inherited release workflow does produce SHA-256 sums, a CycloneDX SBOM, and a GitHub build-provenance attestation when a `v*` tag is pushed. Verify those. They prove which workflow built the files. They are not a personal signature.

## Use

Run it only against systems you own or have written authorization to test, on a network you control. The original author's stricter study-only notice is still in the archive history. Authorization is not created by installing the program.

Build from this source. Refuse these image names:

* `tomicdd/artex`
* `congdonglinux/artex`
