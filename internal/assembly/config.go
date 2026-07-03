package assembly

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/common/hlog"

	applog "go-app-template/internal/common/log"
)

// bcryptCost picks the per-hash work factor. 12 ≈ 250 ms on a modern
// x86 core in 2026 — slow enough to make offline cracking painful, fast
// enough to keep sign-up/sign-in latency under typical request budgets.
const bcryptCost = 12

// Config holds every environment-derived setting. Downstream providers
// read from this struct instead of touching os.Getenv directly, which
// keeps env-var coupling at the assembly boundary.
type Config struct {
	ServerHost string
	ServerPort string

	// LogFormat is "json" (nested objects, for deploy/prod) or "text"
	// (logfmt, nested groups flattened to dotted keys like req.method, for
	// local dev). LogOutput is a comma-separated set of destinations drawn
	// from {console, file}; both may be on at once.
	LogFormat string
	LogLevel  string
	LogOutput string

	// LogFile* configure the rolling file destination (used only when
	// LogOutput includes "file"). They map onto lumberjack's knobs.
	LogFilePath       string
	LogFileMaxSizeMB  int
	LogFileMaxBackups int
	LogFileMaxAgeDays int
	LogFileCompress   bool

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	BcryptCost int
}

// LoadConfig first applies the (best-effort) .env file at path — already
// set env vars win, so a container's injected vars override anything
// left in a committed .env — then materializes a Config from os.Getenv.
// Defaults match the values main.go used pre-DI so behaviour is unchanged.
func LoadConfig(dotenvPath string) Config {
	loadDotenv(dotenvPath)
	return Config{
		ServerHost: envOr("SERVER__HOST", "0.0.0.0"),
		ServerPort: envOr("SERVER__PORT", "8080"),
		LogFormat:  envOr("LOG__FORMAT", "json"),
		LogLevel:   envOr("LOG__LEVEL", "info"),
		LogOutput:  envOr("LOG__OUTPUT", "console"),

		LogFilePath:       envOr("LOG__FILE_PATH", "logs/app.log"),
		LogFileMaxSizeMB:  envOrInt("LOG__FILE_MAX_SIZE_MB", 100),
		LogFileMaxBackups: envOrInt("LOG__FILE_MAX_BACKUPS", 7),
		LogFileMaxAgeDays: envOrInt("LOG__FILE_MAX_AGE_DAYS", 30),
		LogFileCompress:   envOrBool("LOG__FILE_COMPRESS", false),

		DBHost:     envOr("DATABASE__HOST", "localhost"),
		DBPort:     envOr("DATABASE__PORT", "5432"),
		DBUser:     os.Getenv("DATABASE__USER"),
		DBPassword: os.Getenv("DATABASE__PASSWORD"),
		DBName:     os.Getenv("DATABASE__NAME"),
		BcryptCost: bcryptCost,
	}
}

// Validate reports every required setting that is missing or malformed, as a
// single error. main calls it right after LoadConfig and exits non-zero on
// failure, so a misconfigured process fails fast with a clear message instead
// of booting on defaults and erroring cryptically later. Concretely: an empty
// DATABASE__NAME otherwise falls back to the connection user as the database
// name, surfacing only as `database "<user>" does not exist` on the first query
// — this turns that into a startup-time `DATABASE__NAME is required`.
//
// DATABASE__PASSWORD is intentionally not required: trust / peer auth uses an
// empty password. Fields with a built-in default (host, log settings) can't be
// empty, so they aren't checked here.
func (c Config) Validate() error {
	var problems []string
	if c.DBUser == "" {
		problems = append(problems, "DATABASE__USER is required")
	}
	if c.DBName == "" {
		problems = append(problems, "DATABASE__NAME is required")
	}
	if _, err := strconv.Atoi(c.ServerPort); err != nil {
		problems = append(problems, fmt.Sprintf("SERVER__PORT must be numeric, got %q", c.ServerPort))
	}
	if _, err := strconv.Atoi(c.DBPort); err != nil {
		problems = append(problems, fmt.Sprintf("DATABASE__PORT must be numeric, got %q", c.DBPort))
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// Addr is the host:port the HTTP server listens on.
func (c Config) Addr() string { return net.JoinHostPort(c.ServerHost, c.ServerPort) }

// DSN composes a libpq URL from the DB* fields. net/url handles
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
func (c Config) DSN() string {
	q := url.Values{}
	q.Set("sslmode", "disable")
	q.Set("options", "-c timezone=UTC")
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     net.JoinHostPort(c.DBHost, c.DBPort),
		Path:     "/" + c.DBName,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// InitLogger installs slog.Default and bridges Hertz's hlog onto the very
// same *slog.Logger, so every line the process emits — ours or Hertz's
// framework lines — shares one format (text/json), one level, and one set
// of destinations. The bridge (applog.NewHlogBridge) replaces
// hertz-contrib/logger/slog, which hardcodes JSON and would otherwise emit
// JSON even under LOG__FORMAT=text. Called by main before BuildContainer so
// any provider that logs on startup uses the final logger configuration.
//
// It returns a close func that releases the file sink (a no-op when only
// console is configured); main defers it so rotated files are closed
// cleanly on shutdown. An error here (e.g. an unwritable log directory)
// is fatal — the caller should not proceed with a half-configured logger.
func InitLogger(cfg Config) (func() error, error) {
	w, closeLog, err := applog.NewWriter(cfg.logOutput())
	if err != nil {
		return nil, err
	}
	logger, opts := applog.Init(w, cfg.LogFormat, cfg.LogLevel)
	hlog.SetLogger(applog.NewHlogBridge(logger, opts))
	return closeLog, nil
}

// logOutput translates the comma-separated LogOutput setting into an
// applog.Output. Unknown tokens are ignored; an empty/all-unknown setting
// yields a zero Output, which applog.NewWriter degrades to stdout so the
// process is never silently logless.
func (c Config) logOutput() applog.Output {
	var out applog.Output
	for tok := range strings.SplitSeq(c.LogOutput, ",") {
		switch strings.ToLower(strings.TrimSpace(tok)) {
		case "console", "stdout":
			out.Console = true
		case "file":
			out.File = &applog.FileSink{
				Path:       c.LogFilePath,
				MaxSizeMB:  c.LogFileMaxSizeMB,
				MaxBackups: c.LogFileMaxBackups,
				MaxAgeDays: c.LogFileMaxAgeDays,
				Compress:   c.LogFileCompress,
			}
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envOrBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// loadDotenv parses KEY=VALUE lines and exports them as env vars, skipping
// blanks and # comments. It is intentionally minimal — same shape as the
// db_migrations shell scripts — and already-set env vars win, so a
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
