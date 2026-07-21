# agent-check demo: verify a registered external ACP agent with one live turn —
# streamed reply, stop reason, advertised commands, forwarded MCP servers.
# Seed OFF-CAMERA first (docs/development/recording-shot-list.md, Take 4): a
# registered `demo-agent` whose config declares an mcp_servers allowlist and that
# advertises at least one slash command, e.g.
#   contenox agent add demo-agent -- <acp-agent-cmd>
#   contenox agent edit demo-agent        # add "mcp_servers": ["..."]
# Render: cd scripts/demos && vhs agent-check.tape && ./mkgif.sh agent-check

Output out/agent-check.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 4m

Type `contenox agent check demo-agent "summarize this repo in one line"`
Sleep 800ms
Enter

Wait+Screen /Forwarding MCP servers:/
Sleep 1.5s

Wait+Screen /stopReason=/
Sleep 1s

Wait+Screen /advertises/
Sleep 3.5s
