package app

import (
	"net/http"
	"time"

	"suppa-ahg-stack/common-golang/logger"
	"suppa-ahg-stack/common-golang/serverutil"
	"suppa-ahg-stack/common-golang/sse"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PageComponent struct {
	ContentFunc    func() templ.Component
	TargetSelector string
	IsLayout       bool
}

type csrf struct {
	token     string
	expiresAt time.Time
}

type App[TConfig any, TQueries any, TSessionService any, TSseNames any] struct {
	Config          *TConfig
	Logger          *logger.FileLogger
	Db              *pgxpool.Pool
	Queries         *TQueries
	SseEvents       *sse.SseEvents
	SseNames        *TSseNames
	DomUpdateBroker *sse.Broker
	RateLimiter     *serverutil.RateLimiter
	SessionService  *TSessionService
	EmailSender     *serverutil.EmailSender
	Cookies         struct {
		LangCookie *http.Cookie
	}
	Routes        map[string]serverutil.PageRoute
	PageComponent map[string]map[string]PageComponent
}
