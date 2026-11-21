package handlers

import "testing"

func TestTailBufferKeepsOnlyMostRecentBytes(t *testing.T) {
	buffer := &tailBuffer{limit: 5}
	if _, err := buffer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte("defghi")); err != nil {
		t.Fatal(err)
	}

	if got, want := buffer.String(), "efghi"; got != want {
		t.Fatalf("tailBuffer.String() = %q, want %q", got, want)
	}
}
