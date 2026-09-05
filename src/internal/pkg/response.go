package pkg

import (
	"encoding/json"
	"net/http"
)

// RespondWithJSON writes payload as JSON with the given status.
func RespondWithJSON(w http.ResponseWriter, status int, payload any) {
	response, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}

// RespondWithError writes a JSON error body. Prefer WriteError, which picks
// the status from the error itself.
func RespondWithError(w http.ResponseWriter, status int, message string) {
	RespondWithJSON(w, status, map[string]string{"error": message})
}

// RespondWithPacket writes raw bytes, for responses that are not JSON.
func RespondWithPacket(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(status)
	w.Write(data)
}

// RespondNoContent writes a 204, which by definition carries no body.
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
