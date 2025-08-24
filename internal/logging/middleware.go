package logging

import (
	"net/http"
	"time"
)

// HTTPMiddleware adds correlation ID and request logging to HTTP handlers
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate or extract correlation ID
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = NewCorrelationID()
		}

		// Add correlation ID to context
		requestCtx := r.Context()
		ctx := WithCorrelationID(requestCtx, correlationID)
		r = r.WithContext(ctx)

		// Add correlation ID to response headers
		w.Header().Set("X-Correlation-ID", correlationID)

		// Log request start
		start := time.Now()
		Info(ctx, ComponentHTTP, ActionRequest, "HTTP request started", map[string]interface{}{
			"method":     r.Method,
			"path":       r.URL.Path,
			"query":      r.URL.RawQuery,
			"remote_ip":  r.RemoteAddr,
			"user_agent": r.Header.Get("User-Agent"),
		})

		// Wrap response writer to capture status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: 200}

		// Call next handler
		next.ServeHTTP(wrapper, r)

		// Log request completion
		duration := time.Since(start)
		level := INFO
		if wrapper.statusCode >= 500 {
			level = ERROR
		} else if wrapper.statusCode >= 400 {
			level = WARN
		}

		if logger := GetGlobalLogger(); logger != nil {
			logger.WithDuration(ctx, level, ComponentHTTP, ActionResponse, "HTTP request completed", duration, map[string]interface{}{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": wrapper.statusCode,
				"bytes_sent":  wrapper.bytesWritten,
			})
		}
	})
}

// responseWrapper wraps http.ResponseWriter to capture status code and bytes written
type responseWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWrapper) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// CorrelationIDMiddleware is a simpler middleware that only adds correlation IDs
func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = NewCorrelationID()
		}

		var requestCtx = r.Context()
		ctx := WithCorrelationID(requestCtx, correlationID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Correlation-ID", correlationID)

		next.ServeHTTP(w, r)
	})
}
