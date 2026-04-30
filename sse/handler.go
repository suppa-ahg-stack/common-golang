package sse

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type SseEventOpts struct {
	HeartbeatInterval   time.Duration
	OnConnectHandler    func(*http.Request)
	OnDisconnectHandler func(*http.Request)
	Event               *Event
	Broker              *Broker
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
func Handler(sseEvents *SseEvents) http.HandlerFunc {
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
		w.Header().Set("X-Accel-Buffering", "no")

		// Fan-in channel: all event streams converge here.
		merged := make(chan Event)

		// Single connection-level heartbeat.
		var heartbeat <-chan time.Time
		var ticker *time.Ticker

		// Use the smallest configured heartbeat interval.
		var minHeartbeat time.Duration

		for _, sseEvent := range sseEvents.Events {
			_, events, cleanup := sseEvent.GetBroker().subscribe(r.Context())

			sseEvent.OnConnect(r)

			defer func(event EventHandler, cleanupFn context.CancelFunc) {
				cleanupFn()
				event.OnDisconnect(r)
			}(sseEvent, cleanup)

			interval := sseEvent.GetHeartbeatInterval()
			if interval > 0 && (minHeartbeat == 0 || interval < minHeartbeat) {
				minHeartbeat = interval
			}

			// One goroutine per broker: reads only, never writes to ResponseWriter.
			go func(events <-chan Event) {
				for {
					select {
					case <-r.Context().Done():
						return

					case e, ok := <-events:
						if !ok {
							return
						}

						select {
						case merged <- e:
						case <-r.Context().Done():
							return
						}
					}
				}
			}(events)
		}

		if minHeartbeat > 0 {
			ticker = time.NewTicker(minHeartbeat)
			defer ticker.Stop()
			heartbeat = ticker.C
		}

		// Single writer loop.
		for {
			select {
			case <-r.Context().Done():
				return

			case <-heartbeat:
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()

			case e := <-merged:
				if err := writeEvent(w, e); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// writeEvent serialises a typed Event to the SSE wire format.
func writeEvent(w http.ResponseWriter, e Event) error {
	if e.ID != "" {
		fmt.Fprintf(w, "id: %s\n", e.ID)
	}
	if e.Type != "" {
		fmt.Fprintf(w, "event: %s\n", e.Type)
	}
	if e.Retry > 0 {
		fmt.Fprintf(w, "retry: %d\n", e.Retry)
	}

	fmt.Fprintf(w, "data: %s\n\n", e.Data)
	return nil
}
