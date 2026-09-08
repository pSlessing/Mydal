package httpx

import (
	"encoding/json"
	"net/http"
)

// RespondWithJSON writes payload as JSON with the given status. A payload
// that fails to marshal (in practice, only a bug - every response type here
// is a plain struct or map of marshalable fields) still answers with the
// JSON error contract every other response uses, rather than the plain-text
// body http.Error would write.
func RespondWithJSON(w http.ResponseWriter, status int, payload any) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"internal_error","error":"internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}

// RespondWithError writes a JSON error body carrying a stable code beside the
// message, so a client can branch on the code without parsing prose. Prefer
// WriteError, which picks the status and code from the error itself.
func RespondWithError(w http.ResponseWriter, status int, code, message string) {
	RespondWithJSON(w, status, map[string]string{"code": code, "error": message})
}

// RespondNoContent writes a 204, which by definition carries no body.
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
