package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

var requestCounter uint64

type requestLogKey struct{}

type requestLogMeta struct {
	cacheStatus string
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&requestCounter, 1))
		}
		w.Header().Set("X-Request-ID", requestID)

		sw := &statusWriter{ResponseWriter: w}
		meta := &requestLogMeta{}
		logRequest := r.WithContext(context.WithValue(r.Context(), requestLogKey{}, meta))
		next.ServeHTTP(sw, logRequest)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		route := logRequest.Pattern
		if route == "" {
			route = r.URL.Path
		}

		logger.Info("request",
			"request_id", requestID,
			"method", r.Method,
			"route", route,
			"path", r.URL.Path,
			"status", status,
			"bytes", sw.bytes,
			"duration_ms", time.Since(start).Milliseconds(),
			"user_agent_category", UserAgentCategory(r),
			"cache", meta.cacheStatus,
		)
	})
}

func setCacheStatus(ctx context.Context, status string) {
	meta, ok := ctx.Value(requestLogKey{}).(*requestLogMeta)
	if !ok || meta == nil {
		return
	}
	meta.cacheStatus = status
}
