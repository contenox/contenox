# Quickstart demo: registered backends -> first chat on a local model (modeld/llama).
# Render: cd scripts/demos && vhs quickstart.tape

Output out/quickstart.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 5m

Hide
Type "mkdir -p /tmp/contenox-demo/.contenox && cd /tmp/contenox-demo && clear"
Enter
Wait
Show

Type "contenox backend list"
Sleep 800ms
Enter
Wait
Sleep 2s

Type `contenox "In one short sentence: what can you help me with here?"`
Sleep 800ms
Enter
Wait
Sleep 3.5s
