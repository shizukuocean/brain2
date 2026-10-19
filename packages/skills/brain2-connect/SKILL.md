---
name: brain2-connect
description: Connect an AI agent to a governed local Brain2 workspace through the Brain2 MCP tools. Use when the user asks to connect, enter, read, update, remember in, hand off through, or check a Brain2 workspace, including phrases such as “读取大脑”, “接入 Brain2”, “记一下”, “写进大脑”, “交接”, or “检查大脑”.
---

# Brain2 Connect

Treat Brain2 as a governed control system, not an unstructured notes folder.

## Connect

1. When Launcher provides `connection_id`, `nonce`, and `agent_id`, call `brain2_get_bootstrap` with those exact values.
2. Read the returned entry documents before acting.
3. Call `brain2_health` with the same `connection_id`. Verify its `workspace_id` and `brain2_root` match the bootstrap response before continuing. The tool returns a bounded summary; read `.brain2/health-report.json` only when detailed findings are needed.
4. Complete the connection only through `brain2_complete_handshake`. Never claim a connection from file installation alone.

## Work

1. Use `brain2_read_file` for additional Brain2 context.
2. Identify the current device, Agent identity, task owner, project entry, and write boundary.
3. Keep live code and data in their local source repositories. Do not use a cloud handoff directory as a live development root.
4. Before writing, follow the governance and D6 feedback rules returned by Brain2.
5. Do not expose connection nonces, credentials, or private Brain2 contents outside the active task.
6. Treat project discovery as a proposal. Submit candidates through `brain2_submit_project_candidates`; do not create project notes, move files, or authorize Blueprint/Protector on the user's behalf.
7. A Launcher project registration is metadata-only. Reading, changing, protecting, or scaffolding that project requires a separate user-approved task and scope.
8. After completing a meaningful development node, update the existing project progress entry with the verified result, changed source location, remaining risk, and next action. Use the workspace's governed write path and avoid duplicate progress logs. If the current session has no authorized write path, say that progress is not yet recorded and generate a Launcher/Agent handoff instead of claiming success.

## Failure handling

- If MCP tools are unavailable, say that the environment is installed but the current session is not connected, then use the Launcher Prompt handoff.
- If health is `critical`, report the blocking checks and do not submit a successful handshake. If it is `needs_attention`, report the non-blocking summary and continue the handshake without repairing content.
- If a connection task is expired or mismatched, request a fresh connection from Launcher.
