Output assets/demo/calendar-invitation.gif

Set Theme "Builtin Solarized Dark"
Set FontSize 13
Set Width 1400
Set Height 700
Set Padding 8
Set Framerate 18
Set TypingSpeed 35ms

Type "./bin/herald --demo"
Enter
Sleep 3s
Enter
Sleep 0.5s

# Open the deterministic ICS invitation fixture.
Right
Sleep 0.2s
Down 11
Sleep 0.4s
Enter
Sleep 1s

# Launch Create Calendar Event and choose the calendar with an existing UID.
Type "i"
Sleep 1s
Down 3
Sleep 0.3s
Enter
Sleep 1.2s

# Check duplicate UID handling, then skip without mutating demo state further.
Type "s"
Sleep 0.5s
