package app

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
)

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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		log.Printf("load teacher slots: %v", err)
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	availability, err := studyAvailability(s.db, s.loc, availableSchedule, days, studentID)
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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		log.Printf("load teacher slots: %v", err)
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	createdSchedule, err := createRecurringSchedule(s.db, session.User, req.Date, req.Time, req.Topic, s.defaultMeetingURL, s.loc, availableSchedule)
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
	writeJSON(w, map[string]any{"ok": true, "schedule": createdSchedule}, http.StatusCreated)
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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		writeJSONError(w, "не удалось загрузить слоты", http.StatusInternalServerError)
		return
	}
	options, err := listRecurringSlotOptions(s.db, availableSchedule, s.loc, session.User.ID, currentScheduleID)
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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	availability, err := rescheduleAvailability(s.db, s.loc, availableSchedule, session.User.ID, strings.TrimSpace(r.URL.Query().Get("booking_id")))
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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	_, err = requestTemporaryReschedule(s.db, session.User, strings.TrimSpace(req.BookingID), strings.TrimSpace(req.Date), strings.TrimSpace(req.Time), req.Reason, s.loc, availableSchedule)
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
	availableSchedule, err := s.currentBookingSchedule()
	if err != nil {
		writeJSONError(w, "не удалось загрузить расписание", http.StatusInternalServerError)
		return
	}
	_, err = requestPermanentReschedule(s.db, session.User, strings.TrimSpace(req.ScheduleID), req.Weekday, strings.TrimSpace(req.Time), req.Reason, s.loc, availableSchedule)
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
