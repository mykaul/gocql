package replayer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gocql/gocql/dialer"
)

// optionsFrame builds a body-less OPTIONS frame with the given version byte.
func optionsFrame(version byte) []byte {
	return []byte{version, 0x00, 0x00, 0x01, 0x05, 0x00, 0x00, 0x00, 0x00}
}

// TestConnectionReplayerRejectsProtoV5 pins the rejection path dkropachev asked
// for: past the handshake a v5+ connection carries transport segments, which
// the replayer can neither hash for matching nor patch stream ids into without
// breaking the segment CRCs. The handshake frames are never segmented, so the
// version byte of the first write is genuine and the connection fails there —
// with an explicit error, not the unmatched-response panic.
func TestConnectionReplayerRejectsProtoV5(t *testing.T) {
	c := &ConnectionReplayer{gotRequest: make(chan struct{}, 1)}

	n, err := c.Write(optionsFrame(0x05))
	if !errors.Is(err, dialer.ErrProtoV5NotSupported) {
		t.Fatalf("Write(v5 frame) error = %v, want ErrProtoV5NotSupported", err)
	}
	if n != 0 {
		t.Errorf("Write(v5 frame) reported %d bytes written, want 0", n)
	}
}

// TestConnectionReplayerAcceptsProtoV4 pins that the rejection is scoped to
// v5+: a v4 frame with a matching recorded hash replays normally.
func TestConnectionReplayerAcceptsProtoV4(t *testing.T) {
	req := optionsFrame(0x04)
	c := &ConnectionReplayer{
		frames:     []*FrameRecorded{{Response: optionsFrame(0x84), Hash: dialer.GetFrameHash(req, false)}},
		gotRequest: make(chan struct{}, 1),
	}

	n, err := c.Write(req)
	if err != nil {
		t.Fatalf("Write(v4 frame) error = %v, want nil", err)
	}
	if n != len(req) {
		t.Errorf("Write(v4 frame) = %d bytes, want %d", n, len(req))
	}
}

// writeRecords writes the given records to a file in the recorder's format, one
// JSON object per line, and returns its path.
func writeRecords(t *testing.T, name string, records ...dialer.Record) string {
	t.Helper()

	var buf bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshalling record %d: %v", record.StreamID, err)
		}
		buf.Write(append(line, '\n'))
	}

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// TestLoadFramesFromFileLargeRecord pins that a record is bounded by the frame it
// holds and nothing else. The recorder writes one record per frame, and Record.Data
// is a []byte that encoding/json base64-inflates by 4/3, so a frame over ~48 KiB
// produces a line past bufio.MaxScanTokenSize — which is what the loader used to
// read records with, and it failed the whole recording with "token too long".
func TestLoadFramesFromFileLargeRecord(t *testing.T) {
	big := dialer.Record{StreamID: 1, Data: bytes.Repeat([]byte{0xAB}, 128*1024)}
	small := dialer.Record{StreamID: 2, Data: optionsFrame(0x04), UseMetadataID: true}

	line, err := json.Marshal(big)
	if err != nil {
		t.Fatalf("marshalling the large record: %v", err)
	}
	if len(line) <= bufio.MaxScanTokenSize {
		t.Fatalf("the large record encodes to %d bytes, which does not exceed the old %d-byte cap", len(line), bufio.MaxScanTokenSize)
	}

	records, err := loadFramesFromFile(writeRecords(t, "Reads", big, small))
	if err != nil {
		t.Fatalf("loadFramesFromFile() error = %v, want nil", err)
	}
	if len(records) != 2 {
		t.Fatalf("loaded %d records, want 2", len(records))
	}
	if !bytes.Equal(records[1].Data, big.Data) {
		t.Errorf("record 1 holds %d bytes, want the %d recorded", len(records[1].Data), len(big.Data))
	}
	// The record that follows a large one must still be read, so a big frame cannot
	// silently truncate a recording.
	if !bytes.Equal(records[2].Data, small.Data) || !records[2].UseMetadataID {
		t.Errorf("record 2 = %+v, want %+v", records[2], small)
	}
}

// TestLoadFramesFromFileSkipsDamagedRecord pins the behaviour the read loop keeps: a
// recording is a debugging artefact, so a record that does not decode is skipped and
// the frames around it still load. The last record deliberately has no trailing
// newline, which a truncated recording also looks like.
func TestLoadFramesFromFileSkipsDamagedRecord(t *testing.T) {
	first := dialer.Record{StreamID: 1, Data: optionsFrame(0x04)}
	last := dialer.Record{StreamID: 3, Data: optionsFrame(0x04)}

	firstLine, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshalling the first record: %v", err)
	}
	lastLine, err := json.Marshal(last)
	if err != nil {
		t.Fatalf("marshalling the last record: %v", err)
	}

	path := filepath.Join(t.TempDir(), "Reads")
	content := append(append(firstLine, "\n{\"data\":\"not json\n"...), lastLine...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	records, err := loadFramesFromFile(path)
	if err != nil {
		t.Fatalf("loadFramesFromFile() error = %v, want nil", err)
	}
	if len(records) != 2 {
		t.Fatalf("loaded %d records, want 2 (the damaged one skipped)", len(records))
	}
	if !bytes.Equal(records[1].Data, first.Data) {
		t.Errorf("record 1 = %+v, want %+v", records[1], first)
	}
	if !bytes.Equal(records[3].Data, last.Data) {
		t.Errorf("record 3 = %+v, want %+v, so a record without a trailing newline is still read", records[3], last)
	}
}
