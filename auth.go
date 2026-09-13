package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	passwordIterations = 600000
	sessionLifetime    = 14 * 24 * time.Hour
	sessionCookieName  = "study_session"
	anonCSRFCookieName = "study_csrf"
)

type StudyUser struct {
	ID       string
	Email    string
	Name     string
	Telegram string
	Role     string
	Created  time.Time
}

type UserSession struct {
	User      StudyUser
	TokenHash string
	CSRFToken string
	ExpiresAt time.Time
}

type AuthService struct {
	db           *AppDB
	cookieSecure bool
	limiter      *loginLimiter
}

func NewAuthService(db *AppDB) *AuthService {
	secure := strings.EqualFold(strings.TrimSpace(os.Getenv("COOKIE_SECURE")), "true") || strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
	return &AuthService{db: db, cookieSecure: secure, limiter: newLoginLimiter(10, 15*time.Minute)}
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 128 {
		return "", errors.New("password must be between 8 and 128 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordIterations, 32)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 100000 || iterations > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) < 16 {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func passwordHashNeedsUpgrade(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return true
	}
	iterations, err := strconv.Atoi(parts[1])
	return err != nil || iterations < passwordIterations
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func randomToken(bytesCount int) (string, error) {
	buf := make([]byte, bytesCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

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

type loginBucket struct {
	Count int
	Start time.Time
}

type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	items  map[string]loginBucket
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, items: make(map[string]loginBucket)}
}

func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	bucket := l.items[key]
	if bucket.Start.IsZero() || now.Sub(bucket.Start) > l.window {
		l.items[key] = loginBucket{Count: 1, Start: now}
		return true
	}
	if bucket.Count >= l.max {
		return false
	}
	bucket.Count++
	l.items[key] = bucket
	return true
}

func bootstrapAdmin(auth *AuthService) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}
	rows, err := auth.db.Query(`SELECT id,role FROM users WHERE email=? LIMIT 1`, email)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		if rows[0][1] != "admin" {
			return auth.db.Exec(`UPDATE users SET role='admin' WHERE id=?`, rows[0][0])
		}
		return nil
	}
	name := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	if name == "" {
		name = "Кирилл"
	}
	_, err = auth.CreateUser(name, email, strings.TrimSpace(os.Getenv("ADMIN_TELEGRAM")), password, "admin")
	return err
}

func generateTemporaryPassword(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%"
	if length < 12 {
		length = 12
	}
	buf := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(buf), nil
}

func (a *AuthService) ResetStudentPassword(adminID, studentID string) (string, error) {
	rows, err := a.db.Query(`SELECT role FROM users WHERE id=? LIMIT 1`, studentID)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 || rows[0][0] != "student" {
		return "", errors.New("студент не найден")
	}
	password, err := generateTemporaryPassword(18)
	if err != nil {
		return "", err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	eventID, err := randomToken(12)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Unix()
	if err := a.db.WithTx(func(tx *dbTx) error {
		if err := tx.exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, studentID); err != nil {
			return err
		}
		if err := tx.exec(`DELETE FROM sessions WHERE user_id=?`, studentID); err != nil {
			return err
		}
		return tx.exec(`INSERT INTO password_reset_events(id,admin_user_id,target_user_id,created_at) VALUES(?,?,?,?)`, eventID, adminID, studentID, now)
	}); err != nil {
		return "", err
	}
	return password, nil
}

func (a *AuthService) ChangePassword(userID, currentTokenHash, currentPassword, newPassword string) error {
	rows, err := a.db.Query(`SELECT password_hash FROM users WHERE id=? LIMIT 1`, userID)
	if err != nil {
		return err
	}
	if len(rows) == 0 || !verifyPassword(rows[0][0], currentPassword) {
		return errors.New("текущий пароль указан неверно")
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return errors.New("новый пароль должен содержать от 8 до 128 символов")
	}
	return a.db.WithTx(func(tx *dbTx) error {
		if err := tx.exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, userID); err != nil {
			return err
		}
		return tx.exec(`DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, userID, currentTokenHash)
	})
}
