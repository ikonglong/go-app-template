package rest

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/ikonglong/go-apperror"
)

// healthReadyEvent names the readiness-probe operation. Pure operation
// name; the failure mode lives in the AppError's Code, never in the event
// string (error-handling guide §6.2). On a failed probe it is the event
// of the NewUnavailable error that renderError logs.
const healthReadyEvent = "health.ready"

const (
	serviceName    = "go-app-template"
	serviceVersion = "0.1.0"

	// readinessPingTimeout caps how long a single readiness probe waits on
	// the database. K8s readinessProbe.timeoutSeconds defaults to 1s, so
	// staying under it ensures the probe fails fast with our own diagnostic
	// rather than being killed mid-flight by the kubelet.
	readinessPingTimeout = 800 * time.Millisecond
)

// HealthCheckResponse is the body shared by every process-health endpoint.
// Checks is optional and stays nil on liveness responses — only readiness,
// which actually pings downstream dependencies, fills it in.
type HealthCheckResponse struct {
	Status    string            `json:"status"`
	Service   string            `json:"service"`
	Version   string            `json:"version"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// HealthCheckHandler exposes the process-health endpoints. Its routes are
// mounted at the root (no prefix) so load balancers and Kubernetes probes
// can reach them without path rewrites.
//
// dbPing is the readiness dependency. A function (rather than a full port
// interface) is deliberate: it's the minimum surface — *sql.DB.PingContext,
// pgxpool.Pool.Ping, or a test stub all satisfy it without forcing the
// handler to import a persistence package.
type HealthCheckHandler struct {
	dbPing func(context.Context) error
}

func NewHealthCheckHandler(dbPing func(context.Context) error) *HealthCheckHandler {
	return &HealthCheckHandler{dbPing: dbPing}
}

// Register wires the handler's routes onto the provided hertz router.
// Pass it the *server.Hertz instance (or any sub-group) at bootstrap time.
func (h *HealthCheckHandler) Register(r route.IRouter) {
	r.GET("/health/alive", h.Alive)
	r.GET("/health/ready", h.Ready)
}

// Alive answers the Kubernetes liveness probe: it only confirms that the
// process is up and the HTTP stack can serve a request. No external
// dependency is touched on purpose — readiness is the place for that, so
// a transient database hiccup never restarts the container.
func (h *HealthCheckHandler) Alive(_ context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, &HealthCheckResponse{
		Status:    "alive",
		Service:   serviceName,
		Version:   serviceVersion,
		Timestamp: nowISO(),
	})
}

// Ready answers the Kubernetes readiness probe: it pings the database and,
// on success, reports per-dependency status in Checks. A failed ping is
// rendered through renderError as a 503 UNAVAILABLE — the pod stops
// receiving traffic but is NOT restarted (readiness flaps like a brief DB
// failover are expected; the kubelet rides them out via the alive probe).
//
// Routing the failure through the shared errorResp path keeps the error log
// identical to every other endpoint (sanitized body, ErrGroup fields, db
// error preserved as cause) and costs nothing on the probe side: the
// kubelet inspects only the status code, never the body. The per-dependency
// diagnostic therefore lives in the log (cause), not in the response.
func (h *HealthCheckHandler) Ready(ctx context.Context, c *app.RequestContext) {
	pingCtx, cancel := context.WithTimeout(ctx, readinessPingTimeout)
	defer cancel()

	if err := h.dbPing(pingCtx); err != nil {
		renderError(ctx, c, apperror.NewUnavailable(healthReadyEvent,
			apperror.WithMessage("database not ready"),
			apperror.WithCause(err)))
		return
	}

	c.JSON(consts.StatusOK, &HealthCheckResponse{
		Status:    "ready",
		Service:   serviceName,
		Version:   serviceVersion,
		Timestamp: nowISO(),
		Checks:    map[string]string{"database": "healthy"},
	})
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }
