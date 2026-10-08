package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry"
)

func TestRequestLogger_GenerateAndPropagateRequestID(t *testing.T) {
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, ok := c.Request.Context().Value(telemetry.RequestID).(string)
		if !ok || reqID == "" {
			t.Fatalf("request_id missing in context")
		}
		if got := c.Writer.Header().Get(requestIDHeader); got != reqID {
			t.Fatalf("response header request_id mismatch, header=%q ctx=%q", got, reqID)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if w.Header().Get(requestIDHeader) == "" {
		t.Fatalf("X-Request-ID should be set")
	}
}

func TestRequestLogger_KeepIncomingRequestID(t *testing.T) {
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, _ := c.Request.Context().Value(telemetry.RequestID).(string)
		if reqID != "rid-fixed" {
			t.Fatalf("request_id=%q, want rid-fixed", reqID)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(requestIDHeader, "rid-fixed")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get(requestIDHeader); got != "rid-fixed" {
		t.Fatalf("header=%q, want rid-fixed", got)
	}
}

func TestRequestLoggerBoundsIncomingRequestID(t *testing.T) {
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		reqID, _ := c.Request.Context().Value(telemetry.RequestID).(string)
		if len(reqID) != 36 {
			t.Fatalf("request_id length=%d", len(reqID))
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set(requestIDHeader, strings.Repeat("r", 1024))
	r.ServeHTTP(w, req)
	if got := len(w.Header().Get(requestIDHeader)); got != 36 {
		t.Fatalf("response request_id length=%d", got)
	}
}
