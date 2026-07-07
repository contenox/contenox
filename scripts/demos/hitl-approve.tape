# HITL demo: a destructive command needs explicit human approval before it runs.
# Uses demo-chain.json (adds rm/touch to the shell allowlist; the HITL policy still gates rm).
# Render: cd scripts/demos && vhs hitl-approve.tape

Output out/hitl-approve.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 4m

Hide
Type "mkdir -p /tmp/contenox-demo/.contenox && cd /tmp/contenox-demo && touch build-cache.tmp && export DEMO_CHAIN=/home/naro/src/github.com/contenox/enterprise/runtime/scripts/demos/demo-chain.json && clear"
Enter
Wait
Show

Type "ls"
Sleep 500ms
Enter
Wait
Sleep 1.5s

Type `contenox --chain $DEMO_CHAIN --shell "Delete build-cache.tmp. Just run rm once, do not verify afterwards."`
Sleep 800ms
Enter

Wait+Screen /Approve\? \[y\/N\]:/
Sleep 2.5s
Type "y"
Sleep 500ms
Enter
Wait
Sleep 1.5s

Type "ls"
Sleep 500ms
Enter
Wait
Sleep 3.5s
