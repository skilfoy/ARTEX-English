# ARTEX English

An English-language fork of [Autumn-27/ARTEX](https://github.com/Autumn-27/ARTEX), an autonomous security research system with a Go backend, PostgreSQL storage, and a Next.js interface.

The interface contains task planning, agent sessions, findings, asset graphs, traffic evidence, approval records, model configuration, and system settings. The hosted preview uses simulated data. A functioning local installation requires PostgreSQL and an LLM configuration.

This fork translates the interface, preview data, built-in agent prompts, backend messages, and documentation. Existing installations keep previously saved prompts until an administrator resets them to the built-in defaults.

[Open the English demo](https://artex-english.vercel.app/function/tasks)

## Preview and deployment

Full-stack Terraform configurations for AWS, Google Cloud, and Azure are documented in [infra/README.md](infra/README.md). Each configuration builds this fork's Go backend with the frontend embedded and runs it with PostgreSQL. Review the upstream author's usage conditions below before provisioning or using the software.

The `web/` directory supports a frontend-only demo with `NEXT_PUBLIC_MOCK=1`. This mode uses simulated tasks, assets, and findings and does not run the Go backend or make requests to real targets.

For Vercel, import this repository with these settings:

| Setting | Value |
| --- | --- |
| Root directory | `web` |
| Framework | Next.js |
| Build command | `NEXT_PUBLIC_MOCK=1 npm run build` |

The build command is included in [web/vercel.json](web/vercel.json).

The full application serves its API and event streams from the Go process. A Vercel deployment of `web/` provides the frontend preview only.

To run the preview locally:

```bash
cd web
npm ci
NEXT_PUBLIC_MOCK=1 npm run dev
```

Open the address printed by Next.js. The mock preview requires no database or model credentials.

## Build the English application

The default Docker Compose configuration builds the frontend and backend from this fork's source. Copy the example environment file, set a strong database password, and start the services:

```bash
cp .env.example .env
# Edit POSTGRES_PASSWORD in .env.
docker compose up -d --build
```

Open `http://localhost:8787` to complete administrator setup. Add an LLM provider through the interface or the supported environment variables, including `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`.

For a native build, install the versions of Go and Node.js specified by `go.mod` and `web/package.json`, and provide a PostgreSQL instance. Build the static interface and embed it in the Go binary:

```bash
cd web
npm ci
npm run build:static
cd ..
mkdir -p server/webui/dist
cp -R web/out/. server/webui/dist/
CGO_ENABLED=0 go build -tags embedui -o artex ./cmd/artex
cp config.example.json config.json
./start.sh
```

Set the database connection in `config.json` or `ARTEX_PG_DSN`. `start.sh` manages restarts requested by the in-app updater. The updater checks releases from this fork.

## Application structure

| Directory | Role |
| --- | --- |
| `agent/` | Agent roles, prompts, and tools |
| `db/` | PostgreSQL storage and schema |
| `server/` | HTTP API, authentication, and event streams |
| `traffic/` and `evidence/` | Traffic capture and evidence storage |
| `web/` | Next.js interface and simulated preview data |
| `skills/` | Agent skill instructions |

The backend coordinates a planner and workers around an exploration graph and a shared asset graph. The interface presents tasks, session traces, discoveries, asset coverage, and approval decisions. See the [architecture guide](docs/ARCHITECTURE.md) for the data model and execution flow.

## Development checks

```bash
go test ./...
cd web
npm ci
npx tsc --noEmit
NEXT_PUBLIC_MOCK=1 npm run build
```

For integrated local development, `./dev.sh` starts the Go backend, traffic proxy, and Next.js development server. The [cloud deployment guide](infra/README.md) covers virtual machines, DNS, HTTPS, and operations.

## License and use conditions

ARTEX is licensed under the [GNU Affero General Public License, version 3](LICENSE). Preserve the license, source disclosure obligations, and attribution when distributing modified versions or offering them over a network.

The upstream author states that use is restricted to personal study, source code research, and technical verification in a locally isolated environment. The author expressly prohibits scanning, probing, exploiting, or attacking websites, online services, or networked systems, including systems owned by or authorized for the user. The stated restrictions also prohibit actual penetration testing, attack and defense exercises, production use, and unlawful or destructive activity. Users are responsible for applicable legal requirements. The software is provided as is without warranties, and the author disclaims liability for losses arising from its use. The application displays these conditions at login. Consult the [upstream source](https://github.com/Autumn-27/ARTEX/blob/main/README.md) for the author's complete notice.

This fork acknowledges [Autumn-27 and the upstream contributors](https://github.com/Autumn-27/ARTEX/graphs/contributors). Its English text is a translation and adaptation for accessibility. Git history preserves the upstream source and its original wording.
