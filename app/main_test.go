package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// --- healthHandler ---

func TestHealthHandler_Returns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestHealthHandler_ReturnsHealthyStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("want status=healthy, got %q", body["status"])
	}
}

// --- readyHandler ---

func TestReadyHandler_Returns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	readyHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestReadyHandler_ReturnsReadyStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	readyHandler(rr, req)

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if body["status"] != "ready" {
		t.Errorf("want status=ready, got %q", body["status"])
	}
}

// --- rootHandler ---

func TestRootHandler_ReturnsOkStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	rootHandler(rr, req)

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("want status=ok, got %q", body["status"])
	}
}

func TestRootHandler_UsesServiceNameEnvVar(t *testing.T) {
	os.Setenv("SERVICE_NAME", "my-test-service")
	defer os.Unsetenv("SERVICE_NAME")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	rootHandler(rr, req)

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if body["service"] != "my-test-service" {
		t.Errorf("want service=my-test-service, got %q", body["service"])
	}
}

func TestRootHandler_DefaultsToAppA(t *testing.T) {
	os.Unsetenv("SERVICE_NAME")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	rootHandler(rr, req)

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if body["service"] != "app-a" {
		t.Errorf("want default service=app-a, got %q", body["service"])
	}
}

func TestRootHandler_ReturnsJSONContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	rootHandler(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("want Content-Type=application/json, got %q", ct)
	}
}

// --- responseWriter ---

func TestResponseWriter_DefaultStatus200(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: 200}
	if rw.status != 200 {
		t.Errorf("want default status 200, got %d", rw.status)
	}
}

func TestResponseWriter_CapturesWriteHeader(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: 200}

	rw.WriteHeader(http.StatusCreated)

	if rw.status != http.StatusCreated {
		t.Errorf("want captured status %d, got %d", http.StatusCreated, rw.status)
	}
}

func TestResponseWriter_CapturesErrorStatus(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: 200}

	rw.WriteHeader(http.StatusInternalServerError)

	if rw.status != http.StatusInternalServerError {
		t.Errorf("want captured status %d, got %d", http.StatusInternalServerError, rw.status)
	}
}

// --- instrument middleware ---

func TestInstrument_CallsUnderlyingHandler(t *testing.T) {
	called := false
	handler := instrument("/test", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler(httptest.NewRecorder(), req)

	if !called {
		t.Error("expected underlying handler to be called")
	}
}

func TestInstrument_PassesThroughStatusCode(t *testing.T) {
	handler := instrument("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

	if rr.Code != http.StatusAccepted {
		t.Errorf("want 202, got %d", rr.Code)
	}
}

func TestInstrument_PassesThroughResponseBody(t *testing.T) {
	handler := instrument("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"key":"value"}`))
	})

	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/test", nil))

	if rr.Body.String() != `{"key":"value"}` {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

func TestInstrument_PropagatesContext(t *testing.T) {
	var receivedCtx interface{}
	handler := instrument("/ctx-test", func(w http.ResponseWriter, r *http.Request) {
		receivedCtx = r.Context()
	})

	req := httptest.NewRequest(http.MethodGet, "/ctx-test", nil)
	handler(httptest.NewRecorder(), req)

	if receivedCtx == nil {
		t.Error("expected a non-nil context to be passed to the handler")
	}
}
