# vhs tape for the README demo GIF.
# Regenerate: go build -o /tmp/slack-tui-demo . && vhs docs/demo.tape
# Runs against the built-in mock workspace (scratch HOME, no tokens).

Output docs/demo.gif

Set FontSize 15
Set Width 1100
Set Height 640
Set Theme "Catppuccin Mocha"
Set Padding 12
Set TypingSpeed 40ms

Hide
Type `export HOME=/tmp/vhs-demo XDG_CONFIG_HOME=/tmp/vhs-demo/.config SLACK_USER_TOKEN= SLACK_APP_TOKEN= SLACK_BOT_TOKEN=`
Enter
Type `mkdir -p $XDG_CONFIG_HOME/slack-tui && printf '{"handle":"you","theme":"charcoal","accent":"auto","density":"comfortable","status":"online","onboarded":true}' > $XDG_CONFIG_HOME/slack-tui/prefs.json; clear`
Enter
Type "/tmp/slack-tui-demo"
Enter
Sleep 1s
Show

Sleep 1.5s
Type "k" Sleep 300ms
Type "k" Sleep 300ms
Type "t" Sleep 1.5s
Escape Sleep 600ms

# command palette: fuzzy jump
Ctrl+K Sleep 800ms
Type "rndm" Sleep 900ms
Enter Sleep 1.2s

# write with mention autocomplete
Type "i" Sleep 500ms
Type "looks great " Sleep 300ms
Type "@ad" Sleep 900ms
Tab Sleep 600ms
Escape Sleep 100ms
Escape Sleep 600ms

# settings + theme
Type "," Sleep 900ms
Type "l" Sleep 700ms
Type "l" Sleep 700ms
Escape Sleep 800ms

# help
Type "?" Sleep 1.8s
Escape Sleep 700ms

Type "q"
Sleep 500ms
