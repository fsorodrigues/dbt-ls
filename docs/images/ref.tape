Output docs/images/ref.gif

Require nvim

Set Width 1200
Set Height 600
Set Padding 5
Set Margin 80
Set MarginFill "#6B50FF"
Set FontSize 16
Set TypingSpeed 40ms

Type "nvim models/clients.sql"
Enter
Sleep 1s

Type "i"
Type `select client_id from {{ ref('pr`
Sleep 0.5s
Ctrl+j
Sleep 0.25s
Enter
Type `') }}`
Sleep 1s
