package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"goinmetrics/internal/monitor"
	"goinmetrics/internal/web"
)

type Server struct {
	Monitor *monitor.Monitor
	assets  fs.FS
}

func New(m *monitor.Monitor) http.Handler {
	assets, _ := fs.Sub(web.Public, "public")
	s := &Server{Monitor: m, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/host", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, m.Host()) })
	mux.HandleFunc("GET /api/snapshot", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, m.Latest()) })
	mux.HandleFunc("GET /api/stream", s.stream)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("PUT /api/service", s.service)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API-маршрут не найден") })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			fail(w, 405, "Метод не поддерживается")
			return
		}
		s.static(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if !localHost(r.Host) {
			fail(w, 403, "Разрешён только локальный адрес. Используйте SSH-туннель.")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || !localHost(u.Host) || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Host != r.Host {
				fail(w, 403, "Недопустимый источник запроса")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "Запрос с постороннего сайта запрещён")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}
func localHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}
func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, message string) {
	reply(w, code, map[string]string{"error": message})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	status := s.Monitor.Status()
	code := 200
	if !status.StorageOK {
		code = 503
	}
	reply(w, code, status)
}
func dbContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 5*time.Second)
}
func (s *Server) service(w http.ResponseWriter, r *http.Request) {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		fail(w, 415, "Ожидается application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if decoder.Decode(&body) != nil || body.Enabled == nil {
		fail(w, 400, "Укажите enabled: true или false")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		fail(w, 400, "Ожидается один JSON-объект")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err := s.Monitor.SetEnabled(ctx, *body.Enabled); err != nil {
		fail(w, 503, "Не удалось изменить состояние мониторинга. Проверьте подключение к БД.")
		return
	}
	reply(w, 200, s.Monitor.Status())
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	to := now
	from := to.Add(-15 * time.Minute)
	var err error
	if v := r.URL.Query().Get("to"); v != "" {
		to, err = time.Parse(time.RFC3339Nano, v)
		if err != nil {
			fail(w, 400, "Неверное время to")
			return
		}
	}
	if v := r.URL.Query().Get("from"); v != "" {
		from, err = time.Parse(time.RFC3339Nano, v)
		if err != nil {
			fail(w, 400, "Неверное время from")
			return
		}
	}
	if !from.Before(to) || to.Sub(from) > 24*time.Hour || from.Before(now.Add(-24*time.Hour-time.Minute)) || to.After(now.Add(time.Minute)) {
		fail(w, 400, "Выберите период в пределах последних 24 часов")
		return
	}
	ctx, cancel := dbContext(r)
	defer cancel()
	points, step, e := s.Monitor.Store.History(ctx, from, to)
	if e != nil {
		fail(w, 503, "История временно недоступна")
		return
	}
	reply(w, 200, map[string]any{"points": points, "step": step, "from": from, "to": to})
}
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "Поток недоступен")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, unsubscribe := s.Monitor.Subscribe()
	defer unsubscribe()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	controller := http.NewResponseController(w)
	send := func(update monitor.Update) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		status, _ := json.Marshal(update.Status)
		if _, err := fmt.Fprintf(w, "event: service\ndata: %s\n\n", status); err != nil {
			return false
		}
		if update.Sample != nil {
			data, _ := json.Marshal(update.Sample)
			if _, err := fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", data); err != nil {
				return false
			}
		}
		flusher.Flush()
		return true
	}
	latest := s.Monitor.Latest()
	if !send(monitor.Update{Sample: &latest, Status: s.Monitor.Status()}) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case update := <-ch:
			if !send(update) {
				return
			}
		case <-ticker.C:
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	data, err := fs.ReadFile(s.assets, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(path, "_nuxt/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path, time.Time{}, strings.NewReader(string(data)))
}
