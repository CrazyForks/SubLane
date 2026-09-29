package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/murongg/SubLane/internal/gateway"
	"github.com/murongg/SubLane/internal/logging"
)

type requestLogKey struct{}
type requestLogState struct{ upgraded bool }

func requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Workspace routers nest; only the outer request owns the writer and summary.
		if r.Context().Value(requestLogKey{}) != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := gateway.WithRequestIdentity(r.Context(), 0, "http")
		state := &requestLogState{}
		ctx = context.WithValue(ctx, requestLogKey{}, state)
		logger := logging.From(ctx)
		ctx = logging.WithLogger(ctx, logger)
		r = r.WithContext(ctx)
		id := gateway.RequestID(ctx)
		w.Header().Set("X-Request-ID", id)
		// chi preserves optional writer interfaces and Unwrap for streaming and deadlines.
		writer := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		started := time.Now()
		defer func() {
			failure := recover()
			route := chi.RouteContext(ctx).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			status := writer.Status()
			if state.upgraded {
				status = http.StatusSwitchingProtocols
			} else if status == 0 && failure == nil {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			case route == "/healthz" || route == "/readyz" || route == "/*" || route == "/":
				level = slog.LevelDebug
			}
			attrs := []slog.Attr{
				slog.String("request_id", id), slog.String("method", logMethod(r.Method)),
				slog.String("route", route), slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()), slog.Int("bytes", writer.BytesWritten()),
			}
			if failure != nil {
				if err, ok := failure.(error); ok && err == http.ErrAbortHandler {
					level = slog.LevelWarn
					attrs = append(attrs, slog.String("error_code", "response_aborted"))
				} else {
					level = slog.LevelError
					attrs = append(attrs, slog.String("error_code", "handler_panic"), slog.String("stack", panicStack()))
				}
			}
			logger.LogAttrs(ctx, level, "HTTP request completed", attrs...)
			if failure != nil {
				// Preserve net/http's abort semantics, including after SSE or a hijack.
				// ErrAbortHandler prevents its fallback logger from exposing the panic value.
				panic(http.ErrAbortHandler)
			}
		}()
		next.ServeHTTP(logResponse(writer), r)
	})
}

type flushLogWriter struct{ middleware.WrapResponseWriter }

func (w *flushLogWriter) FlushError() error {
	if w.Status() == 0 {
		w.WriteHeader(http.StatusOK)
	}
	// chi's Flush method drops FlushError; forward through the controller so
	// disconnects remain visible, while recording headers committed by a flush.
	return http.NewResponseController(w.Unwrap()).Flush()
}

func (w *flushLogWriter) Flush() { _ = w.FlushError() }

type flushHijackLogWriter struct {
	*flushLogWriter
	http.Hijacker
}

type fancyLogWriter struct {
	*flushHijackLogWriter
	io.ReaderFrom
}

type pushLogWriter struct {
	*flushLogWriter
	http.Pusher
}

func logResponse(writer middleware.WrapResponseWriter) http.ResponseWriter {
	if _, ok := writer.(http.Flusher); !ok {
		return writer
	}
	flusher := &flushLogWriter{writer}
	if hijacker, ok := writer.(http.Hijacker); ok {
		combined := &flushHijackLogWriter{flusher, hijacker}
		if reader, ok := writer.(io.ReaderFrom); ok {
			return &fancyLogWriter{combined, reader}
		}
		return combined
	}
	if pusher, ok := writer.(http.Pusher); ok {
		return &pushLogWriter{flusher, pusher}
	}
	return flusher
}

func logMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

func panicStack() string {
	// Frames identify code locations without debug.Stack's argument values or panic contents.
	var pcs [32]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs[:])])
	var stack strings.Builder
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&stack, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			return stack.String()
		}
	}
}
