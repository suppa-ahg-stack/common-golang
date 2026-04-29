package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SseEvent[T any] struct {
	HeartbeatInterval   time.Duration
	OnConnectHandler    func(*http.Request)
	OnDisconnectHandler func(*http.Request)
	Data                T
	Broker              *Broker[T]
	Name                string
}

// HandlerOptions configures the SSE handler behaviour.
type HandlerOptions struct {
	// HeartbeatInterval sends a comment ping to keep connections alive.
	// Zero disables it.
	HeartbeatInterval time.Duration

	// OnConnect is called when a client successfully subscribes.
	OnConnect func(r *http.Request)

	// OnDisconnect is called when a client disconnects (or context is cancelled).
	OnDisconnect func(r *http.Request)
}

// Handler returns an http.HandlerFunc that streams typed SSE events.
func Handler[T any](sseEvent *SseEvent[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// SSE requires a flushing ResponseWriter.
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

		_, events, cleanup := sseEvent.Broker.subscribe(r.Context())
		if sseEvent.OnConnectHandler != nil {
			sseEvent.OnConnectHandler(r)
		}
		defer func() {
			cleanup()
			if sseEvent.OnDisconnectHandler != nil {
				sseEvent.OnDisconnectHandler(r)
			}
		}()

		var heartbeat <-chan time.Time
		if sseEvent.HeartbeatInterval > 0 {
			t := time.NewTicker(sseEvent.HeartbeatInterval)
			defer t.Stop()
			heartbeat = t.C
		}

		for {
			select {
			case <-r.Context().Done():
				return

			case <-heartbeat:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()

			case e, ok := <-events:
				if !ok {
					return
				}
				if err := writeEvent(w, e); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// writeEvent serialises a typed Event to the SSE wire format.
func writeEvent[T any](w http.ResponseWriter, e Event[T]) error {
	if e.ID != "" {
		fmt.Fprintf(w, "id: %s\n", e.ID)
	}
	if e.Type != "" {
		fmt.Fprintf(w, "event: %s\n", e.Type)
	}
	if e.Retry > 0 {
		fmt.Fprintf(w, "retry: %d\n", e.Retry)
	}

	data, err := json.Marshal(e.Data)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	return nil
}
