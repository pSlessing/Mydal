package logging

import (
	"bytes"
	"strings"
	"testing"
)

// An unrecognised LOG_LEVEL used to fall back to info silently - the only way
// to learn it was wrong was a support ticket about missing debug logs.
func TestUnrecognisedLevelWarns(t *testing.T) {
	var buf bytes.Buffer
	newTo(&buf, "not-a-level")

	if !strings.Contains(buf.String(), "Unrecognised LOG_LEVEL") {
		t.Errorf("no warning for an unrecognised level: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "not-a-level") {
		t.Errorf("warning did not name the offending value: %s", buf.String())
	}
}

func TestKnownLevelDoesNotWarn(t *testing.T) {
	var buf bytes.Buffer
	newTo(&buf, "debug")

	if buf.Len() != 0 {
		t.Errorf("a valid LOG_LEVEL warned: %s", buf.String())
	}
}

// An empty level means "use the default", not a mistake.
func TestEmptyLevelDoesNotWarn(t *testing.T) {
	var buf bytes.Buffer
	newTo(&buf, "")

	if buf.Len() != 0 {
		t.Errorf("an empty LOG_LEVEL warned: %s", buf.String())
	}
}

// The level actually takes effect: debug-level logging is enabled after an
// unrecognised value falls back to info, not silently disabled altogether.
func TestFallbackLevelIsInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := newTo(&buf, "not-a-level")
	buf.Reset() // drop the warning emitted while building the logger

	logger.Info("should appear")
	logger.Debug("should not appear")

	if !strings.Contains(buf.String(), "should appear") {
		t.Error("info-level log missing after an unrecognised LOG_LEVEL")
	}
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("debug-level log present after an unrecognised LOG_LEVEL - fallback should be info")
	}
}
