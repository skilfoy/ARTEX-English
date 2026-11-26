# Security

This repository has two lines.

| Branch | What it is | Expose it? |
| --- | --- | --- |
| `main` | Natural-English archive of upstream v0.3.14 | No |
| `next` | Operator distribution based on BaaRai/ARTEX v0.7.0, plus this fork's cloud installer and defender pack | Only inside a lab you control |

`main` inherits a publicly reported console authentication weakness from v0.3.14. Maintained lines after that release fixed it. This archive does not try to re-implement that fix in place. Use `next`, or do not run it.

## Reporting

Report vulnerabilities in this fork to the maintainer through [GitHub private vulnerability reporting](https://github.com/skilfoy/ARTEX-English/security) on this repository, or by opening a GitHub security advisory. Do not file an exploit in a public issue.

This is not the deleted Autumn-27 project and it is not ThreatBook RedAgents. Reports about those belong with their owners. Reports about BaaRai/ARTEX itself also belong [there](https://github.com/BaaRai/ARTEX/blob/main/SECURITY.md) when the behavior is unchanged from that base.

## What this archive changed

* In-app updates are off unless `ARTEX_UPDATES=1`. The default path does not contact GitHub and does not swap the binary.
* Local Compose publishes `8787` and `8788` on `127.0.0.1` only.
* The cloud stack does not publish Postgres, the console, or the recording proxy. SSH and the web ports are limited to `admin_cidr`. `admin_cidr` cannot be `0.0.0.0/0`.
* CI rejects the prebuilt image names `tomicdd/artex` and `congdonglinux/artex`. Those images were reported in November 2026 to carry an unauthenticated command backdoor. Build from this source tree. Do not run a community image you did not build.
* There is no recommended GitHub Release of this archive. A tag on `main` is marked a prerelease and is not `latest`.

## Operator rules

* Run it only against systems you own or have written authorization to test.
* Do not publish port 8787 or 8788.
* Keep the recording-proxy data and Postgres private. Both can contain credentials from the lab.
* Treat agent tool output as untrusted. The `next` line frames that output by default. This archive does not.
* A self-update that is switched on becomes a remote code path. Leave it off until you are on `next` and you have verified the release checksums yourself.

## Supply chain

Build the binary and the container from this repository. Pin anything you mirror. The floating tag `postgres:16-alpine` in Compose is still a gap on `main`; do not "fix" it by switching to an unpublished third-party ARTEX image.

Known-bad image names, listed so they can be blocked:

* `tomicdd/artex`
* `congdonglinux/artex`

## Intended use

ARTEX can execute commands and send network traffic. That is the product. Authorization is not created by installing it. The original author also displayed a stricter study-only notice at login. That notice remains in this archive. Read it before you start the process.
