package web_server

import (
	"encoding/json"
	"net/http"
)

func marshalJSONPayload(payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	if w == nil {
		return
	}
	if status <= 0 {
		status = http.StatusOK
	}
	b, err := marshalJSONPayload(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeJSONBytes(w http.ResponseWriter, status int, payload []byte) {
	if w == nil {
		return
	}
	if status <= 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
