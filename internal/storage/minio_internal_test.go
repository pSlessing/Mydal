package storage

import (
	"io"
	"log/slog"
	"testing"
)

// The upload handler tests all cap uploads at 4 KiB, far too small to ever
// need more than one part, so none of them can observe whether Put actually
// bounds the part size for a chunked upload of real-world size. This tests
// the formula NewMinIOStore derives it with directly.
func TestPartSizeDerivedFromMaxUploadBytes(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tc := range []struct {
		name           string
		maxUploadBytes int64
		want           uint64
	}{
		{"tiny cap floors to the minimum", 4096, minPartSize},
		{"default 1 GiB cap still floors to the minimum", 1 << 30, minPartSize},
		{"a cap large enough to need it scales the part size up", 200 << 30, uint64(200<<30) / maxParts},
		{"zero falls back to the minimum rather than a zero part size", 0, minPartSize},
		{"negative falls back to the minimum", -1, minPartSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMinIOStore(nil, "bucket", tc.maxUploadBytes, quiet)
			if s.partSize != tc.want {
				t.Errorf("partSize = %d, want %d", s.partSize, tc.want)
			}
			// However large the cap, the resulting part count for the largest
			// allowed upload must stay under S3's multipart-upload ceiling.
			if tc.maxUploadBytes > 0 && uint64(tc.maxUploadBytes)/s.partSize > maxParts {
				t.Errorf("maxUploadBytes=%d needs more than %d parts at partSize=%d",
					tc.maxUploadBytes, maxParts, s.partSize)
			}
		})
	}
}
