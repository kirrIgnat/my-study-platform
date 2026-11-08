package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func validBuilderRequest() BuilderRequest {
	return BuilderRequest{ID: "12345678-1234-1234-1234-123456789abc", Preset: "minimal", Theme: "light", Accent: "#7768d8", Layout: "sidebar", Style: "soft", Font: "sans", Home: "overview", Name: "Username", Contact: "username@example.org", Description: "Описание Ваших занятий", Features: []string{"whiteboard", "homework"}}
}
func TestBuilderValidation(t *testing.T) {
	b := validBuilderRequest()
	if err := b.validate(); err != nil {
		t.Fatal(err)
	}
	tests := []func(*BuilderRequest){func(b *BuilderRequest) { b.Preset = "unknown" }, func(b *BuilderRequest) { b.Features = []string{"courses"} }, func(b *BuilderRequest) { b.Features = []string{"auth", "auth"} }, func(b *BuilderRequest) { b.Accent = "red;url(x)" }, func(b *BuilderRequest) { b.Description = "short" }, func(b *BuilderRequest) { b.ID = "../test" }, func(b *BuilderRequest) { b.Company = "spam" }, func(b *BuilderRequest) { b.Font = "external" }}
	for i, change := range tests {
		b := validBuilderRequest()
		change(&b)
		if b.validate() == nil {
			t.Errorf("case %d accepted invalid input", i)
		}
	}
}
func TestBuilderRejectBeforeDatabase(t *testing.T) {
	s := &StudyApp{}
	tests := []struct {
		method, body, origin, content string
		status                        int
	}{{"GET", "", "", "", 405}, {"POST", "{}", "https://evil.example", "application/json", 403}, {"POST", "{}", "", "text/plain", 415}, {"POST", "{}", "", "application/json", 400}, {"POST", strings.Repeat("x", 33000), "", "application/json", 400}}
	for _, tc := range tests {
		r := httptest.NewRequest(tc.method, "https://kirr-ignat.dev/site-builder/api/requests", strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		s.builderSubmit(w, r)
		if w.Code != tc.status {
			t.Fatalf("want %d got %d", tc.status, w.Code)
		}
	}
}
