package stream

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type messageJSON struct {
	Reply string `json:"reply"`
	// Keys is present, possibly empty, only when the frame replaced the set.
	Keys *[]string `json:"keys,omitempty"`
}

// MessageHandler serves POST /message, through which an external hub holding
// a connection has gnotifd answer that connection's client frames.
func MessageHandler(p Protocol) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /message", func(w http.ResponseWriter, r *http.Request) {
		frame, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxFrame))
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "unreadable body")
			return
		}
		reply := p.Handle(r.Context(), frame)
		answer := messageJSON{Reply: string(reply.Frame)}
		if reply.Replaced {
			answer.Keys = &reply.Keys
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(answer)
	})
	return mux
}
