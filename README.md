# ARTEX English

An English-language fork of [Autumn-27/ARTEX](https://github.com/Autumn-27/ARTEX), an autonomous security research system with a Go backend, PostgreSQL storage, and a Next.js interface. The original Chinese README is preserved as [README.zh-CN.md](README.zh-CN.md).

The interface contains task planning, agent sessions, findings, asset graphs, traffic evidence, approval records, model configuration, and system settings. The hosted preview uses simulated data. A functioning local installation requires PostgreSQL and an LLM configuration.

This fork translates the interface, preview data, built-in agent prompts, and default agent descriptions. Existing installations keep previously saved prompts until an administrator resets them to the built-in defaults. Some legacy backend messages and developer comments remain in Chinese.

[Open the English demo](https://artex-english.vercel.app/function/tasks)

## Preview and deployment

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

The upstream Docker image contains the upstream build. Compile this fork from source to include its English interface.

1. Install Go and Node.js versions compatible with `go.mod` and `web/package.json`. Provide a PostgreSQL instance.
2. Build the static interface and copy it into the directory embedded by the Go server.
3. Compile the Go binary with the `embedui` build tag.

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

Set the database connection in `config.json` or `ARTEX_PG_DSN`. Configure a model through the interface or with the supported environment variables, including `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`. The default application address is `http://localhost:8787`. The initial setup page creates the administrator password.

`start.sh` manages restarts required by the in-app updater. The updater currently targets upstream releases, which contain upstream builds. Keep the English build by recompiling this fork after upstream updates.

## Application structure

| Directory | Role |
| --- | --- |
| `agent/` | Agent roles, prompts, and tools |
| `db/` | PostgreSQL storage and schema |
| `server/` | HTTP API, authentication, and event streams |
| `traffic/` and `evidence/` | Traffic capture and evidence storage |
| `web/` | Next.js interface and simulated preview data |
| `skills/` | Agent skill instructions |

The backend coordinates a planner and workers around an exploration graph and a shared asset graph. The interface presents tasks, session traces, discoveries, asset coverage, and approval decisions. See the upstream [architecture documentation](https://github.com/Autumn-27/ARTEX#system-technical-architecture) for the full design.

## Development checks

```bash
go test ./...
cd web
npm ci
npx tsc --noEmit
NEXT_PUBLIC_MOCK=1 npm run build
```

For integrated local development, `./dev.sh` starts the Go backend, traffic proxy, and Next.js development server. The upstream README contains additional installation, update, and reverse proxy details.

## License and use conditions

ARTEX is licensed under the [GNU Affero General Public License, version 3](LICENSE). Preserve the license and attribution when distributing modified versions. The upstream author also supplies usage restrictions and disclaimers in the original README and the application's login notice. Review those terms before using or redistributing the software.

This fork acknowledges [Autumn-27 and the upstream contributors](https://github.com/Autumn-27/ARTEX/graphs/contributors). Its English text is a translation and adaptation for accessibility. The original Chinese wording remains available in [README.zh-CN.md](README.zh-CN.md).
