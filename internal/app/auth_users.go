package app

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

func (a *AuthService) CreateUser(name, email, telegram, password, role string) (StudyUser, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	telegram = strings.TrimSpace(telegram)
	if len([]rune(name)) < 2 || len([]rune(name)) > 80 {
		return StudyUser{}, errors.New("укажите имя от 2 до 80 символов")
	}
	if !validEmail(email) {
		return StudyUser{}, errors.New("укажите корректный email")
	}
	if role != "student" && role != "admin" {
		return StudyUser{}, errors.New("invalid role")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return StudyUser{}, errors.New("пароль должен содержать от 8 до 128 символов")
	}
	id, err := randomToken(12)
	if err != nil {
		return StudyUser{}, err
	}
	now := time.Now().UTC()
	err = a.db.WithTx(func(tx *dbTx) error {
		if err := tx.exec(`INSERT INTO users(id,email,password_hash,name,telegram,role,created_at) VALUES(?,?,?,?,?,?,?)`, id, email, hash, name, telegram, role, now.Unix()); err != nil {
			return err
		}
		if role == "student" {
			return tx.exec(`INSERT INTO student_profiles(user_id,timezone,created_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(user_id) DO NOTHING`, id, "Europe/Moscow", now.Unix(), now.Unix())
		}
		return nil
	})
	if err != nil {
		if isDBConstraint(err) {
			return StudyUser{}, errors.New("пользователь с таким email уже существует")
		}
		return StudyUser{}, err
	}
	return StudyUser{ID: id, Email: email, Name: name, Telegram: telegram, Role: role, Created: now}, nil
}

func (a *AuthService) Authenticate(email, password string) (StudyUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	rows, err := a.db.Query(`SELECT id,email,password_hash,name,telegram,role,created_at FROM users WHERE email = ? LIMIT 1`, email)
	if err != nil {
		return StudyUser{}, err
	}
	if len(rows) == 0 || !verifyPassword(rows[0][2], password) {
		return StudyUser{}, errors.New("неверный email или пароль")
	}
	row := rows[0]
	if passwordHashNeedsUpgrade(row[2]) {
		if upgraded, hashErr := hashPassword(password); hashErr == nil {
			_ = a.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, upgraded, row[0])
		}
	}
	return StudyUser{ID: row[0], Email: row[1], Name: row[3], Telegram: row[4], Role: row[5], Created: time.Unix(parseInt64(row[6]), 0).UTC()}, nil
}

func (a *AuthService) StartSession(w http.ResponseWriter, userID string) (UserSession, error) {
	token, err := randomToken(32)
	if err != nil {
		return UserSession{}, err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return UserSession{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(sessionLifetime)
	hash := tokenHash(token)
	if err := a.db.Exec(`INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,created_at) VALUES(?,?,?,?,?)`, hash, userID, csrf, expires.Unix(), now.Unix()); err != nil {
		return UserSession{}, err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires,
	})
	return UserSession{TokenHash: hash, CSRFToken: csrf, ExpiresAt: expires}, nil
}

func (a *AuthService) CurrentSession(r *http.Request) (*UserSession, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, nil
	}
	hash := tokenHash(cookie.Value)
	rows, err := a.db.Query(`SELECT s.token_hash,s.csrf_token,s.expires_at,u.id,u.email,u.name,u.telegram,u.role,u.created_at
		FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? LIMIT 1`, hash)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	row := rows[0]
	expires := time.Unix(parseInt64(row[2]), 0)
	if time.Now().After(expires) {
		_ = a.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, hash)
		return nil, nil
	}
	return &UserSession{
		TokenHash: hash,
		CSRFToken: row[1],
		ExpiresAt: expires,
		User:      StudyUser{ID: row[3], Email: row[4], Name: row[5], Telegram: row[6], Role: row[7], Created: time.Unix(parseInt64(row[8]), 0).UTC()},
	}, nil
}

func (a *AuthService) Logout(w http.ResponseWriter, session *UserSession) {
	if session != nil {
		_ = a.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, session.TokenHash)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (a *AuthService) CleanupSessions() {
	_ = a.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Unix())
}

func (a *AuthService) EnsureAnonCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(anonCSRFCookieName); err == nil && len(cookie.Value) >= 24 {
		return cookie.Value
	}
	token, _ := randomToken(24)
	http.SetCookie(w, &http.Cookie{Name: anonCSRFCookieName, Value: token, Path: "/study", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
	return token
}

func (a *AuthService) VerifyCSRF(r *http.Request, session *UserSession) bool {
	var got string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		got = strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	} else {
		got = strings.TrimSpace(r.FormValue("csrf_token"))
	}
	if session != nil {
		return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(session.CSRFToken)) == 1
	}
	cookie, err := r.Cookie(anonCSRFCookieName)
	return err == nil && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(cookie.Value)) == 1
}

func validEmail(s string) bool {
	at := strings.LastIndexByte(s, '@')
	return at > 0 && at < len(s)-3 && strings.Contains(s[at+1:], ".") && len(s) <= 254
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
