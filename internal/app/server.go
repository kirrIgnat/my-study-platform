package app

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PageData struct {
	Title string
}

var templates = mustLoadTemplates()

func projectPath(candidates ...string) string {
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func mustLoadTemplates() *template.Template {
	dir := projectPath("web/templates", "../../web/templates")
	return template.Must(template.ParseGlob(filepath.Join(dir, "*.html")))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func onlyGet(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

func render(w http.ResponseWriter, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func Run() {
	mux := http.NewServeMux()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required (PostgreSQL), e.g. postgres://kirill_study:password@127.0.0.1:5432/kirill_study?sslmode=disable")
	}
	db, err := OpenAppDB(databaseURL)
	if err != nil {
		log.Fatalf("open PostgreSQL database: %v", err)
	}
	defer db.Close()
	if err := migrateStudyDB(db); err != nil {
		log.Fatalf("migrate study database: %v", err)
	}
	auth := NewAuthService(db)
	if err := bootstrapAdmin(auth); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	auth.CleanupSessions()
	study := NewStudyApp(db, auth)
	study.RegisterRoutes(mux)

	staticDir := projectPath("web/static", "../../web/static")
	static := http.FileServer(http.Dir(staticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", static))

	mux.HandleFunc("/", onlyGet(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		render(w, "loading.html", PageData{Title: "Кирилл Игнатьев — Загрузка"})
	}))

	mux.HandleFunc("/profile", onlyGet(func(w http.ResponseWriter, r *http.Request) {
		render(w, "profile.html", PageData{Title: "Кирилл Игнатьев — Разработчик"})
	}))

	mux.HandleFunc("/projects", onlyGet(func(w http.ResponseWriter, r *http.Request) {
		render(w, "projects.html", PageData{Title: "Проекты — Coming Soon"})
	}))

	mux.HandleFunc("/lessons", onlyGet(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/study/booking", http.StatusMovedPermanently)
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	addr := "127.0.0.1:" + port

	srv := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("Portfolio server listening on http://%s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
