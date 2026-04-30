package sse

import (
	"net/http"
	"suppa-ahg-stack/common-golang/logger"
	"time"
)

type EventHandler interface {
	GetName() string
	GetBroker() *Broker
	GetHeartbeatInterval() time.Duration
	OnConnect(*http.Request)
	OnDisconnect(*http.Request)
}

type SseEvents struct {
	Logger *logger.FileLogger
	Events []EventHandler
}

type EventInitializer func() EventHandler

func (s *SseEventOpts) GetName() string {
	return s.Name
}

func (s *SseEventOpts) GetBroker() *Broker {
	return s.Broker
}

func (s *SseEventOpts) GetHeartbeatInterval() time.Duration {
	return s.HeartbeatInterval
}

func (s *SseEventOpts) OnConnect(r *http.Request) {
	if s.OnConnectHandler != nil {
		s.OnConnectHandler(r)
	}
}

func (s *SseEventOpts) GetEvent() *Event {
	return s.Event
}

func (s *SseEventOpts) OnDisconnect(r *http.Request) {
	if s.OnDisconnectHandler != nil {
		s.OnDisconnectHandler(r)
	}
}

func (s *SseEvents) InitSseEvents(initializers ...EventInitializer) {
	for _, initFn := range initializers {
		s.Events = append(s.Events, initFn())
	}
}
