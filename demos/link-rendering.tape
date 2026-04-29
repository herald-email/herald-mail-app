Output assets/demo/link-rendering.gif

Set Theme "Builtin Solarized Dark"
Set FontSize 13
Set Width 1400
Set Height 700
Set Padding 8

Type "./bin/herald --demo"
Enter
Sleep 3s

# Find the link-heavy fixture without relying on row position
Type "/"
Sleep 0.2s
Type "Link rendering stress"
Sleep 0.2s
Enter
Sleep 0.8s

# Open preview
Enter
Sleep 1s

# Move focus to the preview and scroll to the link-heavy section
Tab
Sleep 0.3s
Down
Sleep 0.2s
Down
Sleep 0.2s
Down
Sleep 0.2s

Type "q"
