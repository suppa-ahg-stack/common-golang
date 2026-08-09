package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

func TestHandlerCallbacksReceiveBrokerConnectionID(t *testing.T) {
	broker := NewBroker(4)
	events := &SseEvents{Events: []EventHandler{&SseEventOpts{Broker: broker, Name: "test"}}}
	connected := make(chan uint64, 1)
	disconnected := make(chan uint64, 1)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/sse-events", nil).WithContext(ctx)
	request.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})
	done := make(chan struct{})
	go func() {
		Handler(events, "session", HandlerOptions{
			OnConnect:    func(_ *http.Request, id uint64) { connected <- id },
			OnDisconnect: func(_ *http.Request, id uint64) { disconnected <- id },
		}, &logger.FileLogger{})(httptest.NewRecorder(), request)
		close(done)
	}()

	var connectionID uint64
	select {
	case connectionID = <-connected:
	case <-time.After(time.Second):
		t.Fatal("connect callback was not called")
	}
	if connectionID == 0 {
		t.Fatal("connect callback received an empty connection id")
	}
	cancel()
	select {
	case disconnectedID := <-disconnected:
		if disconnectedID != connectionID {
			t.Fatalf("disconnect id = %d, want %d", disconnectedID, connectionID)
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect callback was not called")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}
}
