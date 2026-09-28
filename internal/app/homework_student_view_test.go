package app

import (
	"testing"
	"time"
)

func TestSplitStudentHomeworkAssignments(t *testing.T) {
	now := time.Now().UTC()
	assignments := []HomeworkAssignment{
		{ID: "repeat-new", TaskID: "repeat", Status: "assigned", AssignedAt: now},
		{ID: "open-new", TaskID: "open", Status: "incorrect", AssignedAt: now.Add(-time.Minute)},
		{ID: "done-new", TaskID: "done", Status: "correct", AssignedAt: now.Add(-2 * time.Minute)},
		{ID: "repeat-old", TaskID: "repeat", Status: "incorrect", AssignedAt: now.Add(-3 * time.Minute)},
		{ID: "done-old", TaskID: "done", Status: "incorrect", AssignedAt: now.Add(-4 * time.Minute)},
	}

	active, completed := splitStudentHomeworkAssignments(assignments)
	if len(active) != 2 {
		t.Fatalf("active len = %d, want 2", len(active))
	}
	if active[0].ID != "repeat-new" || active[1].ID != "open-new" {
		t.Fatalf("unexpected active assignments: %#v", active)
	}
	if len(completed) != 1 || completed[0].ID != "done-new" {
		t.Fatalf("unexpected completed assignments: %#v", completed)
	}
}

func TestSplitStudentHomeworkCorrectHidesOldIncorrect(t *testing.T) {
	assignments := []HomeworkAssignment{
		{ID: "correct", TaskID: "task", Status: "correct"},
		{ID: "incorrect", TaskID: "task", Status: "incorrect"},
	}
	active, completed := splitStudentHomeworkAssignments(assignments)
	if len(active) != 0 {
		t.Fatalf("correct task leaked into active homework: %#v", active)
	}
	if len(completed) != 1 || completed[0].ID != "correct" {
		t.Fatalf("completed = %#v", completed)
	}
}
