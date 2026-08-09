package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

// HandlerOptions configures the SSE handler behaviour.
// SseEventOpts is a concrete EventHandler implementation.
type SseEventOpts struct {
	HeartbeatInterval   time.Duration
	OnConnectHandler    func(*http.Request)
	OnDisconnectHandler func(*http.Request)
	Event               *Event
	Broker              *Broker
	Name                string
}

type HandlerOptions struct {
	// HeartbeatInterval sends a comment ping to keep connections alive.
	// Zero disables it.
	HeartbeatInterval time.Duration

	// OnConnect is called once a client successfully subscribes.
	OnConnect func(r *http.Request, connectionID uint64)

	// OnDisconnect is called when a client disconnects (or context is cancelled).
	OnDisconnect func(r *http.Request, connectionID uint64)

	// UserIDExtractor returns the application-level user identifier to use for
	// routing events. If nil or empty, the session cookie value is used.
	UserIDExtractor func(r *http.Request) string

	// PageResolver returns the selectors and fragment IDs present on the given page.
	// If nil, the connection starts with no known page context.
	PageResolver func(page string) (selectors, fragmentIDs map[string]bool)
}

// Handler returns an http.HandlerFunc that streams typed SSE events.
func Handler(sseEvents *SseEvents, sessionName string, opts HandlerOptions, logger *logger.FileLogger) http.HandlerFunc {
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

		cookie, err := r.Cookie(sessionName)

		if errors.Is(err, http.ErrNoCookie) {
			logger.Error("No session cookie found during sse handler execution")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		sessionID := cookie.Value
		userID := sessionID
		if opts.UserIDExtractor != nil {
			if extracted := opts.UserIDExtractor(r); extracted != "" {
				userID = extracted
			}
		}

		var connID uint64
		var cleanup context.CancelFunc

		for i, sseEvent := range sseEvents.Events {
			var events <-chan Event
			broker := sseEvent.GetBroker()
			if i == 0 {
				connID, events, cleanup = broker.SubscribeWithIDs(r.Context(), sessionID, userID)
			} else {
				// All event streams for a single HTTP connection share the same
				// connection identifier, so page-context updates and per-connection
				// publishing always target the same client regardless of which broker
				// owns a given event type.
				events, cleanup = broker.SubscribeWithConnectionID(r.Context(), sessionID, userID, connID)
			}

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
		if opts.OnConnect != nil {
			opts.OnConnect(r, connID)
		}
		if opts.OnDisconnect != nil {
			defer opts.OnDisconnect(r, connID)
		}

		primaryBroker := sseEvents.Events[0].GetBroker()

		// Set initial page context from Referer on every registered broker so that
		// the broker used for DOM updates (e.g. app.DomUpdateBroker) has the same
		// routing metadata as the primary broker.
		if opts.PageResolver != nil {
			page := pageFromReferer(r)
			if page != "" {
				selectors, fragmentIDs := opts.PageResolver(page)
				for _, sseEvent := range sseEvents.Events {
					sseEvent.GetBroker().UpdateConnectionPage(connID, page, selectors, fragmentIDs)
				}
			}
		}

		// Send connection ID to the client so it can identify itself in /unh and /uih.
		connPayload, err := json.Marshal(map[string]any{
			"kind": "data",
			"name": "system.connection_id",
			"data": map[string]any{
				"connection_id": strconv.FormatUint(connID, 10),
			},
		})
		if err == nil {
			primaryBroker.PublishToConnection(connID, Event{
				Type: "app-event",
				Data: connPayload,
			})
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

func pageFromReferer(r *http.Request) string {
	referer := r.Header.Get("Referer")
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	return u.Path
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
