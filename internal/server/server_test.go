package server

import (
	"goinmetrics/internal/collector"
	"goinmetrics/internal/monitor"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestBrowserBoundary(t *testing.T) {
	handler := New(monitor.New(&collector.Collector{Proc: "/proc", Root: "/"}, nil, true))
	for _, tc := range []struct {
		name, host, origin, fetch string
		want                      int
	}{
		{"local", "127.0.0.1:9099", "", "", 200},
		{"ssh tunnel", "127.0.0.1:19099", "http://127.0.0.1:19099", "same-origin", 200},
		{"DNS rebinding", "attacker.example:9099", "", "", 403},
		{"other-local-origin", "127.0.0.1:9099", "http://localhost:5555", "", 403},
		{"cross-origin", "127.0.0.1:9099", "https://attacker.example", "", 403},
		{"cross-site fetch", "127.0.0.1:9099", "", "cross-site", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tc.host+"/api/snapshot", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.fetch)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
		})
	}
}
func TestRejectsInvalidHistoryBeforeDatabase(t *testing.T) {
	handler := New(monitor.New(&collector.Collector{}, nil, true))
	for _, query := range []string{"from=bad", "to=bad", "from=2000-01-01T00:00:00Z", "from=2099-01-01T00:00:00Z"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9099/api/history?"+query, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("query=%s status=%d", query, w.Code)
		}
	}
}

func TestGeneratedNuxtAssetsAreServed(t *testing.T) {
	handler := New(monitor.New(&collector.Collector{}, nil, true))
	indexReq := httptest.NewRequest("GET", "http://127.0.0.1:9099/", nil)
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, indexReq)
	refs := regexp.MustCompile(`(?:src|href)="(/_nuxt/[^" ]+)"`).FindAllStringSubmatch(index.Body.String(), -1)
	if len(refs) == 0 {
		t.Skip("generate Nuxt assets to run packaging regression check")
	}
	for _, ref := range refs {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9099"+ref[1], nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("Nuxt asset omitted from embedded filesystem: %s status=%d", ref[1], w.Code)
		}
		if strings.HasSuffix(ref[1], ".css") && !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
			t.Fatalf("stylesheet MIME mismatch: %s", w.Header().Get("Content-Type"))
		}
	}
}

func TestServiceControlRequiresExplicitJSONBoolean(t *testing.T) {
	handler := New(monitor.New(&collector.Collector{}, nil, true))
	for _, tc := range []struct {
		body, content string
		status        int
	}{
		{`{}`, "application/json", 400}, {`{"enabled":null}`, "application/json", 400},
		{`{"enabled":"false"}`, "application/json", 400}, {`{"enabled":false,"extra":1}`, "application/json", 400},
		{`{"enabled":false} {}`, "application/json", 400}, {`{"enabled":false}`, "text/plain", 415},
	} {
		r := httptest.NewRequest("PUT", "http://127.0.0.1:9099/api/service", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("body=%s status=%d", tc.body, w.Code)
		}
	}
}
func TestRemovedEventRoutes(t *testing.T) {
	handler := New(monitor.New(&collector.Collector{}, nil, true))
	for _, path := range []string{"events", "rules"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9099/api/"+path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("removed path=%s status=%d", path, w.Code)
		}
	}
}
