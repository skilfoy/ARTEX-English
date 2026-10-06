# Side question context budgets

A side question uses a copy of the latest completed main agent request. The preparation step estimates tokens from UTF-8 content, system instructions, tool schemas, and message structure. This estimate provides a budget guard and is not a provider token count.

The input budget reserves space for the model's answer and a safety margin. The answer limit defaults to at most 8,192 tokens, subject to the selected model's configured limit. `ARTEX_BTW_MAX_OUTPUT_TOKENS` sets a ceiling from 256 to 32,768 tokens. An unknown context window uses the service's default window estimate.

The service includes recent successful side question exchanges, subject to the input budget. Older exchanges can be summarized into rolling memory. If the main snapshot still exceeds the available budget, older copied messages are summarized while recent structured tool calls and their results stay paired. Summarization has a bounded call count and shares the side question's 120 second request limit.

A provider context overflow before answer content or a tool call permits one additional reduction attempt. Preparation stops if the estimated size does not decrease. The service records usage reported by the provider for preparation and answering. A provider that omits usage leaves those fields at zero rather than substituting an estimate.

PostgreSQL stores rolling memory and preparation status alongside the side question session. Version checks prevent an older snapshot or a late write from restoring cleared history. An interrupted request retains its saved partial answer and usage. Task archives include these records, with compatibility handling for older archive versions.

## Verification

Use an isolated PostgreSQL database for integration tests:

```bash
go test ./sidequestion ./db ./server -run 'TestSide|TestCheckpoint|TestSnapshot|TestBuildRequest|TestService|TestMainSide|TestTaskArchive' -count=1
cd web
npx tsc --noEmit
```
