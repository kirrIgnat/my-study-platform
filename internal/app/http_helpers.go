package app

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func (s *StudyApp) requireStudentJSON(w http.ResponseWriter, r *http.Request) *UserSession {
	session := s.session(r)
	if session == nil {
		writeJSONError(w, "для этого действия необходимо войти", http.StatusUnauthorized)
		return nil
	}
	if session.User.Role != "student" {
		writeJSONError(w, "действие доступно только студенту", http.StatusForbidden)
		return nil
	}
	if !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "csrf validation failed", http.StatusForbidden)
		return nil
	}
	return session
}

func (s *StudyApp) decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONError(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONError(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *StudyApp) requireAdmin(w http.ResponseWriter, r *http.Request) *UserSession {
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return nil
	}
	if session.User.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return session
}

func userID(user *StudyUser) string {
	if user == nil {
		return ""
	}
	return user.ID
}

func normalizeCabinetTab(raw, role string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	allowed := map[string]bool{}
	if role == "admin" {
		allowed = map[string]bool{"overview": true, "schedule": true, "transfers": true, "students": true, "tests": true}
	} else {
		allowed = map[string]bool{"overview": true, "schedule": true, "materials": true, "transfers": true, "security": true}
	}
	if allowed[raw] {
		return raw
	}
	return "overview"
}

func dashboardPath(role string) string {
	if role == "admin" {
		return "/study/admin"
	}
	return "/study/account"
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	methodNotAllowed(w, methods...)
	return false
}

func allowMethodJSON(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func writeJSON(w http.ResponseWriter, payload any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json response: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	writeJSON(w, map[string]any{"ok": false, "error": message}, status)
}
