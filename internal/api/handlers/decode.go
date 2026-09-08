package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mydal/internal/domain"
	"net/http"
)

// maxJSONBodyBytes bounds a JSON request body. Every create endpoint takes a
// handful of fields; without a cap, a slow or hostile client can hold a
// connection open indefinitely feeding it bytes.
const maxJSONBodyBytes = 1 << 20 // 1 MiB

// decodeJSON reads exactly one JSON value into dst: capped in size, strict
// about fields it does not recognise, and rejecting anything after that
// value. Every create endpoint decodes its body this way instead of a bare
// json.NewDecoder, so an oversized, malformed, or trailing-garbage body is
// always the same 400 rather than however encoding/json happens to react to
// each on its own.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: malformed JSON body", domain.ErrInvalidInput)
	}
	// A second Decode call reads the next token; anything but EOF means the
	// body held more than the one value it is allowed to.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request body must contain a single JSON value", domain.ErrInvalidInput)
	}
	return nil
}
