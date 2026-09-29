package app

import (
	"testing"
	"time"
)

func TestHomeworkPublicIDNumber(t *testing.T) {
	tests := []struct {
		value string
		n     int
		ok    bool
	}{
		{"id_t1", 1, true},
		{"ID_T27", 27, true},
		{" id_t103 ", 103, true},
		{"id_t0", 0, false},
		{"id_t01", 0, false},
		{"task_1", 0, false},
		{"id_tabc", 0, false},
	}
	for _, tt := range tests {
		n, ok := homeworkPublicIDNumber(tt.value)
		if n != tt.n || ok != tt.ok {
			t.Fatalf("homeworkPublicIDNumber(%q) = (%d,%v), want (%d,%v)", tt.value, n, ok, tt.n, tt.ok)
		}
	}
}

func TestHomeworkLanguageAndFilename(t *testing.T) {
	cases := map[string]string{
		"C":      ".c",
		"C++":    ".cpp",
		"Go":     ".go",
		"Python": ".py",
	}
	for language, extension := range cases {
		if got := homeworkSourceExtension(language); got != extension {
			t.Fatalf("extension for %q = %q, want %q", language, got, extension)
		}
	}
}

func TestDecorateHomeworkAssignment(t *testing.T) {
	loc := time.FixedZone("test", 3*60*60)
	assignment := HomeworkAssignment{
		AssignedAt: time.Date(2026, 9, 19, 12, 30, 0, 0, time.UTC),
		Task:       HomeworkTask{PublicID: "id_t15", Language: "C"},
	}
	decorateHomeworkAssignment(&assignment, loc)
	if assignment.HomeworkDir != "homework/hw_19092026" {
		t.Fatalf("HomeworkDir = %q", assignment.HomeworkDir)
	}
	if assignment.SourceFilename != "id_t15.c" || assignment.CodeLanguage != "C" {
		t.Fatalf("unexpected source defaults: filename=%q language=%q", assignment.SourceFilename, assignment.CodeLanguage)
	}

	assignment.Submission = &HomeworkSubmission{Language: "C++"}
	decorateHomeworkAssignment(&assignment, loc)
	if assignment.SourceFilename != "id_t15.cpp" || assignment.Submission.Filename != "id_t15.cpp" {
		t.Fatalf("unexpected C++ filename: assignment=%q submission=%q", assignment.SourceFilename, assignment.Submission.Filename)
	}
}
