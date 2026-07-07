# "The Chain is the contract": a command outside the chain's allowlist is hard-blocked
# by the engine, and the policy lives in a reviewable JSON file.
# Render: cd scripts/demos && vhs chain-blocked.tape

Output out/chain-blocked.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 4m

Hide
Type "mkdir -p /tmp/contenox-demo/.contenox && cd /tmp/contenox-demo && clear"
Enter
Wait
Show

Type `contenox --auto --shell --local-exec-allowed-dir . "Run 'sudo systemctl restart nginx'"`
Sleep 800ms
Enter
Wait
Sleep 2.5s

Type "jq '.tasks[1].execute_config.tools_policies.local_shell' ~/.contenox/default-chain.json"
Sleep 800ms
Enter
Wait
Sleep 3.5s
