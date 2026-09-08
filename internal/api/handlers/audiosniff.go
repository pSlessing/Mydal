package handlers

import "bytes"

// sniffLen is how much of the body is inspected. It matches
// http.DetectContentType's window, which is the fallback below.
const sniffLen = 512

// audioFormat is what the upload endpoint needs to know about a file: the type
// to store it as, and the extension its object key should carry.
type audioFormat struct {
	contentType string
	ext         string
}

// sniffAudio identifies an audio container from its leading bytes.
//
// The client's Content-Type header is not trusted: it decides both the stored
// content type and the object key, so a wrong or hostile header renames the
// object and mislabels what the streaming endpoint later serves.
//
// http.DetectContentType is not enough on its own here - it has no signature
// for FLAC, and reports OGG as application/ogg and WAV as audio/wave - so the
// containers a music library actually holds are matched explicitly.
func sniffAudio(head []byte) (audioFormat, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("fLaC")):
		return audioFormat{"audio/flac", ".flac"}, true

	case bytes.HasPrefix(head, []byte("ID3")):
		return audioFormat{"audio/mpeg", ".mp3"}, true

	case bytes.HasPrefix(head, []byte("OggS")):
		return audioFormat{"audio/ogg", ".ogg"}, true

	case len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WAVE")):
		return audioFormat{"audio/wav", ".wav"}, true

	case len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")):
		return audioFormat{"audio/mp4", ".m4a"}, true
	}

	// A bare MPEG-family frame: eleven set sync bits, then the layer field
	// separates an MP3 frame from an ADTS-framed AAC one, which reserves it.
	if len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0 {
		if (head[1]>>1)&0x03 == 0 {
			return audioFormat{"audio/aac", ".aac"}, true
		}
		return audioFormat{"audio/mpeg", ".mp3"}, true
	}

	return audioFormat{}, false
}
