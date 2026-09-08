package handlers

import "testing"

// The upload endpoint used to trust the client's Content-Type, which chose
// both the object key and the type the streaming endpoint later served.
func TestSniffAudio(t *testing.T) {
	pad := func(prefix string, n int) []byte {
		b := make([]byte, 0, n)
		b = append(b, prefix...)
		for len(b) < n {
			b = append(b, 0x00)
		}
		return b
	}
	wav := func() []byte {
		b := append([]byte("RIFF"), 0, 0, 0, 0)
		return append(b, []byte("WAVEfmt ")...)
	}
	mp4 := func() []byte {
		return append([]byte{0, 0, 0, 0x18}, []byte("ftypM4A ")...)
	}

	for _, tc := range []struct {
		name        string
		body        []byte
		wantType    string
		wantExt     string
		wantMatched bool
	}{
		{"flac", pad("fLaC", 32), "audio/flac", ".flac", true},
		{"mp3 with id3", pad("ID3\x04\x00", 32), "audio/mpeg", ".mp3", true},
		{"ogg", pad("OggS\x00", 32), "audio/ogg", ".ogg", true},
		{"wav", wav(), "audio/wav", ".wav", true},
		{"m4a", mp4(), "audio/mp4", ".m4a", true},
		// A bare MPEG frame: the layer bits separate MP3 from ADTS AAC.
		{"mp3 bare frame", []byte{0xFF, 0xFB, 0x90, 0x00}, "audio/mpeg", ".mp3", true},
		{"aac adts", []byte{0xFF, 0xF1, 0x50, 0x80}, "audio/aac", ".aac", true},
		{"aac adts mpeg2", []byte{0xFF, 0xF9, 0x50, 0x80}, "audio/aac", ".aac", true},

		{"plain text", []byte("this is not audio, it is prose"), "", "", false},
		{"empty", nil, "", "", false},
		{"too short to decide", []byte{0xFF}, "", "", false},
		{"riff but not wave", append([]byte("RIFF"), []byte("\x00\x00\x00\x00AVI ")...), "", "", false},
		// A JPEG must not be accepted just because a client says it is audio.
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sniffAudio(tc.body)
			if ok != tc.wantMatched {
				t.Fatalf("matched = %v, want %v (got %+v)", ok, tc.wantMatched, got)
			}
			if !ok {
				return
			}
			if got.contentType != tc.wantType || got.ext != tc.wantExt {
				t.Fatalf("got %s%s, want %s%s", got.contentType, got.ext, tc.wantType, tc.wantExt)
			}
		})
	}
}
