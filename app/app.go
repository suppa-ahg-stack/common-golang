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

// GlobalFragment describes a fragment that is present on every page.
type GlobalFragment struct {
	ID       FragmentID
	Selector string
}

// EmailSender is the small surface the generic application needs from an email
// delivery implementation. The concrete *serverutil.EmailSender satisfies it,
// and projects can provide mocks for tests or development.
type EmailSender interface {
	SendResetPassword(email, resetLink string) error
	SendOtpCode(to, code string, ttlSeconds int) error
	SendPasswordSetup(to, setupLink string, ttlHours int) error
	IsConfigured() bool
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
	EmailSender     EmailSender
	Cookies         struct {
		LangCookie *http.Cookie
	}
	Routes          map[string]serverutil.PageRoute
	PageComponent   map[string]map[string]RenderableComponent
	FragmentPlanner *FragmentPlanner
	GlobalFragments []GlobalFragment

	// GetUserID extracts the application-level user identifier from a request.
	// If nil or empty, the session cookie value is used as the routing key.
	GetUserID func(r *http.Request) string

	// GetActiveSessionIDsForUser returns the active session IDs for a user.
	// The map keys are session IDs; values are ignored.
	GetActiveSessionIDsForUser func(userID string) map[string]bool
}
