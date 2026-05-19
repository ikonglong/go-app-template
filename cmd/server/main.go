package main

import (
	"bufio"
	"database/sql"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	_ "github.com/lib/pq"

	"go-app-template/internal/adapter/in/rest"
	"go-app-template/internal/adapter/out/db"
	"go-app-template/internal/adapter/out/security"
	"go-app-template/internal/adapter/out/sys"
	"go-app-template/internal/application/account"
)

// bcryptCost picks the per-hash work factor. 12 ≈ 250 ms on a modern
// x86 core in 2026 — slow enough to make offline cracking painful, fast
// enough to keep sign-up/sign-in latency under typical request budgets.
const bcryptCost = 12

// main wires the HTTP server: load .env (best-effort), open the DB
// (lazily — no startup ping so /health/alive stays answerable even when
// Postgres is down), build hertz, register inbound adapters, then hand off
// to Spin which blocks until SIGINT/SIGTERM and performs a graceful
// shutdown. Configuration follows the project-wide DOMAIN__KEY env
// convention (mirrors DATABASE__HOST/PORT in .env and the shell scripts).
func main() {
	loadDotenv(".env")

	addr := envOr("SERVER__HOST", "0.0.0.0") + ":" + envOr("SERVER__PORT", "8080")

	sqlDB, err := sql.Open("postgres", buildDSN())
	if err != nil {
		hlog.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	// Wiring: outbound adapters → application use cases → inbound handlers.
	// Use the go-jet AccountRepo (the sqlcdb adapter is the parallel option
	// kept around for benchmarking / migration — pick one per binary).
	accountRepo := db.NewAccountRepo(sqlDB, db.NewAccountMapper())
	hasher := security.NewBcryptHasher(bcryptCost)
	clock := sys.NewRealClock()
	idGen := sys.NewULIDGen()

	signUp := account.NewSignUpService(accountRepo, hasher, clock, idGen)
	signIn := account.NewSignInService(accountRepo, hasher)

	h := server.Default(server.WithHostPorts(addr))

	rest.NewHealthCheckHandler(sqlDB.PingContext).Register(h)
	rest.NewAccountHandler(signUp).Register(h)
	rest.NewSessionHandler(signIn).Register(h)

	hlog.Infof("HTTP server listening on %s", addr)
	h.Spin()
}

// buildDSN composes a libpq URL from DATABASE__* env vars. net/url handles
// percent-encoding so passwords with @, /, ?, etc. round-trip correctly
// without forcing the operator to pre-encode values in .env.
//
// `options=-c timezone=UTC` pins the per-connection session timezone to
// UTC for every connection in the pool. Postgres stores TIMESTAMPTZ as
// UTC internally but renders it in the session timezone on return —
// without this, rows from a PG box configured for e.g. Asia/Shanghai
// would land in Go as time.Time with a +08:00 location, forcing every
// downstream layer to remember to call .UTC(). With this set, the rule
// becomes simply "everything is UTC end-to-end" — no per-call fixups.
func buildDSN() string {
	q := url.Values{}
	q.Set("sslmode", "disable")
	q.Set("options", "-c timezone=UTC")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("DATABASE__USER"), os.Getenv("DATABASE__PASSWORD")),
		Host:     net.JoinHostPort(envOr("DATABASE__HOST", "localhost"), envOr("DATABASE__PORT", "5432")),
		Path:     "/" + os.Getenv("DATABASE__NAME"),
		RawQuery: q.Encode(),
	}
	return u.String()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadDotenv parses KEY=VALUE lines and exports them as env vars, skipping
// blanks and # comments. It is intentionally minimal — same shape as the
// db-migrations shell scripts — and already-set env vars win, so a
// container's injected vars override anything left in a committed .env.
// Quoted values have a single layer of surrounding "/' stripped; anything
// more elaborate (escapes, multiline) is out of scope.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[0] == v[len(v)-1] {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
