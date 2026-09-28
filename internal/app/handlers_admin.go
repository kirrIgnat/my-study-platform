package app

import (
	"net/http"
	"strings"
	"time"
)

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
	switch r.URL.Query().Get("status") {
	case "resolved":
		data.Success = "Запрос на перенос обработан. Ученик получил уведомление."
	case "cancelled":
		data.Success = "Занятие отменено. Ученик получил уведомление."
	case "deleted":
		data.Success = "Занятие удалено из расписания. Ученик получил уведомление."
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
	if err := updateBookingStatus(s.db, session.User.ID, strings.TrimSpace(r.FormValue("booking_id")), strings.TrimSpace(r.FormValue("status"))); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	redirect := "/study/admin?tab=schedule"
	if strings.TrimSpace(r.FormValue("status")) == "cancelled" {
		redirect += "&status=cancelled"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
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
	if err := updateBookingAdmin(s.db, session.User.ID, bookingID, strings.TrimSpace(r.FormValue("status")), r.FormValue("meeting_url"), r.FormValue("date"), r.FormValue("time"), s.loc, s.schedule); err != nil {
		data := s.baseData(w, r, "Занятие — KIRILL.STUDY", "admin")
		data.CabinetTab = "schedule"
		data.Error = err.Error()
		data.Lesson, _ = getAdminLesson(s.db, bookingID, s.loc)
		s.populateAdminLessonTests(&data, bookingID)
		renderStudy(w, "study_admin_lesson.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=schedule", http.StatusSeeOther)
}

func (s *StudyApp) adminBookingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := deleteBookingAdmin(s.db, session.User.ID, strings.TrimSpace(r.FormValue("booking_id"))); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/study/admin?tab=schedule&status=deleted", http.StatusSeeOther)
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
	s.populateAdminLessonTests(&data, bookingID)
	switch r.URL.Query().Get("saved") {
	case "schedule":
		data.Success = "Параметры занятия сохранены. Ученик получил уведомление об изменениях."
	case "materials":
		data.Success = "Материалы сохранены как черновик."
	case "published":
		data.Success = "Материалы опубликованы. Ученик получил уведомление о новом ДЗ/материалах."
	case "test":
		data.Success = "Тест прикреплён к занятию. Ученик получил уведомление."
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
		s.populateAdminLessonTests(&data, bookingID)
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
