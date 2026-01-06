package analysis

import (
	rope "github.com/zyedidia/generic/rope"
)

type Rope = rope.Node[rune]

func getOffset(r *Rope, targetLine, targetChar int) int {
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

func getLine(r *Rope, targetLine int) string {
	offset := 0
	line := 0
	startOffset := -1
	endOffset := -1
	foundStart := false

	r.Each(func(n *Rope) {
		if endOffset != -1 {
			return
		}

		for _, ch := range n.Value() {
			if line == targetLine && !foundStart {
				startOffset = offset
				foundStart = true
			}
			if line == targetLine+1 {
				endOffset = offset
				return
			}
			if ch == '\n' {
				line++
			}
			offset++
		}
	})

	if startOffset == -1 {
		return ""
	}
	if endOffset == -1 {
		endOffset = offset
	}

	return string(r.Slice(startOffset, endOffset))
}
