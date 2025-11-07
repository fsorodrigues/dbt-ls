package analysis

import (
	rope "github.com/zyedidia/generic/rope"
)

type Rope = rope.Node[rune]

func getOffset(r *rope.Node[rune], targetLine, targetChar int) int {
	offset := 0
	line := 0
	char := 0
	found := false

	r.Each(func(n *Rope) {
		if found {
			return
		}

		for _, ch := range n.Value() {
			if line == targetLine && char == targetChar {
				found = true
				return
			}
			if ch == '\n' {
				line++
				char = 0
			} else {
				char++
			}
			offset++
		}
	})

	return offset
}
