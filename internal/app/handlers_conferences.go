package app

import (
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *StudyApp) populateAdminLessonConference(data *StudyPageData, bookingID string) {
	if data == nil {
		return
	}
	data.JitsiEnabled = s.jitsi.Enabled()
	data.JitsiRecordingEnabled = s.jitsi.RecordingEnabled()
	data.JitsiPublicURL = s.jitsi.PublicURL
	data.JitsiDomain = s.jitsi.Domain
	if !data.JitsiEnabled {
		return
	}
	conference, _ := getLessonConference(s.db, bookingID, s.loc)
	data.Conference = conference
	if conference.RoomName != "" {
		data.Lesson.ConferenceReady = true
		data.Lesson.ConferenceRoom = conference.RoomName
	}
	if err := s.syncJibriRecordingsForBooking(bookingID); err != nil {
		// A recording-directory problem must not make the lesson page unavailable.
		log.Printf("sync Jibri recordings for %s: %v", bookingID, err)
	}
	data.Recordings, _ = listLessonRecordings(s.db, bookingID, true, s.loc)
}

func (s *StudyApp) adminConferenceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !s.jitsi.Enabled() {
		http.Error(w, "Jitsi не настроен на сервере", http.StatusServiceUnavailable)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	if _, err := createLessonConference(s.db, session.User.ID, bookingID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=conference", http.StatusSeeOther)
}

func (s *StudyApp) adminConferenceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	if err := deleteLessonConference(s.db, session.User.ID, bookingID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=conference-deleted", http.StatusSeeOther)
}

func (s *StudyApp) conferencePage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	bookingID := strings.TrimSpace(r.URL.Query().Get("id"))
	lesson, err := getAdminLesson(s.db, bookingID, s.loc)
	if err != nil || lesson.BookingID == "" {
		http.NotFound(w, r)
		return
	}
	moderator := session.User.Role == "admin"
	if !moderator && (session.User.Role != "student" || lesson.UserID != session.User.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if lesson.Status == "cancelled" || !lesson.ConferenceReady || lesson.ConferenceRoom == "" {
		http.NotFound(w, r)
		return
	}
	if !s.jitsi.Enabled() {
		http.Error(w, "Jitsi временно не настроен", http.StatusServiceUnavailable)
		return
	}
	jwt, err := s.jitsi.token(session.User, lesson.ConferenceRoom, moderator, time.Now().UTC())
	if err != nil {
		http.Error(w, "не удалось создать токен конференции", http.StatusInternalServerError)
		return
	}
	_ = s.syncJibriRecordingsForBooking(bookingID)
	recordings, _ := listLessonRecordings(s.db, bookingID, moderator, s.loc)
	data := s.baseData(w, r, "Конференция — MY.STUDY", "conference")
	data.CabinetTab = "schedule"
	data.Lesson = lesson
	data.Recordings = recordings
	data.JitsiEnabled = true
	data.JitsiRecordingEnabled = s.jitsi.RecordingEnabled()
	data.JitsiPublicURL = s.jitsi.PublicURL
	data.JitsiDomain = s.jitsi.Domain
	data.JitsiRoom = lesson.ConferenceRoom
	data.JitsiJWT = jwt
	data.ConferenceModerator = moderator
	renderStudy(w, "study_conference.html", data)
}

func (s *StudyApp) adminRecordingVisibility(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	recordingID := strings.TrimSpace(r.FormValue("recording_id"))
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	recording, _, err := getLessonRecording(s.db, recordingID, s.loc)
	if err != nil || recording.BookingID != bookingID {
		http.Error(w, "запись не найдена", http.StatusNotFound)
		return
	}
	if err := setLessonRecordingVisibility(s.db, recordingID, r.FormValue("visible") == "1"); err != nil {
		http.Error(w, "не удалось изменить доступ", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=recording", http.StatusSeeOther)
}

func (s *StudyApp) adminRecordingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	recordingID := strings.TrimSpace(r.FormValue("recording_id"))
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	recording, _, err := getLessonRecording(s.db, recordingID, s.loc)
	if err != nil || recording.BookingID != bookingID {
		http.Error(w, "запись не найдена", http.StatusNotFound)
		return
	}
	if err := s.removeRecordingFiles(recording.RelativePath); err != nil {
		http.Error(w, "не удалось удалить файл записи: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := deleteLessonRecordingDB(s.db, recordingID); err != nil {
		http.Error(w, "не удалось удалить запись из базы", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=recording-deleted", http.StatusSeeOther)
}

func (s *StudyApp) lessonRecordingFile(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	recording, ownerID, err := getLessonRecording(s.db, strings.TrimSpace(r.URL.Query().Get("id")), s.loc)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if session.User.Role != "admin" && (session.User.Role != "student" || ownerID != session.User.ID || !recording.StudentVisible) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path, err := s.recordingAbsolutePath(recording.RelativePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	contentType := recording.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": recording.FileName}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("Cache-Control", "private, no-store")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	http.ServeContent(w, r, recording.FileName, info.ModTime(), file)
}
