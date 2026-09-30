package analysis

import "testing"

func TestSnapshotPositionOffsetRoundTrip(t *testing.T) {
	snapshot := Snapshot{Text: []rune("alpha\nbéta\ngamma")}

	for offset := 0; offset <= len(snapshot.Text); offset++ {
		position := snapshot.Position(offset)
		if got := snapshot.Offset(position); got != offset {
			t.Fatalf("Offset(Position(%d)) = %d, want %d", offset, got, offset)
		}
	}
}

func TestSnapshotRange(t *testing.T) {
	snapshot := Snapshot{Text: []rune("one\ntwö")}
	got := snapshot.Range(2, 7)

	if got.Start.Line != 0 || got.Start.Character != 2 {
		t.Fatalf("Range start = %+v, want line 0 character 2", got.Start)
	}
	if got.End.Line != 1 || got.End.Character != 3 {
		t.Fatalf("Range end = %+v, want line 1 character 3", got.End)
	}
}
