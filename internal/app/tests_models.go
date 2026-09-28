package app

import "time"

type TestTemplate struct {
	ID              string
	Slug            string
	Title           string
	Description     string
	DurationMinutes int
	Status          string
	QuestionCount   int
	ScoredQuestions int
	MaxScore        int
	Questions       []TestQuestion
}

type TestQuestion struct {
	ID          string
	TestID      string
	SortOrder   int
	Section     string
	Prompt      string
	Code        string
	Kind        string
	Options     []TestOption
	Correct     []string
	Explanation string
	Category    string
	Scored      bool
}

type TestOption struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type TestOptionView struct {
	Key      string
	Text     string
	Selected bool
	Correct  bool
	State    string
}

type TestQuestionView struct {
	ID          string
	SortOrder   int
	Section     string
	Prompt      string
	Code        string
	Kind        string
	Scored      bool
	Explanation string
	Options     []TestOptionView
	Answered    bool
	IsCorrect   bool
}

type TestAssignment struct {
	ID               string
	TestID           string
	TestTitle        string
	UserID           string
	UserName         string
	UserEmail        string
	BookingID        string
	BookingDate      string
	BookingTime      string
	AttachMode       string
	Status           string
	TimeLimitMinutes int
	HasDeadline      bool
	DeadlineAt       time.Time
	CreatedAt        time.Time
	AttemptID        string
	AttemptStatus    string
	HasAttempt       bool
	StartedAt        time.Time
	ExpiresAt        time.Time
	SubmittedAt      time.Time
	Score            int
	MaxScore         int
}

type TestAttempt struct {
	ID           string
	AssignmentID string
	UserID       string
	Status       string
	StartedAt    time.Time
	ExpiresAt    time.Time
	SubmittedAt  time.Time
	UpdatedAt    time.Time
	Score        int
	MaxScore     int
	Answers      map[string][]string
}

type TestBreakdown struct {
	Key      string
	Label    string
	Score    int
	MaxScore int
	Level    string
}
