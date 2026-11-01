package app

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJitsiTokenScopedToLessonRoom(t *testing.T) {
	cfg := jitsiConfig{PublicURL: "https://meet.example.com", Origin: "https://meet.example.com", Domain: "meet.example.com", JWTSubject: "meet.example.com", AppID: "my-study", AppSecret: "test-secret"}
	user := StudyUser{ID: "student-1", Name: "Student", Email: "student@example.com", Role: "student"}
	token, err := cfg.token(user, "lesson-aabbcc", false, time.Unix(1_800_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT parts: %d", len(parts))
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["room"] != "lesson-aabbcc" {
		t.Fatalf("room claim = %#v", payload["room"])
	}
	context, ok := payload["context"].(map[string]any)
	if !ok {
		t.Fatal("missing context")
	}
	ctxUser, ok := context["user"].(map[string]any)
	if !ok {
		t.Fatal("missing context.user")
	}
	if ctxUser["affiliation"] != "member" || ctxUser["moderator"] != false {
		t.Fatalf("unexpected student role: %#v", ctxUser)
	}
}

func TestJitsiModeratorTokenUsesOwnerAffiliation(t *testing.T) {
	cfg := jitsiConfig{PublicURL: "https://meet.example.com", Origin: "https://meet.example.com", Domain: "meet.example.com", JWTSubject: "meet.example.com", AppID: "my-study", AppSecret: "test-secret"}
	user := StudyUser{ID: "admin-1", Name: "Teacher", Email: "teacher@example.com", Role: "admin"}
	token, err := cfg.token(user, "lesson-room", true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	payloadBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var payload map[string]any
	_ = json.Unmarshal(payloadBytes, &payload)
	ctxUser := payload["context"].(map[string]any)["user"].(map[string]any)
	if ctxUser["affiliation"] != "owner" || ctxUser["moderator"] != true {
		t.Fatalf("unexpected teacher role: %#v", ctxUser)
	}
}

func TestRecordingMetadataIdentity(t *testing.T) {
	bookingID, room := recordingMetadataIdentity([]byte(`{"meeting_url":"https://meet.example.com/lesson-123","additional_metadata":{"booking_id":"booking-42"}}`))
	if bookingID != "booking-42" || room != "lesson-123" {
		t.Fatalf("booking=%q room=%q", bookingID, room)
	}
}

func TestSafeRecordingPathRejectsTraversal(t *testing.T) {
	if _, err := safeRecordingPath("/tmp/recordings", "../secret.mp4"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	path, err := safeRecordingPath("/tmp/recordings", "session/file.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/tmp/recordings/session/file.mp4") {
		t.Fatalf("unexpected path %q", path)
	}
}
