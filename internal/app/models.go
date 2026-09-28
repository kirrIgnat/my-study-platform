package app

import (
	"errors"
	"html/template"
	"time"
)

type StudyCourse struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Level       string
	Duration    string
	Status      string
	Enrolled    bool
	Progress    int
	Students    int
}

type StudyBooking struct {
	ID                  string
	UserID              string
	UserName            string
	UserEmail           string
	Telegram            string
	SlotStart           time.Time
	Date                string
	Time                string
	EndTime             string
	DurationMinutes     int
	Timezone            string
	Topic               string
	MeetingURL          string
	WeekKey             string
	ScheduleID          string
	OccurrenceKey       string
	OriginalDate        string
	OriginalTime        string
	Rescheduled         bool
	Status              string
	CreatedAt           time.Time
	Tests               []TestAssignment     `json:"-"`
	HomeworkAssignments []HomeworkAssignment `json:"-"`
}

type TutoringSchedule struct {
	ID              string
	UserID          string
	UserName        string
	UserEmail       string
	Weekday         int
	WeekdayName     string
	Time            string
	EndTime         string
	DurationMinutes int
	Timezone        string
	Topic           string
	MeetingURL      string
	EffectiveFrom   string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RecurringSlotOption struct {
	Weekday     int    `json:"weekday"`
	WeekdayName string `json:"weekday_name"`
	Time        string `json:"time"`
	Available   bool   `json:"available"`
	Current     bool   `json:"current"`
}

type RescheduleRequest struct {
	ID               string
	UserID           string
	UserName         string
	UserEmail        string
	ScheduleID       string
	BookingID        string
	Kind             string
	RequestedWeekday int
	RequestedDayName string
	RequestedTime    string
	RequestedDate    string
	RequestedSlot    time.Time
	MonthKey         string
	Reason           string
	Status           string
	TeacherNote      string
	OriginalDate     string
	OriginalTime     string
	CreatedAt        time.Time
	ResolvedAt       time.Time
}

type HomeworkTask struct {
	ID              string
	Title           string
	Topic           string
	Language        string
	Difficulty      string
	DifficultyLabel string
	StatementMD     string
	InputMD         string
	OutputMD        string
	ExamplesMD      string
	StatementHTML   template.HTML
	InputHTML       template.HTML
	OutputHTML      template.HTML
	ExamplesHTML    template.HTML
	Active          bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type HomeworkAssignment struct {
	ID          string
	TaskID      string
	UserID      string
	BookingID   string
	AssignedBy  string
	Status      string
	StatusLabel string
	TeacherNote string
	AssignedAt  time.Time
	UpdatedAt   time.Time
	Task        HomeworkTask
}

type HomeworkTaskChoice struct {
	Task              HomeworkTask
	TimesAssigned     int
	LastStatus        string
	LastStatusLabel   string
	AlreadyThisLesson bool
}

type StudyLesson struct {
	BookingID           string
	ScheduleID          string
	UserID              string
	UserName            string
	UserEmail           string
	Date                string
	Time                string
	EndTime             string
	DurationMinutes     int
	Timezone            string
	Topic               string
	MeetingURL          string
	Status              string
	Title               string
	Theory              string
	TheoryHTML          template.HTML
	Homework            string
	HomeworkHTML        template.HTML
	VideoURL            string
	Published           bool
	PublishedAt         time.Time
	UpdatedAt           time.Time
	Tests               []TestAssignment     `json:"-"`
	HomeworkAssignments []HomeworkAssignment `json:"-"`
}

type StudyNotification struct {
	ID                  string
	Type                string
	Title               string
	Body                string
	BookingID           string
	RescheduleRequestID string
	Read                bool
	CreatedAt           time.Time
}

type AdminStats struct {
	Students         int
	Bookings         int
	Upcoming         int
	Unread           int
	PublishedLessons int
	PendingChanges   int
	CourseMembers    int
}

var (
	errStudySlotBusy             = errors.New("study slot already booked")
	errStudyRecurringSlotBusy    = errors.New("recurring slot already assigned")
	errTemporaryMoveLimitReached = errors.New("temporary move monthly limit reached")
	errRescheduleAlreadyPending  = errors.New("reschedule request already pending")
)
