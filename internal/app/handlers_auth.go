package app

import (
	"log"
	"net/http"
)

func (s *StudyApp) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if session := s.session(r); session != nil {
			http.Redirect(w, r, dashboardPath(session.User.Role), http.StatusSeeOther)
			return
		}
		renderStudy(w, "study_login.html", s.baseData(w, r, "Вход — KIRILL.STUDY", "login"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
		return
	}
	data := s.baseData(w, r, "Вход — KIRILL.STUDY", "login")
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, nil) {
		data.Error = "Сессия формы устарела. Обновите страницу и попробуйте снова."
		renderStudy(w, "study_login.html", data)
		return
	}
	if !s.auth.limiter.Allow("login:" + clientIP(r)) {
		data.Error = "Слишком много попыток входа. Попробуйте позже."
		w.WriteHeader(http.StatusTooManyRequests)
		renderStudy(w, "study_login.html", data)
		return
	}
	user, err := s.auth.Authenticate(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		data.Error = "Неверный email или пароль."
		renderStudy(w, "study_login.html", data)
		return
	}
	if _, err := s.auth.StartSession(w, user.ID); err != nil {
		log.Printf("start session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, dashboardPath(user.Role), http.StatusSeeOther)
}

func (s *StudyApp) register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if session := s.session(r); session != nil {
			http.Redirect(w, r, dashboardPath(session.User.Role), http.StatusSeeOther)
			return
		}
		renderStudy(w, "study_register.html", s.baseData(w, r, "Регистрация — KIRILL.STUDY", "login"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
		return
	}
	data := s.baseData(w, r, "Регистрация — KIRILL.STUDY", "login")
	r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, nil) {
		data.Error = "Сессия формы устарела. Обновите страницу и попробуйте снова."
		renderStudy(w, "study_register.html", data)
		return
	}
	if !s.auth.limiter.Allow("register:" + clientIP(r)) {
		data.Error = "Слишком много попыток регистрации. Попробуйте позже."
		w.WriteHeader(http.StatusTooManyRequests)
		renderStudy(w, "study_register.html", data)
		return
	}
	password := r.FormValue("password")
	if password != r.FormValue("password_confirm") {
		data.Error = "Пароли не совпадают."
		renderStudy(w, "study_register.html", data)
		return
	}
	user, err := s.auth.CreateUser(r.FormValue("name"), r.FormValue("email"), r.FormValue("telegram"), password, "student")
	if err != nil {
		data.Error = err.Error()
		renderStudy(w, "study_register.html", data)
		return
	}
	if _, err := s.auth.StartSession(w, user.ID); err != nil {
		log.Printf("start session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/booking", http.StatusSeeOther)
}

func (s *StudyApp) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.auth.Logout(w, session)
	http.Redirect(w, r, "/study", http.StatusSeeOther)
}
