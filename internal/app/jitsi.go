package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

type jitsiConfig struct {
	PublicURL     string
	Origin        string
	Domain        string
	JWTSubject    string
	AppID         string
	AppSecret     string
	RecordingsDir string
}

func loadJitsiConfig() jitsiConfig {
	cfg := jitsiConfig{
		AppID:         strings.TrimSpace(os.Getenv("JITSI_APP_ID")),
		AppSecret:     strings.TrimSpace(os.Getenv("JITSI_APP_SECRET")),
		RecordingsDir: strings.TrimSpace(os.Getenv("JIBRI_RECORDINGS_DIR")),
	}
	rawURL := strings.TrimSpace(os.Getenv("JITSI_PUBLIC_URL"))
	if rawURL == "" {
		return cfg
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		log.Printf("ignoring invalid JITSI_PUBLIC_URL")
		return cfg
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") && parsed.Scheme != "https" {
		log.Printf("ignoring insecure JITSI_PUBLIC_URL in production")
		return cfg
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path != "" {
		log.Printf("JITSI_PUBLIC_URL must not include a path; ignoring %q", rawURL)
		return cfg
	}
	cfg.PublicURL = strings.TrimRight(parsed.String(), "/")
	cfg.Origin = parsed.Scheme + "://" + parsed.Host
	cfg.Domain = parsed.Host
	cfg.JWTSubject = parsed.Hostname()
	return cfg
}

func (c jitsiConfig) Enabled() bool {
	return c.PublicURL != "" && c.Domain != "" && c.AppID != "" && c.AppSecret != ""
}

func (c jitsiConfig) RecordingEnabled() bool {
	return c.Enabled() && c.RecordingsDir != ""
}

func (c jitsiConfig) token(user StudyUser, room string, moderator bool, now time.Time) (string, error) {
	if !c.Enabled() {
		return "", errors.New("Jitsi не настроен")
	}
	room = strings.TrimSpace(room)
	if room == "" {
		return "", errors.New("комната конференции не задана")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	role := "member"
	if moderator {
		role = "owner"
	}
	claims := map[string]any{
		"aud":  c.AppID,
		"iss":  c.AppID,
		"sub":  c.JWTSubject,
		"room": room,
		"iat":  now.Add(-15 * time.Second).Unix(),
		"nbf":  now.Add(-15 * time.Second).Unix(),
		"exp":  now.Add(4 * time.Hour).Unix(),
		"context": map[string]any{
			"user": map[string]any{
				"id":          user.ID,
				"name":        user.Name,
				"email":       user.Email,
				"moderator":   moderator,
				"affiliation": role,
			},
			"features": map[string]any{
				"recording":     moderator,
				"livestreaming": false,
				"transcription": false,
			},
		},
	}
	return signHS256JWT(claims, c.AppSecret)
}

func signHS256JWT(claims map[string]any, secret string) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString(headerJSON) + "." + enc.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + enc.EncodeToString(mac.Sum(nil)), nil
}

func newConferenceRoomName() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "lesson-" + hex.EncodeToString(buf), nil
}

func jitsiSecuritySources() (origin, websocket string) {
	cfg := loadJitsiConfig()
	if cfg.Origin == "" {
		return "", ""
	}
	parsed, err := url.Parse(cfg.Origin)
	if err != nil {
		return "", ""
	}
	wsScheme := "wss"
	if parsed.Scheme == "http" {
		wsScheme = "ws"
	}
	return cfg.Origin, wsScheme + "://" + parsed.Host
}
