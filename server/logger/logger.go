package logger

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type contextKey struct{}

var DefaultLogger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(contextKey{}).(*slog.Logger)
	if !ok {
		return DefaultLogger
	}
	return l
}

func SetDefault(l *slog.Logger) {
	DefaultLogger = l
}

func InfoC(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Info(msg, args...)
}

func ErrorC(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Error(msg, args...)
}

// Middleware injects a slog.Logger into both gin.Context and context.Context
func Middleware(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Clone logger with request-scoped fields
		reqLogger := base.With(
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
		)

		// Inject into gin.Context and stdlib context
		ctx := WithLogger(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)

		// Store in gin.Keys too if needed (optional)
		c.Set("logger", reqLogger)

		c.Next()

		// Optionally log request duration
		reqLogger.Info("request complete",
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(start)),
		)
	}
}
