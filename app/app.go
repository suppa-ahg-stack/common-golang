package app

import (
	"context"
	"io"
	"net/http"
	"time"

	"suppa-ahg-stack/common-golang/logger"
	"suppa-ahg-stack/common-golang/serverutil"
	"suppa-ahg-stack/common-golang/sse"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RenderableComponent interface {
	Render(r *http.Request, ctx context.Context, w io.Writer) error
	GetTargetSelector() string
	IsLayoutComponent() bool
}

type PageComponent[TProps any] struct {
	Props          TProps
	PropsBuilder   func(r *http.Request) TProps
	ContentFunc    func(props TProps) templ.Component
	TargetSelector string
	IsLayout       bool
}

func (pc PageComponent[TProps]) Render(r *http.Request, ctx context.Context, w io.Writer) error {
	props := pc.Props
	if pc.PropsBuilder != nil {
		props = pc.PropsBuilder(r)
	}
	return pc.ContentFunc(props).Render(ctx, w)
}

func (pc PageComponent[TProps]) GetTargetSelector() string {
	return pc.TargetSelector
}

func (pc PageComponent[TProps]) IsLayoutComponent() bool {
	return pc.IsLayout
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
	PageComponent map[string]map[string]RenderableComponent
}
