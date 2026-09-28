package app

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

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
	lessons, lessonErr := listPublishedLessonsForStudent(s.db, session.User.ID, s.loc)
	if lessonErr != nil {
		log.Printf("account lessons: %v", lessonErr)
	}
	assignments, assignmentErr := listStudentTestAssignments(s.db, session.User.ID, s.loc)
	if assignmentErr != nil {
		log.Printf("account tests: %v", assignmentErr)
	}
	bookings, lessons = attachTestsToStudentData(bookings, lessons, assignments)
	data.Bookings = bookings
	data.Lessons = lessons
	data.TestAssignments = assignments
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
