# Side questions with `/btw`

A side question lets a user ask about the current agent session without adding the exchange to the main transcript. Enter `/btw` followed by a question in the chat input, or open side question history with `/btw` alone. The side panel displays the answer and earlier side questions.

The service takes a structured snapshot of the main agent's latest completed model request and its actual provider selection. It includes the system instructions, messages, tool definitions, and generation settings needed to answer the question. The side question uses a separate model request with no executable tools. Its answer and usage are saved independently from the main session.

## Session behavior

- One side question may run for each parent session. The service permits up to four concurrent side questions across sessions.
- Closing the panel or disconnecting the event stream leaves the request running. The answer remains available in history.
- Cancel stops the current side question. Clear cancels it and removes the side question history for that parent session.
- A server restart marks an unfinished request as interrupted and retains its saved partial answer. A later question can use the most recent valid snapshot.
- A deleted model configuration or a changed model identity requires a fresh main agent snapshot before another side question.
- Side questions have a 120 second request limit and a 4,000 character question limit.

Snapshots and side question records are stored in PostgreSQL tables `side_question_sessions` and `side_question_requests`. A conversation ID identifies a normal chat. Task and worker questions use their task, exploration, and intent identifiers. The service checks the same parent resource permissions as the main session.

## HTTP endpoints

`{parent}` is one of `/api/conversations/{id}`, `/api/tasks/{id}/chat`, or `/api/tasks/{id}/intents/{iid}`.

| Request | Purpose |
| --- | --- |
| `GET {parent}/side-questions?before={ordinal}` | Read history, current status, and snapshot metadata. History is paginated. |
| `POST {parent}/side-questions` | Submit `{ "question": "...", "client_request_id": "UUID" }`. Reusing the same request ID is idempotent. |
| `DELETE {parent}/side-questions` | Cancel and clear the parent's side question history. |
| `GET /api/side-questions/{requestID}/events` | Receive cumulative answer and status updates over server sent events. |
| `POST /api/side-questions/{requestID}/cancel` | Cancel an active request. |

The frontend merges event updates by request ID and sequence number. It discards updates for a previous parent session after the user switches context or clears history.

[Context budgets and recovery](CONTEXT_BUDGET.md) describe how a large main session is prepared for a side question.
