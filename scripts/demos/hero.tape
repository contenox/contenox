# Hero demo: natural-language task -> agent reads the repo -> answer.
# Render: cd scripts/demos && vhs hero.tape   (then see mkgif.sh for GIF post-processing)

Output out/hero.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 4m

Hide
Type "mkdir -p /tmp/contenox-demo/.contenox && cd /tmp/contenox-demo && cp ~/src/github.com/contenox/runtime/README.md . && clear"
Enter
Wait
Show

Type `contenox --shell "Read README.md and tell me in one sentence what this project does."`
Sleep 800ms
Enter
Wait
Sleep 4s
