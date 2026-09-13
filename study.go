package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type StudyPageData struct {
	Title              string
	Active             string
	User               *StudyUser
	CSRF               string
	Error              string
	Success            string
	Courses            []StudyCourse
	Bookings           []StudyBooking
	UpcomingBookings   []StudyBooking
	BookingHistory     []StudyBooking
	Lessons            []StudyLesson
	Lesson             StudyLesson
	Notifications      []StudyNotification
	Students           []StudyUser
	Stats              AdminStats
	Timezone           string
	Unread             int
	TemporaryPassword  string
	PasswordResetUser  string
	CabinetTab         string
	StudentSchedule    *TutoringSchedule
	StudentSchedules   []TutoringSchedule
	RescheduleRequests []RescheduleRequest
	PendingReschedules []RescheduleRequest
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
	mux.HandleFunc("/study/admin/lesson", s.adminLesson)
	mux.HandleFunc("/study/admin/lesson/save", s.adminLessonSave)
	mux.HandleFunc("/study/admin/students/password-reset", s.adminResetStudentPassword)
	mux.HandleFunc("/study/admin/reschedule/resolve", s.adminResolveReschedule)
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
	renderStudy(w, "study_home.html", s.baseData(w, r, "KIRILL.STUDY — Онлайн-обучение", "home"))
}

func (s *StudyApp) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if session := s.session(r); session != nil {
			http.Redirect(w, r, dashboardPath(session.User.Role), http.StatusSeeOther)
			return
		}
		renderStudy(w, "study_login.html", s.baseData(w, r, "Вход — KIRILL.STUDY", "login"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
		return
	}
	data := s.baseData(w, r, "Вход — KIRILL.STUDY", "login")
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, nil) {
		data.Error = "Сессия формы устарела. Обновите страницу и попробуйте снова."
		renderStudy(w, "study_login.html", data)
		return
	}
	if !s.auth.limiter.Allow("login:" + clientIP(r)) {
		data.Error = "Слишком много попыток входа. Попробуйте позже."
		w.WriteHeader(http.StatusTooManyRequests)
		renderStudy(w, "study_login.html", data)
		return
	}
	user, err := s.auth.Authenticate(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		data.Error = "Неверный email или пароль."
		renderStudy(w, "study_login.html", data)
		return
	}
	if _, err := s.auth.StartSession(w, user.ID); err != nil {
		log.Printf("start session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, dashboardPath(user.Role), http.StatusSeeOther)
}

func (s *StudyApp) register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if session := s.session(r); session != nil {
			http.Redirect(w, r, dashboardPath(session.User.Role), http.StatusSeeOther)
			return
		}
		renderStudy(w, "study_register.html", s.baseData(w, r, "Регистрация — KIRILL.STUDY", "login"))
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
		return
	}
	data := s.baseData(w, r, "Регистрация — KIRILL.STUDY", "login")
	r.Body = http.MaxBytesReader(w, r.Body, 20<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, nil) {
		data.Error = "Сессия формы устарела. Обновите страницу и попробуйте снова."
		renderStudy(w, "study_register.html", data)
		return
	}
	if !s.auth.limiter.Allow("register:" + clientIP(r)) {
		data.Error = "Слишком много попыток регистрации. Попробуйте позже."
		w.WriteHeader(http.StatusTooManyRequests)
		renderStudy(w, "study_register.html", data)
		return
	}
	password := r.FormValue("password")
	if password != r.FormValue("password_confirm") {
		data.Error = "Пароли не совпадают."
		renderStudy(w, "study_register.html", data)
		return
	}
	user, err := s.auth.CreateUser(r.FormValue("name"), r.FormValue("email"), r.FormValue("telegram"), password, "student")
	if err != nil {
		data.Error = err.Error()
		renderStudy(w, "study_register.html", data)
		return
	}
	if _, err := s.auth.StartSession(w, user.ID); err != nil {
		log.Printf("start session: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/booking", http.StatusSeeOther)
}

func (s *StudyApp) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.auth.Logout(w, session)
	http.Redirect(w, r, "/study", http.StatusSeeOther)
}

func (s *StudyApp) bookingPage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	data := s.baseData(w, r, "Постоянное расписание — KIRILL.STUDY", "booking")
	if data.User != nil && data.User.Role == "student" {
		data.StudentSchedules, _ = getUserSchedules(s.db, data.User.ID)
		if len(data.StudentSchedules) > 0 {
			data.StudentSchedule = &data.StudentSchedules[0]
		}
	}
	renderStudy(w, "study_booking.html", data)
}

func (s *StudyApp) coursesPage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	renderStudy(w, "study_courses.html", s.baseData(w, r, "Курсы — скоро | KIRILL.STUDY", "courses"))
}

func (s *StudyApp) studentAccountData(w http.ResponseWriter, r *http.Request, session *UserSession) StudyPageData {
	_ = ensureRecurringBookings(s.db, s.loc, 16)
	data := s.baseData(w, r, "Личный кабинет — KIRILL.STUDY", "account")
	data.CabinetTab = normalizeCabinetTab(r.URL.Query().Get("tab"), "student")
	data.StudentSchedules, _ = getUserSchedules(s.db, session.User.ID)
	if len(data.StudentSchedules) > 0 {
		data.StudentSchedule = &data.StudentSchedules[0]
	}
	bookings, err := listUserBookings(s.db, session.User.ID, s.loc)
	if err != nil {
		log.Printf("account bookings: %v", err)
	}
	data.Bookings = bookings
	now := time.Now().In(s.loc)
	for _, booking := range bookings {
		if booking.Status != "cancelled" && booking.Status != "done" && booking.SlotStart.After(now) {
			data.UpcomingBookings = append(data.UpcomingBookings, booking)
		} else {
			data.BookingHistory = append(data.BookingHistory, booking)
		}
	}
	sort.Slice(data.UpcomingBookings, func(i, j int) bool {
		return data.UpcomingBookings[i].SlotStart.Before(data.UpcomingBookings[j].SlotStart)
	})
	data.Lessons, _ = listPublishedLessonsForStudent(s.db, session.User.ID, s.loc)
	data.RescheduleRequests, _ = listRescheduleRequestsForUser(s.db, session.User.ID, s.loc)
	data.Notifications, _ = listNotifications(s.db, session.User.ID)
	if len(data.Notifications) > 5 {
		data.Notifications = data.Notifications[:5]
	}
	return data
}

func (s *StudyApp) account(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role == "admin" {
		http.Redirect(w, r, "/study/admin", http.StatusSeeOther)
		return
	}
	data := s.studentAccountData(w, r, session)
	switch r.URL.Query().Get("status") {
	case "password-changed":
		data.Success = "Пароль успешно изменён. Остальные активные сессии завершены."
	case "move-requested":
		data.Success = "Запрос на перенос отправлен преподавателю. Решение появится в уведомлениях."
	}
	renderStudy(w, "study_account.html", data)
}

func (s *StudyApp) accountPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role != "student" || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	newPassword := r.FormValue("new_password")
	if newPassword != r.FormValue("new_password_confirm") {
		data := s.studentAccountData(w, r, session)
		data.CabinetTab = "security"
		data.Error = "Новые пароли не совпадают."
		renderStudy(w, "study_account.html", data)
		return
	}
	if err := s.auth.ChangePassword(session.User.ID, session.TokenHash, r.FormValue("current_password"), newPassword); err != nil {
		data := s.studentAccountData(w, r, session)
		data.CabinetTab = "security"
		data.Error = err.Error()
		renderStudy(w, "study_account.html", data)
		return
	}
	http.Redirect(w, r, "/study/account?tab=security&status=password-changed", http.StatusSeeOther)
}

func (s *StudyApp) notificationsPage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	data := s.baseData(w, r, "Уведомления — KIRILL.STUDY", "notifications")
	data.CabinetTab = "notifications"
	data.Notifications, _ = listNotifications(s.db, session.User.ID)
	if session.User.Role == "admin" {
		data.PendingReschedules, _ = listPendingRescheduleRequests(s.db, s.loc)
	} else {
		data.RescheduleRequests, _ = listRescheduleRequestsForUser(s.db, session.User.ID, s.loc)
	}
	switch r.URL.Query().Get("status") {
	case "resolved":
		data.Success = "Запрос на перенос обработан. Ученик получил уведомление."
	case "read":
		data.Success = "Уведомления отмечены прочитанными."
	}
	renderStudy(w, "study_notifications.html", data)
}

func (s *StudyApp) notificationRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.FormValue("all") == "1" {
		_ = markAllNotificationsRead(s.db, session.User.ID)
	} else {
		_ = markNotificationRead(s.db, session.User.ID, strings.TrimSpace(r.FormValue("notification_id")))
	}
	http.Redirect(w, r, "/study/notifications?status=read", http.StatusSeeOther)
}

func (s *StudyApp) adminData(w http.ResponseWriter, r *http.Request, session *UserSession) StudyPageData {
	_ = ensureRecurringBookings(s.db, s.loc, 16)
	data := s.baseData(w, r, "Админ-панель — KIRILL.STUDY", "admin")
	data.CabinetTab = normalizeCabinetTab(r.URL.Query().Get("tab"), "admin")
	data.Stats, _ = adminStats(s.db, time.Now(), session.User.ID)
	data.Unread = data.Stats.Unread
	data.Bookings, _ = listAdminBookings(s.db, s.loc)
	data.Notifications, _ = listNotifications(s.db, session.User.ID)
	if len(data.Notifications) > 6 {
		data.Notifications = data.Notifications[:6]
	}
	data.PendingReschedules, _ = listPendingRescheduleRequests(s.db, s.loc)
	data.Students, _ = listStudents(s.db)
	return data
}

func (s *StudyApp) admin(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	data := s.adminData(w, r, session)
	if r.URL.Query().Get("status") == "resolved" {
		data.Success = "Запрос на перенос обработан. Ученик получил уведомление."
	}
	renderStudy(w, "study_admin.html", data)
}

func (s *StudyApp) adminBookingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := updateBookingStatus(s.db, strings.TrimSpace(r.FormValue("booking_id")), strings.TrimSpace(r.FormValue("status"))); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/study/admin?tab=schedule", http.StatusSeeOther)
}

func (s *StudyApp) adminBookingUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	if err := updateBookingAdmin(s.db, bookingID, strings.TrimSpace(r.FormValue("status")), r.FormValue("meeting_url")); err != nil {
		data := s.baseData(w, r, "Занятие — KIRILL.STUDY", "admin")
		data.CabinetTab = "schedule"
		data.Error = err.Error()
		data.Lesson, _ = getAdminLesson(s.db, bookingID, s.loc)
		renderStudy(w, "study_admin_lesson.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=schedule", http.StatusSeeOther)
}

func (s *StudyApp) adminLesson(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) || s.requireAdmin(w, r) == nil {
		return
	}
	bookingID := strings.TrimSpace(r.URL.Query().Get("id"))
	lesson, err := getAdminLesson(s.db, bookingID, s.loc)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := s.baseData(w, r, "Занятие — KIRILL.STUDY", "admin")
	data.CabinetTab = "schedule"
	data.Lesson = lesson
	switch r.URL.Query().Get("saved") {
	case "schedule":
		data.Success = "Параметры занятия сохранены. Ученик получил уведомление об изменениях."
	case "materials":
		data.Success = "Материалы сохранены как черновик."
	case "published":
		data.Success = "Материалы опубликованы. Ученик получил уведомление о новом ДЗ/материалах."
	}
	renderStudy(w, "study_admin_lesson.html", data)
}

func (s *StudyApp) adminLessonSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	current, err := getAdminLesson(s.db, bookingID, s.loc)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	publish := strings.TrimSpace(r.FormValue("action")) == "publish" || current.Published
	if err := saveLessonMaterial(s.db, bookingID, r.FormValue("title"), r.FormValue("theory"), r.FormValue("homework"), r.FormValue("video_url"), publish); err != nil {
		data := s.baseData(w, r, "Занятие — KIRILL.STUDY", "admin")
		data.CabinetTab = "schedule"
		data.Error = err.Error()
		data.Lesson = current
		data.Lesson.Title, data.Lesson.Theory, data.Lesson.Homework, data.Lesson.VideoURL = r.FormValue("title"), r.FormValue("theory"), r.FormValue("homework"), r.FormValue("video_url")
		renderStudy(w, "study_admin_lesson.html", data)
		return
	}
	state := "materials"
	if publish {
		state = "published"
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved="+state, http.StatusSeeOther)
}

func (s *StudyApp) adminResetStudentPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	studentID := strings.TrimSpace(r.FormValue("student_id"))
	password, err := s.auth.ResetStudentPassword(session.User.ID, studentID)
	data := s.adminData(w, r, session)
	data.CabinetTab = "students"
	if err != nil {
		data.Error = err.Error()
		renderStudy(w, "study_admin.html", data)
		return
	}
	for _, student := range data.Students {
		if student.ID == studentID {
			data.PasswordResetUser = student.Name + " · " + student.Email
			break
		}
	}
	data.TemporaryPassword = password
	data.Success = "Пароль сброшен. Временный пароль показывается только сейчас."
	renderStudy(w, "study_admin.html", data)
}

func (s *StudyApp) adminResolveReschedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := resolveRescheduleRequest(s.db, session.User.ID, strings.TrimSpace(r.FormValue("request_id")), strings.TrimSpace(r.FormValue("decision")), r.FormValue("teacher_note"), s.loc, s.schedule); err != nil {
		data := s.adminData(w, r, session)
		data.CabinetTab = "transfers"
		data.Error = err.Error()
		renderStudy(w, "study_admin.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin?tab=transfers&status=resolved", http.StatusSeeOther)
}

func (s *StudyApp) availabilityAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	_ = ensureRecurringBookings(s.db, s.loc, 16)
	days := defaultBookingDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}
	studentID := ""
	if session := s.session(r); session != nil && session.User.Role == "student" {
		studentID = session.User.ID
	}
	availability, err := studyAvailability(s.db, s.loc, s.schedule, days, studentID)
	if err != nil {
		log.Printf("study availability: %v", err)
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	writeJSON(w, availability, http.StatusOK)
}

type studyBookingRequest struct {
	Date  string `json:"date"`
	Time  string `json:"time"`
	Topic string `json:"topic"`
}

func (s *StudyApp) bookingAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireStudentJSON(w, r)
	if session == nil {
		return
	}
	var req studyBookingRequest
	if !s.decodeJSON(w, r, &req, 8<<10) {
		return
	}
	req.Date, req.Time, req.Topic = strings.TrimSpace(req.Date), strings.TrimSpace(req.Time), strings.TrimSpace(req.Topic)
	if len([]rune(req.Topic)) > 1000 {
		writeJSONError(w, "комментарий слишком длинный", http.StatusBadRequest)
		return
	}
	schedule, err := createRecurringSchedule(s.db, session.User, req.Date, req.Time, req.Topic, s.defaultMeetingURL, s.loc, s.schedule)
	if err != nil {
		switch {
		case errors.Is(err, errStudySlotBusy), errors.Is(err, errStudyRecurringSlotBusy):
			writeJSONError(w, "этот постоянный слот уже занят другим учеником", http.StatusConflict)
		default:
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	if err := s.tg.NotifyBooking(StudyBooking{UserName: session.User.Name, UserEmail: session.User.Email, Telegram: session.User.Telegram, Date: req.Date, Time: req.Time, Timezone: s.loc.String(), Topic: req.Topic, Status: "confirmed"}); err != nil {
		log.Printf("telegram schedule notification: %v", err)
	}
	writeJSON(w, map[string]any{"ok": true, "schedule": schedule}, http.StatusCreated)
}

func (s *StudyApp) recurringSlotsAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil || session.User.Role != "student" {
		writeJSONError(w, "нужен аккаунт студента", http.StatusUnauthorized)
		return
	}
	currentScheduleID := strings.TrimSpace(r.URL.Query().Get("schedule_id"))
	if currentScheduleID != "" {
		if schedule, err := getUserScheduleByID(s.db, session.User.ID, currentScheduleID); err != nil || schedule == nil {
			writeJSONError(w, "постоянное занятие не найдено", http.StatusBadRequest)
			return
		}
	}
	options, err := listRecurringSlotOptions(s.db, s.schedule, s.loc, session.User.ID, currentScheduleID)
	if err != nil {
		writeJSONError(w, "не удалось загрузить слоты", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"timezone": s.loc.String(), "slots": options}, http.StatusOK)
}

func (s *StudyApp) rescheduleAvailabilityAPI(w http.ResponseWriter, r *http.Request) {
	if !allowMethodJSON(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil || session.User.Role != "student" {
		writeJSONError(w, "нужен аккаунт студента", http.StatusUnauthorized)
		return
	}
	availability, err := rescheduleAvailability(s.db, s.loc, s.schedule, session.User.ID, strings.TrimSpace(r.URL.Query().Get("booking_id")))
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, availability, http.StatusOK)
}

type temporaryRescheduleRequestJSON struct {
	BookingID string `json:"booking_id"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Reason    string `json:"reason"`
}

func (s *StudyApp) temporaryRescheduleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireStudentJSON(w, r)
	if session == nil {
		return
	}
	var req temporaryRescheduleRequestJSON
	if !s.decodeJSON(w, r, &req, 12<<10) {
		return
	}
	if len([]rune(req.Reason)) > 1000 {
		writeJSONError(w, "комментарий слишком длинный", http.StatusBadRequest)
		return
	}
	_, err := requestTemporaryReschedule(s.db, session.User, strings.TrimSpace(req.BookingID), strings.TrimSpace(req.Date), strings.TrimSpace(req.Time), req.Reason, s.loc, s.schedule)
	if err != nil {
		switch {
		case errors.Is(err, errTemporaryMoveLimitReached):
			writeJSONError(w, "лимит разовых переносов на этот месяц исчерпан: максимум 2", http.StatusConflict)
		case errors.Is(err, errRescheduleAlreadyPending):
			writeJSONError(w, "по этому занятию уже ожидается решение преподавателя", http.StatusConflict)
		case errors.Is(err, errStudySlotBusy), errors.Is(err, errStudyRecurringSlotBusy):
			writeJSONError(w, "выбранный слот уже занят", http.StatusConflict)
		default:
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "Запрос отправлен преподавателю"}, http.StatusCreated)
}

type permanentRescheduleRequestJSON struct {
	ScheduleID string `json:"schedule_id"`
	Weekday    int    `json:"weekday"`
	Time       string `json:"time"`
	Reason     string `json:"reason"`
}

func (s *StudyApp) permanentRescheduleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireStudentJSON(w, r)
	if session == nil {
		return
	}
	var req permanentRescheduleRequestJSON
	if !s.decodeJSON(w, r, &req, 12<<10) {
		return
	}
	if len([]rune(req.Reason)) > 1000 {
		writeJSONError(w, "комментарий слишком длинный", http.StatusBadRequest)
		return
	}
	_, err := requestPermanentReschedule(s.db, session.User, strings.TrimSpace(req.ScheduleID), req.Weekday, strings.TrimSpace(req.Time), req.Reason, s.loc, s.schedule)
	if err != nil {
		switch {
		case errors.Is(err, errRescheduleAlreadyPending):
			writeJSONError(w, "запрос на постоянный перенос уже ожидает решения", http.StatusConflict)
		case errors.Is(err, errStudyRecurringSlotBusy):
			writeJSONError(w, "этот постоянный слот уже занят", http.StatusConflict)
		default:
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "Запрос отправлен преподавателю"}, http.StatusCreated)
}

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

func (s *StudyApp) requireStudentJSON(w http.ResponseWriter, r *http.Request) *UserSession {
	session := s.session(r)
	if session == nil {
		writeJSONError(w, "для этого действия необходимо войти", http.StatusUnauthorized)
		return nil
	}
	if session.User.Role != "student" {
		writeJSONError(w, "действие доступно только студенту", http.StatusForbidden)
		return nil
	}
	if !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "csrf validation failed", http.StatusForbidden)
		return nil
	}
	return session
}

func (s *StudyApp) decodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONError(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONError(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *StudyApp) requireAdmin(w http.ResponseWriter, r *http.Request) *UserSession {
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return nil
	}
	if session.User.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil
	}
	return session
}

func userID(user *StudyUser) string {
	if user == nil {
		return ""
	}
	return user.ID
}

func normalizeCabinetTab(raw, role string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	allowed := map[string]bool{}
	if role == "admin" {
		allowed = map[string]bool{"overview": true, "schedule": true, "transfers": true, "students": true}
	} else {
		allowed = map[string]bool{"overview": true, "schedule": true, "materials": true, "transfers": true, "security": true}
	}
	if allowed[raw] {
		return raw
	}
	return "overview"
}

func dashboardPath(role string) string {
	if role == "admin" {
		return "/study/admin"
	}
	return "/study/account"
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	methodNotAllowed(w, methods...)
	return false
}

func allowMethodJSON(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func writeJSON(w http.ResponseWriter, payload any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json response: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	writeJSON(w, map[string]any{"ok": false, "error": message}, status)
}
