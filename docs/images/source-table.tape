Output docs/images/source-table.gif

Require nvim

Set Width 1200
Set Height 600
Set Padding 5
Set Margin 80
Set MarginFill "#6B50FF"
Set FontSize 16
Set TypingSpeed 40ms

Type "nvim models/prospects.sql"
Enter
Sleep 1s

Type "i"
Type `select * from {{ source('wa`
Sleep 0.5s
Ctrl+j
Enter
Type `', 'c`
Sleep 0.5s
Enter
Type `') }}`
Sleep 1s
