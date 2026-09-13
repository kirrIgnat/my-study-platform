package app

import (
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

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
