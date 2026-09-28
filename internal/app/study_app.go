package app

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type StudyPageData struct {
	Title                        string
	Active                       string
	User                         *StudyUser
	CSRF                         string
	Error                        string
	Success                      string
	Courses                      []StudyCourse
	Bookings                     []StudyBooking
	UpcomingBookings             []StudyBooking
	BookingHistory               []StudyBooking
	Lessons                      []StudyLesson
	Lesson                       StudyLesson
	HomeworkTasks                []HomeworkTask
	HomeworkTask                 HomeworkTask
	HomeworkTaskChoices          []HomeworkTaskChoice
	HomeworkAssignments          []HomeworkAssignment
	CompletedHomeworkAssignments []HomeworkAssignment
	HomeworkAssignment           HomeworkAssignment
	Notifications                []StudyNotification
	Students                     []StudyUser
	Stats                        AdminStats
	Timezone                     string
	Unread                       int
	TemporaryPassword            string
	PasswordResetUser            string
	CabinetTab                   string
	StudentSchedule              *TutoringSchedule
	StudentSchedules             []TutoringSchedule
	RescheduleRequests           []RescheduleRequest
	PendingReschedules           []RescheduleRequest
	TestTemplates                []TestTemplate
	TestTemplate                 TestTemplate
	TestAssignments              []TestAssignment
	TestAssignment               TestAssignment
	TestAttempt                  TestAttempt
	TestQuestions                []TestQuestionView
	TestBreakdown                []TestBreakdown
	TestStarted                  bool
	TestFinished                 bool
	TestAdminView                bool
}

type StudyApp struct {
	db                *AppDB
	auth              *AuthService
	loc               *time.Location
	schedule          map[time.Weekday][]string
	tg                *TelegramNotifier
	defaultMeetingURL string
}

func NewStudyApp(db *AppDB, auth *AuthService) *StudyApp {
	meetingURL := strings.TrimSpace(os.Getenv("DEFAULT_MEETING_URL"))
	if meetingURL != "" && !validHTTPURL(meetingURL) {
		log.Printf("ignoring invalid DEFAULT_MEETING_URL")
		meetingURL = ""
	}
	return &StudyApp{db: db, auth: auth, loc: bookingTimezone(), schedule: bookingSchedule(), tg: NewTelegramNotifier(), defaultMeetingURL: meetingURL}
}

func (s *StudyApp) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/study", s.studyHome)
	mux.HandleFunc("/study/login", s.login)
	mux.HandleFunc("/study/register", s.register)
	mux.HandleFunc("/study/logout", s.logout)
	mux.HandleFunc("/study/booking", s.bookingPage)
	mux.HandleFunc("/study/courses", s.coursesPage)
	mux.HandleFunc("/study/account", s.account)
	mux.HandleFunc("/study/account/password", s.accountPassword)
	mux.HandleFunc("/study/notifications", s.notificationsPage)
	mux.HandleFunc("/study/notifications/read", s.notificationRead)
	mux.HandleFunc("/study/admin", s.admin)
	mux.HandleFunc("/study/admin/bookings/status", s.adminBookingStatus)
	mux.HandleFunc("/study/admin/booking/update", s.adminBookingUpdate)
	mux.HandleFunc("/study/admin/booking/delete", s.adminBookingDelete)
	mux.HandleFunc("/study/admin/lesson", s.adminLesson)
	mux.HandleFunc("/study/admin/lesson/save", s.adminLessonSave)
	mux.HandleFunc("/study/admin/homework", s.adminHomeworkBank)
	mux.HandleFunc("/study/admin/homework/task/save", s.adminHomeworkTaskSave)
	mux.HandleFunc("/study/admin/homework/assign", s.adminHomeworkAssign)
	mux.HandleFunc("/study/admin/homework/status", s.adminHomeworkStatus)
	mux.HandleFunc("/study/homework", s.studentHomeworkView)
	mux.HandleFunc("/study/admin/students/password-reset", s.adminResetStudentPassword)
	mux.HandleFunc("/study/admin/reschedule/resolve", s.adminResolveReschedule)
	mux.HandleFunc("/study/admin/tests", s.adminTests)
	mux.HandleFunc("/study/admin/tests/view", s.adminTestView)
	mux.HandleFunc("/study/admin/tests/assign", s.adminTestAssign)
	mux.HandleFunc("/study/admin/tests/result", s.adminTestResult)
	mux.HandleFunc("/study/test", s.studyTest)
	mux.HandleFunc("/study/test/start", s.studyTestStart)
	mux.HandleFunc("/study/test/save", s.studyTestSave)
	mux.HandleFunc("/study/test/submit", s.studyTestSubmit)
	mux.HandleFunc("/api/study/availability", s.availabilityAPI)
	mux.HandleFunc("/api/study/bookings", s.bookingAPI)
	mux.HandleFunc("/api/study/recurring-slots", s.recurringSlotsAPI)
	mux.HandleFunc("/api/study/reschedule/availability", s.rescheduleAvailabilityAPI)
	mux.HandleFunc("/api/study/reschedule/temporary", s.temporaryRescheduleAPI)
	mux.HandleFunc("/api/study/reschedule/permanent", s.permanentRescheduleAPI)
	mux.HandleFunc("/api/study/calendar", s.calendarAPI)
	mux.HandleFunc("/api/study/admin/calendar", s.adminCalendarAPI)
	mux.HandleFunc("/api/study/admin/student", s.adminStudentAPI)
	mux.HandleFunc("/api/study/admin/lesson/save", s.adminLessonSaveAPI)
	mux.HandleFunc("/api/study/admin/homework/assign", s.adminHomeworkAssignAPI)
	mux.HandleFunc("/api/study/admin/homework/status", s.adminHomeworkStatusAPI)
}

func (s *StudyApp) session(r *http.Request) *UserSession {
	session, err := s.auth.CurrentSession(r)
	if err != nil {
		log.Printf("study session: %v", err)
		return nil
	}
	return session
}

func (s *StudyApp) baseData(w http.ResponseWriter, r *http.Request, title, active string) StudyPageData {
	data := StudyPageData{Title: title, Active: active, Timezone: s.loc.String()}
	if session := s.session(r); session != nil {
		user := session.User
		data.User = &user
		data.CSRF = session.CSRFToken
		if count, err := unreadNotificationCount(s.db, user.ID); err == nil {
			data.Unread = count
		}
	} else {
		data.CSRF = s.auth.EnsureAnonCSRF(w, r)
	}
	return data
}

func renderStudy(w http.ResponseWriter, name string, data StudyPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (s *StudyApp) studyHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/study" {
		http.NotFound(w, r)
		return
	}
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	renderStudy(w, "study_home.html", s.baseData(w, r, "MY.STUDY — Онлайн-обучение", "home"))
}
