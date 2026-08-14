package terminal

import (
	"bytes"
	"testing"
)

func TestPumpStopsWhenOutputHandlerRejectsData(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, 64*1024)
	reads := 0
	pump(&countingReader{reader: bytes.NewReader(payload), reads: &reads}, func(data []byte) bool {
		if len(data) == 0 {
			t.Fatal("handler received an empty output chunk")
		}
		return false
	})

	if reads != 1 {
		t.Fatalf("pump continued reading after rejection: got %d reads", reads)
	}
}

type countingReader struct {
	reader *bytes.Reader
	reads  *int
}

func (r *countingReader) Read(data []byte) (int, error) {
	(*r.reads)++
	return r.reader.Read(data)
}
