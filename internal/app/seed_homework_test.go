package app

import "testing"

func TestCHomeworkSeedCatalog(t *testing.T) {
	if len(cHomeworkSeedTasks) != 14 {
		t.Fatalf("seed task count = %d, want 14", len(cHomeworkSeedTasks))
	}
	seen := make(map[string]bool, len(cHomeworkSeedTasks))
	for _, task := range cHomeworkSeedTasks {
		if task.ID == "" || task.Title == "" || task.Topic == "" || task.Statement == "" || task.Input == "" || task.Output == "" {
			t.Fatalf("incomplete seed task: %+v", task)
		}
		if seen[task.ID] {
			t.Fatalf("duplicate seed ID: %s", task.ID)
		}
		seen[task.ID] = true
		if task.Language != "C" {
			t.Fatalf("task %s language = %q, want C", task.ID, task.Language)
		}
		if task.Difficulty != "easy" {
			t.Fatalf("task %s difficulty = %q, want easy", task.ID, task.Difficulty)
		}
	}
}
