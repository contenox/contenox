# Install + local-first onboarding demo: install.sh -> contenox setup (downloads
# the modeld daemon) -> model pull -> first prompt on a local model.
# Uses a throwaway HOME and a PATH without sudo so the installer takes the
# ~/.local/bin fallback. Renders slow (real downloads); mkgif.sh compresses.
# Render: cd scripts/demos && vhs install.tape

Output out/install.mp4

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 24
Set TypingSpeed 45ms
Set WaitTimeout 8m

Hide
Type "source ./install-demo-env.sh && clear"
Enter
Wait
Show

Type "# from https://contenox.com/install.sh"
Enter
Type "sh install.sh"
Sleep 500ms
Enter
Wait
Sleep 2s

Type `export PATH="$HOME/.local/bin:$PATH"`
Sleep 300ms
Enter
Wait

Type "clear && contenox setup"
Sleep 500ms
Enter
Wait+Screen /q to quit/
Sleep 2s
Type "5"
Sleep 300ms
Enter
Wait
Sleep 2.5s

Type "clear"
Sleep 300ms
Enter
Wait

Type "~/.contenox/modeld/v*/linux-amd64/modeld serve >/dev/null 2>&1 &"
Sleep 500ms
Enter
Wait
Sleep 1.5s

Type "contenox model pull qwen2.5-coder-1.5b"
Sleep 500ms
Enter
Wait
Sleep 1.5s

Type `contenox "hey, what can you do for me here?"`
Sleep 500ms
Enter
Wait
Sleep 4s
