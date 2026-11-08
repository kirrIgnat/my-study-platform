package app

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed builderweb/index.html builderweb/assets/*
var builderFiles embed.FS

const builderSchema = `CREATE TABLE IF NOT EXISTS site_builder_requests (
 id TEXT PRIMARY KEY,
 recipient_id TEXT NOT NULL REFERENCES users(id),
 notification_id TEXT NOT NULL UNIQUE REFERENCES notifications(id),
 payload JSONB NOT NULL,
 status TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','viewed','in_progress','completed')),
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL
)`

type BuilderRequest struct {
 PrimaryLight string `json:"primary_light,omitempty"`
 PrimaryDark string `json:"primary_dark,omitempty"`
	ID          string   `json:"id"`
	Preset      string   `json:"preset"`
	Theme       string   `json:"theme"`
	Accent      string   `json:"accent"`
	Layout      string   `json:"layout"`
	Style       string   `json:"style"`
	Font        string   `json:"font"`
	Home        string   `json:"home"`
	Features    []string `json:"features"`
	SiteName    string   `json:"site_name"`
	Description string   `json:"description"`
	Extra       string   `json:"extra"`
	Name        string   `json:"name"`
	Contact     string   `json:"contact"`
	Company     string   `json:"company"`
}

var builderFeatures = map[string]string{
	"auth": "Авторизация и регистрация", "teacher-panel": "Кабинет преподавателя", "student-profile": "Кабинет ученика", "schedule": "Индивидуальное расписание", "recurring": "Повторяющиеся занятия", "reschedule": "Перенос и отмена занятий", "homework": "Домашние задания", "materials": "Прикрепление файлов", "comments": "Комментарии к заданиям", "completion": "Отметки выполнения", "reminders": "Уведомления", "telegram": "Интеграция с Telegram", "payments": "Учет оплат", "progress": "Прогресс ученика", "admin": "Административная панель", "whiteboard": "Доска с сохранением в карточке занятия",
}
var builderPresets = map[string]string{"minimal": "Study Minimal", "cozy": "Study Cozy", "modern": "Study Modern", "personal": "Tutor Portfolio", "custom": "Своя идея"}
var builderStatuses = map[string]string{"new": "Новая", "viewed": "Просмотрена", "in_progress": "В работе", "completed": "Завершена"}
var builderHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var builderID = regexp.MustCompile(`^[a-zA-Z0-9-]{20,80}$`)

func builderChoice(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}
func (b *BuilderRequest) validate() error {
 for _, color := range []string{b.PrimaryLight, b.PrimaryDark} {
 if color != "" && !builderHex.MatchString(color) { return errors.New("Некорректный основной цвет") }
 }
	b.Name = strings.TrimSpace(b.Name)
	b.Contact = strings.TrimSpace(b.Contact)
	b.Description = strings.TrimSpace(b.Description)
	if !builderID.MatchString(b.ID) || builderPresets[b.Preset] == "" || !builderChoice(b.Theme, "light", "dark") || !builderHex.MatchString(b.Accent) || !builderChoice(b.Layout, "sidebar", "topbar") || !builderChoice(b.Style, "soft", "sharp") || !builderChoice(b.Font, "sans", "serif", "mono") || !builderChoice(b.Home, "overview", "schedule", "profile") {
		return errors.New("Проверьте настройки оформления")
	}
	if len([]rune(b.Name)) < 2 || len([]rune(b.Name)) > 100 || len([]rune(b.Contact)) < 3 || len([]rune(b.Contact)) > 160 || len([]rune(b.Description)) < 15 || len([]rune(b.Description)) > 4000 || len([]rune(b.Extra)) > 2000 || len([]rune(b.SiteName)) > 100 || len(b.Features) > 16 || b.Company != "" {
		return errors.New("Проверьте имя, контакт и описание проекта")
	}
	seen := map[string]bool{}
	for _, f := range b.Features {
		if builderFeatures[f] == "" || seen[f] {
			return errors.New("Некорректный список функций")
		}
		seen[f] = true
	}
	return nil
}
func (b BuilderRequest) summary() string {
	features := []string{}
	for _, f := range b.Features {
		features = append(features, builderFeatures[f])
	}
	primary := ""
 if b.Preset == "custom" {
 light, dark := b.PrimaryLight, b.PrimaryDark
 if light == "" { light = "#ffffff" }; if dark == "" { dark = "#1e2636" }
 primary = fmt.Sprintf("Основной цвет (светлая тема): %s\nОсновной цвет (темная тема): %s\n", light, dark)
 }
 return primary + fmt.Sprintf("Заявка %s\nИмя: %s\nКонтакт: %s\nНазвание: %s\nПресет: %s\nТема: %s\nАкцентный цвет: %s\nМеню: %s\nФормы: %s\nШрифт: %s\nГлавная: %s\nФункции: %s\n\nОписание:\n%s\n\nДополнительные пожелания:\n%s", b.ID, b.Name, b.Contact, b.SiteName, builderPresets[b.Preset], b.Theme, b.Accent, b.Layout, b.Style, b.Font, b.Home, strings.Join(features, ", "), b.Description, b.Extra)
}

// Global cap is deliberately independent of untrusted X-Forwarded-For headers.
var builderRate = struct {
	sync.Mutex
	start time.Time
	count int
}{}

func builderAllowed() bool {
	builderRate.Lock()
	defer builderRate.Unlock()
	if time.Since(builderRate.start) > time.Minute {
		builderRate.start = time.Now()
		builderRate.count = 0
	}
	builderRate.count++
	return builderRate.count <= 30
}
func (s *StudyApp) registerSiteBuilder(mux *http.ServeMux) {
	// Failure affects only the new module, not existing Study routes.
	if err := s.db.Exec(builderSchema); err != nil {
		log.Printf("site builder disabled: %v", err)
		return
	}
	sub, _ := fs.Sub(builderFiles, "builderweb")
	files := http.FileServer(http.FS(sub))
	mux.HandleFunc("/site-builder", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/site-builder/", http.StatusPermanentRedirect)
	})
	mux.HandleFunc("/site-builder/", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, "GET", "HEAD") {
			return
		}
		if r.URL.Path != "/site-builder/" && r.URL.Path != "/site-builder/assets/app.js" && r.URL.Path != "/site-builder/assets/style.css" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; connect-src 'self'")
		http.StripPrefix("/site-builder/", files).ServeHTTP(w, r)
	})
	mux.HandleFunc("/site-builder/api/requests", s.builderSubmit)
	mux.HandleFunc("/study/site-request", s.builderDetail)
	mux.HandleFunc("/study/site-request/status", s.builderStatus)
}
func (s *StudyApp) builderSubmit(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, "POST") {
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || !builderChoice(u.Scheme, "http", "https") {
			writeJSONError(w, "Недопустимый источник", 403)
			return
		}
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeJSONError(w, "Недопустимый источник", 403)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONError(w, "Нужен JSON", 415)
		return
	}
	if !builderAllowed() {
		w.Header().Set("Retry-After", "60")
		writeJSONError(w, "Слишком много заявок. Повторите через минуту", 429)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	defer r.Body.Close()
	var b BuilderRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&b); err != nil {
		writeJSONError(w, "Некорректная заявка", 400)
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		writeJSONError(w, "Некорректный JSON", 400)
		return
	}
	if err := b.validate(); err != nil {
		writeJSONError(w, err.Error(), 400)
		return
	}
	payload, _ := json.Marshal(b)
	now := time.Now().UTC().Unix()
	err := s.db.WithTx(func(tx *dbTx) error {
		// Serialize submissions and retries; no duplicate notification on retry.
		if err := tx.exec(`SELECT pg_advisory_xact_lock(735193820)`); err != nil {
			return err
		}
		existing, err := tx.query(`SELECT payload::text FROM site_builder_requests WHERE id=?`, b.ID)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			var old BuilderRequest
			if json.Unmarshal([]byte(existing[0][0]), &old) != nil {
				return errors.New("invalid saved request")
			}
			p, _ := json.Marshal(old)
			if string(p) != string(payload) {
				return errors.New("idempotency conflict")
			}
			return nil
		}
		email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
		var owners [][]string
		if email != "" {
			owners, err = tx.query(`SELECT id FROM users WHERE role='admin' AND email=?`, email)
		} else {
			owners, err = tx.query(`SELECT id FROM users WHERE role='admin'`)
		}
		if err != nil {
			return err
		}
		if len(owners) != 1 {
			return errors.New("set ADMIN_EMAIL to exactly one existing admin")
		}
		nid, err := randomToken(12)
		if err != nil {
			return err
		}
		if err = tx.exec(`INSERT INTO notifications(id,recipient_user_id,type,title,body,created_at) VALUES(?,?,?,?,?,?)`, nid, owners[0][0], "site_builder", "Новая заявка на разработку образовательного сайта", b.summary(), now); err != nil {
			return err
		}
		return tx.exec(`INSERT INTO site_builder_requests(id,recipient_id,notification_id,payload,created_at,updated_at) VALUES(?,?,?,?::jsonb,?,?)`, b.ID, owners[0][0], nid, string(payload), now, now)
	})
	if err != nil {
		log.Printf("builder submit: %v", err)
		writeJSONError(w, "Не удалось сохранить заявку. Попробуйте позже", 503)
		return
	}
	writeJSON(w, map[string]any{"ok": true}, http.StatusCreated)
}
func (s *StudyApp) builderDetail(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, "GET", "HEAD") {
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	rows, err := s.db.Query(`SELECT id,payload::text,status,created_at FROM site_builder_requests WHERE notification_id=? AND recipient_id=?`, r.URL.Query().Get("notification"), session.User.ID)
	if err != nil {
		http.Error(w, "Ошибка чтения", 500)
		return
	}
	if len(rows) != 1 {
		http.NotFound(w, r)
		return
	}
	var b BuilderRequest
	if json.Unmarshal([]byte(rows[0][1]), &b) != nil {
		http.Error(w, "Ошибка данных", 500)
		return
	}
	data := struct{ Title, ID, Body, Status, CSRF, Notification, Created string }{"Заявка на сайт", rows[0][0], b.summary(), rows[0][2], session.CSRFToken, r.URL.Query().Get("notification"), rows[0][3]}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := templates.ExecuteTemplate(w, "site_builder_request.html", data); err != nil {
		log.Printf("builder detail template: %v", err)
	}
}
func (s *StudyApp) builderStatus(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, "POST") {
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "CSRF", 403)
		return
	}
	status := r.FormValue("status")
	nid := r.FormValue("notification")
	if builderStatuses[status] == "" {
		http.Error(w, "Неизвестный статус", 400)
		return
	}
	err := s.db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`UPDATE site_builder_requests SET status=?,updated_at=? WHERE notification_id=? AND recipient_id=? RETURNING id`, status, time.Now().UTC().Unix(), nid, session.User.ID)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return errors.New("request not found")
		}
		return tx.exec(`UPDATE notifications SET read_at=COALESCE(read_at,?) WHERE id=? AND recipient_user_id=?`, time.Now().UTC().Unix(), nid, session.User.ID)
	})
	if err != nil {
		http.Error(w, "Не удалось обновить заявку", 400)
		return
	}
	http.Redirect(w, r, "/study/site-request?notification="+url.QueryEscape(nid), http.StatusSeeOther)
}
