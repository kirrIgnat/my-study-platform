package app

import (
	"net/http"
	"strings"
)

type calendarEventJSON struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id,omitempty"`
	UserName    string `json:"user_name,omitempty"`
	Date        string `json:"date"`
	Time        string `json:"time"`
	Timezone    string `json:"timezone"`
	Topic       string `json:"topic"`
	Status      string `json:"status"`
	MeetingURL  string `json:"meeting_url,omitempty"`
	Rescheduled bool   `json:"rescheduled"`
	ScheduleID  string `json:"schedule_id,omitempty"`
}

func calendarEventsFromBookings(bookings []StudyBooking) []calendarEventJSON {
	result := make([]calendarEventJSON, 0, len(bookings))
	for _, b := range bookings {
		result = append(result, calendarEventJSON{ID: b.ID, UserID: b.UserID, UserName: b.UserName, Date: b.Date, Time: b.Time, Timezone: b.Timezone, Topic: b.Topic, Status: b.Status, MeetingURL: b.MeetingURL, Rescheduled: b.Rescheduled, ScheduleID: b.ScheduleID})
	}
	return result
}

func (s *StudyApp) calendarAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		writeJSONError(w, "необходимо войти", http.StatusUnauthorized)
		return
	}
	if session.User.Role == "admin" {
		s.adminCalendarAPI(w, r)
		return
	}
	_ = ensureRecurringBookings(s.db, s.loc, 20)
	bookings, err := listUserBookings(s.db, session.User.ID, s.loc)
	if err != nil {
		writeJSONError(w, "не удалось загрузить календарь", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"timezone": s.loc.String(), "events": calendarEventsFromBookings(bookings)}, http.StatusOK)
}

func (s *StudyApp) adminCalendarAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	if s.requireAdmin(w, r) == nil {
		return
	}
	_ = ensureRecurringBookings(s.db, s.loc, 20)
	bookings, err := listAdminBookings(s.db, s.loc)
	if err != nil {
		writeJSONError(w, "не удалось загрузить календарь", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"timezone": s.loc.String(), "events": calendarEventsFromBookings(bookings)}, http.StatusOK)
}

func (s *StudyApp) adminStudentAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	if s.requireAdmin(w, r) == nil {
		return
	}
	studentID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if studentID == "" {
		writeJSONError(w, "не указан ученик", http.StatusBadRequest)
		return
	}
	student, err := getStudentByID(s.db, studentID)
	if err != nil {
		writeJSONError(w, "не удалось загрузить ученика", http.StatusInternalServerError)
		return
	}
	if student == nil {
		writeJSONError(w, "ученик не найден", http.StatusNotFound)
		return
	}
	_ = ensureRecurringBookings(s.db, s.loc, 20)
	schedules, err := getUserSchedules(s.db, studentID)
	if err != nil {
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	bookings, err := listUserBookings(s.db, studentID, s.loc)
	if err != nil {
		writeJSONError(w, "не удалось загрузить занятия", http.StatusInternalServerError)
		return
	}
	lessons, err := listLessonsForStudentAdmin(s.db, studentID, s.loc)
	if err != nil {
		writeJSONError(w, "не удалось загрузить материалы", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"student":   map[string]any{"id": student.ID, "name": student.Name, "email": student.Email, "telegram": student.Telegram, "created_at": student.Created},
		"schedules": schedules,
		"events":    calendarEventsFromBookings(bookings),
		"lessons":   lessons,
	}, http.StatusOK)
}

type adminLessonSaveRequest struct {
	BookingID string `json:"booking_id"`
	Title     string `json:"title"`
	Theory    string `json:"theory"`
	Homework  string `json:"homework"`
	VideoURL  string `json:"video_url"`
	Publish   bool   `json:"publish"`
}

func (s *StudyApp) adminLessonSaveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "csrf validation failed", http.StatusForbidden)
		return
	}
	var req adminLessonSaveRequest
	if !s.decodeJSON(w, r, &req, 80<<10) {
		return
	}
	req.BookingID = strings.TrimSpace(req.BookingID)
	if req.BookingID == "" {
		writeJSONError(w, "не выбрано занятие", http.StatusBadRequest)
		return
	}
	lesson, err := getAdminLesson(s.db, req.BookingID, s.loc)
	if err != nil {
		writeJSONError(w, "занятие не найдено", http.StatusNotFound)
		return
	}
	if lesson.UserID == "" {
		writeJSONError(w, "занятие не найдено", http.StatusNotFound)
		return
	}
	if err := saveLessonMaterial(s.db, req.BookingID, req.Title, req.Theory, req.Homework, req.VideoURL, req.Publish); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "Материалы сохранены"}, http.StatusOK)
}
