package app

import "testing"

func TestHomeworkChoicesHideSolvedAndPendingKeepIncorrect(t *testing.T) {
	tasks := []HomeworkTask{
		{ID: "solved", Title: "Solved"},
		{ID: "retry", Title: "Retry"},
		{ID: "pending", Title: "Pending"},
		{ID: "new", Title: "New"},
		{ID: "was-correct", Title: "Was correct"},
	}
	rows := [][]string{
		{"solved", "correct", "lesson-1", "100", "110"},
		{"retry", "incorrect", "lesson-1", "100", "120"},
		{"pending", "assigned", "lesson-2", "130", "130"},
		{"was-correct", "correct", "lesson-1", "100", "110"},
		{"was-correct", "incorrect", "lesson-2", "120", "140"},
	}

	choices := homeworkTaskChoicesFromRows(tasks, rows, "lesson-3")
	if len(choices) != 2 {
		t.Fatalf("choices count = %d, want 2: %+v", len(choices), choices)
	}
	if choices[0].Task.ID != "retry" || choices[0].LastStatus != "incorrect" || choices[0].TimesAssigned != 1 {
		t.Fatalf("retry choice = %+v", choices[0])
	}
	if choices[1].Task.ID != "new" || choices[1].LastStatus != "" || choices[1].TimesAssigned != 0 {
		t.Fatalf("new choice = %+v", choices[1])
	}
}

func TestHomeworkChoicesHideRetryWhileNewAttemptPending(t *testing.T) {
	tasks := []HomeworkTask{{ID: "retry", Title: "Retry"}}
	rows := [][]string{
		{"retry", "incorrect", "lesson-1", "100", "120"},
		{"retry", "assigned", "lesson-2", "130", "130"},
	}
	if choices := homeworkTaskChoicesFromRows(tasks, rows, "lesson-3"); len(choices) != 0 {
		t.Fatalf("pending retry must be hidden, got %+v", choices)
	}
}
