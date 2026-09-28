package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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

func (u StudyUser) CabinetName() string {
	telegram := strings.TrimSpace(u.Telegram)
	telegram = strings.TrimSpace(strings.TrimLeft(telegram, "@"))
	if telegram != "" {
		return telegram
	}

	email := strings.TrimSpace(u.Email)
	if at := strings.IndexByte(email, '@'); at > 0 {
		return email[:at]
	}
	if email != "" {
		return email
	}

	return "student"
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
