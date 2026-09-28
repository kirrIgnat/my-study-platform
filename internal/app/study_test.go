package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func newTestDB(t *testing.T) (*AppDB, *AuthService) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; PostgreSQL integration test skipped")
	}
	db, err := OpenAppDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateStudyDB(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := resetTestDatabase(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, NewAuthService(db)
}

func resetTestDatabase(db *AppDB) error {
	return db.Exec(`TRUNCATE TABLE password_reset_events, lesson_materials, notifications, reschedule_requests, booking_suppressions, bookings, tutoring_schedules, enrollments, courses, sessions, student_profiles, users CASCADE`)
}

func futureWeekday(loc *time.Location, weekday time.Weekday, weeksAhead int) time.Time {
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, weeksAhead*7)
	for start.Weekday() != weekday {
		start = start.AddDate(0, 0, 1)
	}
	return start
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("VeryStrongPassword-123")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "VeryStrongPassword-123") {
		t.Fatal("expected password to verify")
	}
	if verifyPassword(hash, "wrong-password") {
		t.Fatal("wrong password verified")
	}
	if strings.Contains(hash, "VeryStrongPassword-123") {
		t.Fatal("plaintext password leaked into hash")
	}
}

func TestPostgresPlaceholderConversion(t *testing.T) {
	query := `SELECT * FROM users WHERE email=? AND role='student?' AND id=?`
	got := postgresPlaceholders(query)
	want := `SELECT * FROM users WHERE email=$1 AND role='student?' AND id=$2`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPasswordHashUpgradeDetection(t *testing.T) {
	old := strings.Replace("pbkdf2_sha256$600000$salt$key", "$600000$", "$210000$", 1)
	if !passwordHashNeedsUpgrade(old) {
		t.Fatal("old PBKDF2 cost should require upgrade")
	}
	hash, err := hashPassword("VeryStrongPassword-123")
	if err != nil {
		t.Fatal(err)
	}
	if passwordHashNeedsUpgrade(hash) {
		t.Fatal("current password hash should not require upgrade")
	}
}

func TestLessonDurationAndOverlap(t *testing.T) {
	if got := lessonEndTime("18:00"); got != "19:30" {
		t.Fatalf("lesson end = %s, want 19:30", got)
	}
	if !recurringSlotsOverlap(2, "18:00", 2, "19:00") {
		t.Fatal("18:00-19:30 must overlap 19:00-20:30")
	}
	if recurringSlotsOverlap(2, "18:00", 2, "19:30") {
		t.Fatal("adjacent 90-minute lessons must not overlap")
	}
	if !recurringSlotsOverlap(7, "23:30", 1, "00:30") {
		t.Fatal("weekly overlap across Sunday/Monday boundary was not detected")
	}
}

func TestRecurringScheduleIsPermanentAndNotifiesAdmin(t *testing.T) {
	db, auth := newTestDB(t)
	defer db.Close()
	admin, _ := auth.CreateUser("Admin", "admin@example.com", "", "StrongAdmin-123", "admin")
	student, _ := auth.CreateUser("Student", "student@example.com", "@student", "StrongStudent-123", "student")
	student2, _ := auth.CreateUser("Student Two", "student2@example.com", "", "StrongStudent-456", "student")
	loc, _ := time.LoadLocation("Europe/Moscow")
	date := futureWeekday(loc, time.Tuesday, 2).Format("2006-01-02")

	schedule, err := createRecurringSchedule(db, student, date, "18:00", "Go concurrency", "https://meet.example.com/class", loc, defaultLessonSchedule)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.Weekday != 2 || schedule.Time != "18:00" || schedule.EffectiveFrom != date {
		t.Fatalf("unexpected schedule: %#v", schedule)
	}
	bookings, err := listUserBookings(db, student.ID, loc)
	if err != nil || len(bookings) < 4 {
		t.Fatalf("expected generated recurring lessons, count=%d err=%v", len(bookings), err)
	}
	for _, booking := range bookings {
		if booking.ScheduleID == schedule.ID && booking.Date < date {
			t.Fatalf("generated lesson before student's selected start date: got %s, start %s", booking.Date, date)
		}
	}
	secondDate := futureWeekday(loc, time.Thursday, 2).Format("2006-01-02")
	secondSchedule, err := createRecurringSchedule(db, student, secondDate, "19:30", "Linux", "", loc, defaultLessonSchedule)
	if err != nil {
		t.Fatalf("student should be able to create a second recurring lesson: %v", err)
	}
	schedules, err := getUserSchedules(db, student.ID)
	if err != nil || len(schedules) != 2 || secondSchedule.ID == "" {
		t.Fatalf("expected two recurring schedules, got %#v err=%v", schedules, err)
	}
	if _, err := createRecurringSchedule(db, student2, date, "18:00", "", "", loc, defaultLessonSchedule); !errors.Is(err, errStudyRecurringSlotBusy) {
		t.Fatalf("expected recurring slot conflict, got %v", err)
	}
	notifications, err := listNotifications(db, admin.ID)
	if err != nil || len(notifications) == 0 || notifications[0].Type != "schedule_created" {
		t.Fatalf("admin was not notified: %#v err=%v", notifications, err)
	}
}

func TestTemporaryRescheduleApprovalAndMonthlyLimit(t *testing.T) {
	db, auth := newTestDB(t)
	defer db.Close()
	admin, _ := auth.CreateUser("Admin", "admin@example.com", "", "StrongAdmin-123", "admin")
	student, _ := auth.CreateUser("Student", "student@example.com", "", "StrongStudent-123", "student")
	loc, _ := time.LoadLocation("Europe/Moscow")
	date := futureWeekday(loc, time.Tuesday, 2).Format("2006-01-02")
	if _, err := createRecurringSchedule(db, student, date, "18:00", "", "", loc, defaultLessonSchedule); err != nil {
		t.Fatal(err)
	}
	bookings, _ := listUserBookings(db, student.ID, loc)
	byMonth := map[string][]StudyBooking{}
	nowLocal := time.Now().In(loc)
	cutoff := nowLocal.AddDate(0, 0, maxBookingDays-3)
	for _, booking := range bookings {
		if booking.SlotStart.After(nowLocal) && booking.SlotStart.Before(cutoff) && len(booking.Date) >= 7 {
			byMonth[booking.Date[:7]] = append(byMonth[booking.Date[:7]], booking)
		}
	}
	var candidates []StudyBooking
	for _, group := range byMonth {
		if len(group) >= 3 {
			candidates = group[:3]
			break
		}
	}
	if len(candidates) < 3 {
		t.Fatal("test needs three recurring lessons in one month")
	}
	for i := 0; i < 2; i++ {
		original, _ := time.ParseInLocation("2006-01-02", candidates[i].Date, loc)
		targetDate := original.AddDate(0, 0, 2).Format("2006-01-02")
		req, err := requestTemporaryReschedule(db, student, candidates[i].ID, targetDate, "19:30", "test", loc, defaultLessonSchedule)
		if err != nil {
			t.Fatalf("temporary request %d: %v", i, err)
		}
		if err := resolveRescheduleRequest(db, admin.ID, req.ID, "approve", "ok", loc, defaultLessonSchedule); err != nil {
			t.Fatalf("approve %d: %v", i, err)
		}
		moved, _ := getBookingForUser(db, candidates[i].ID, student.ID, loc)
		if moved.Date != targetDate || !moved.Rescheduled {
			t.Fatalf("booking was not moved: %#v", moved)
		}
	}
	original, _ := time.ParseInLocation("2006-01-02", candidates[2].Date, loc)
	_, err := requestTemporaryReschedule(db, student, candidates[2].ID, original.AddDate(0, 0, 2).Format("2006-01-02"), "19:30", "third", loc, defaultLessonSchedule)
	if !errors.Is(err, errTemporaryMoveLimitReached) {
		t.Fatalf("expected monthly limit, got %v", err)
	}
	studentNotifications, _ := listNotifications(db, student.ID)
	found := false
	for _, n := range studentNotifications {
		if n.Type == "reschedule_approved" {
			found = true
		}
	}
	if !found {
		t.Fatal("student did not receive approval notification")
	}
}

func TestPermanentRescheduleRequiresApproval(t *testing.T) {
	db, auth := newTestDB(t)
	defer db.Close()
	admin, _ := auth.CreateUser("Admin", "admin@example.com", "", "StrongAdmin-123", "admin")
	student, _ := auth.CreateUser("Student", "student@example.com", "", "StrongStudent-123", "student")
	loc, _ := time.LoadLocation("Europe/Moscow")
	date := futureWeekday(loc, time.Tuesday, 2).Format("2006-01-02")
	if _, err := createRecurringSchedule(db, student, date, "18:00", "", "", loc, defaultLessonSchedule); err != nil {
		t.Fatal(err)
	}
	current, _ := getUserSchedule(db, student.ID)
	req, err := requestPermanentReschedule(db, student, current.ID, 4, "18:00", "work", loc, defaultLessonSchedule)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := getUserSchedule(db, student.ID)
	if before.Weekday != 2 {
		t.Fatal("schedule changed before approval")
	}
	if err := resolveRescheduleRequest(db, admin.ID, req.ID, "approve", "approved", loc, defaultLessonSchedule); err != nil {
		t.Fatal(err)
	}
	after, _ := getUserSchedule(db, student.ID)
	if after.Weekday != 4 || after.Time != "18:00" {
		t.Fatalf("schedule not updated: %#v", after)
	}
}

func TestLessonMaterialPublicationNotifiesStudent(t *testing.T) {
	db, auth := newTestDB(t)
	defer db.Close()
	student, _ := auth.CreateUser("Student", "student@example.com", "", "StrongStudent-123", "student")
	loc, _ := time.LoadLocation("Europe/Moscow")
	date := futureWeekday(loc, time.Tuesday, 2).Format("2006-01-02")
	if _, err := createRecurringSchedule(db, student, date, "18:00", "Pointers", "", loc, defaultLessonSchedule); err != nil {
		t.Fatal(err)
	}
	bookings, _ := listUserBookings(db, student.ID, loc)
	var booking StudyBooking
	for _, b := range bookings {
		if b.Date == date {
			booking = b
			break
		}
	}
	if booking.ID == "" {
		t.Fatal("first recurring lesson missing")
	}
	if err := saveLessonMaterial(db, booking.ID, "Pointers", "Theory text", "Homework text", "https://video.example.com/lesson", true); err != nil {
		t.Fatal(err)
	}
	lessons, _ := listPublishedLessonsForStudent(db, student.ID, loc)
	if len(lessons) != 1 || lessons[0].Homework != "Homework text" || lessons[0].Status != "done" {
		t.Fatalf("unexpected lesson: %#v", lessons)
	}
	notifications, _ := listNotifications(db, student.ID)
	found := false
	for _, n := range notifications {
		if n.Type == "lesson_materials" && strings.Contains(n.Body, "домашнее") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected homework notification")
	}
}

func TestAdminPasswordResetNeverStoresPlaintext(t *testing.T) {
	db, auth := newTestDB(t)
	defer db.Close()
	admin, _ := auth.CreateUser("Admin", "admin@example.com", "", "StrongAdmin-123", "admin")
	student, _ := auth.CreateUser("Student", "student@example.com", "", "OldStrongPassword-123", "student")
	temporary, err := auth.ResetStudentPassword(admin.ID, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(student.Email, temporary); err != nil {
		t.Fatalf("temporary password should authenticate: %v", err)
	}
	rows, _ := db.Query(`SELECT password_hash FROM users WHERE id=?`, student.ID)
	if len(rows) != 1 || strings.Contains(rows[0][0], temporary) || rows[0][0] == temporary {
		t.Fatal("plaintext password leaked into database")
	}
}

func TestPostgresSchemaContainsStudentDataTables(t *testing.T) {
	db, _ := newTestDB(t)
	defer db.Close()
	for _, table := range []string{"users", "student_profiles", "sessions", "tutoring_schedules", "bookings", "lesson_materials", "reschedule_requests", "notifications"} {
		rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, table)
		if err != nil || len(rows) != 1 {
			t.Fatalf("table %s missing: rows=%v err=%v", table, rows, err)
		}
	}
}
