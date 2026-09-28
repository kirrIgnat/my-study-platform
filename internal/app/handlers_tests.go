package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *StudyApp) adminTestsData(w http.ResponseWriter, r *http.Request) StudyPageData {
	data := s.baseData(w, r, "Тесты — KIRILL.STUDY", "admin")
	data.CabinetTab = "tests"
	data.TestTemplates, _ = listTestTemplates(s.db)
	data.Students, _ = listStudents(s.db)
	data.TestAssignments, _ = listAdminTestAssignments(s.db, s.loc)
	return data
}

func (s *StudyApp) adminTests(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) || s.requireAdmin(w, r) == nil {
		return
	}
	data := s.adminTestsData(w, r)
	switch r.URL.Query().Get("status") {
	case "assigned":
		data.Success = "Тест выдан ученику. В кабинете появилось новое задание."
	}
	renderStudy(w, "study_admin_tests.html", data)
}

func (s *StudyApp) adminTestView(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) || s.requireAdmin(w, r) == nil {
		return
	}
	test, err := getTestTemplate(s.db, strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := s.baseData(w, r, test.Title+" — KIRILL.STUDY", "admin")
	data.CabinetTab = "tests"
	data.TestTemplate = test
	data.Students, _ = listStudents(s.db)
	data.TestQuestions = makeTestQuestionViews(test.Questions, map[string][]string{}, false)
	renderStudy(w, "study_admin_test.html", data)
}

func (s *StudyApp) adminTestAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	testID := strings.TrimSpace(r.FormValue("test_id"))
	studentID := strings.TrimSpace(r.FormValue("student_id"))
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	attachMode := strings.TrimSpace(r.FormValue("attach_mode"))
	timeLimit, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("time_limit_minutes")))
	var deadline time.Time
	if raw := strings.TrimSpace(r.FormValue("deadline")); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", raw, s.loc)
		if err != nil {
			s.renderTestAssignError(w, r, bookingID, "Некорректный дедлайн.")
			return
		}
		deadline = parsed
	}
	if _, err := assignTest(s.db, session.User.ID, testID, studentID, bookingID, attachMode, deadline, timeLimit, s.loc); err != nil {
		s.renderTestAssignError(w, r, bookingID, err.Error())
		return
	}
	if bookingID != "" {
		http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=test", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/study/admin/tests?status=assigned", http.StatusSeeOther)
}

func (s *StudyApp) renderTestAssignError(w http.ResponseWriter, r *http.Request, bookingID, message string) {
	if bookingID != "" {
		lesson, err := getAdminLesson(s.db, bookingID, s.loc)
		if err == nil {
			data := s.baseData(w, r, "Занятие — KIRILL.STUDY", "admin")
			data.CabinetTab = "schedule"
			data.Lesson = lesson
			data.Error = message
			data.TestTemplates, _ = listTestTemplates(s.db)
			data.TestAssignments, _ = listTestAssignmentsForBooking(s.db, bookingID, s.loc)
			renderStudy(w, "study_admin_lesson.html", data)
			return
		}
	}
	data := s.adminTestsData(w, r)
	data.Error = message
	renderStudy(w, "study_admin_tests.html", data)
}

func (s *StudyApp) adminTestResult(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) || s.requireAdmin(w, r) == nil {
		return
	}
	assignment, err := getTestAssignment(s.db, strings.TrimSpace(r.URL.Query().Get("id")), s.loc)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	test, testErr := getTestTemplate(s.db, assignment.TestID)
	if testErr != nil {
		http.Error(w, "test unavailable", http.StatusInternalServerError)
		return
	}
	attempt, err := getTestAttemptByAssignment(s.db, assignment.ID)
	if err != nil || attempt.ID == "" {
		data := s.baseData(w, r, "Результат теста — KIRILL.STUDY", "admin")
		data.CabinetTab = "tests"
		data.TestAdminView = true
		data.TestAssignment = assignment
		data.TestTemplate = test
		data.Error = "Ученик ещё не начал тест."
		renderStudy(w, "study_test.html", data)
		return
	}
	if attempt.Status == "in_progress" && !attempt.ExpiresAt.IsZero() && !time.Now().UTC().Before(attempt.ExpiresAt) {
		attempt, _ = finalizeTestAttempt(s.db, attempt.ID, "expired")
		assignment, _ = getTestAssignment(s.db, assignment.ID, s.loc)
	}
	data := s.baseData(w, r, "Результат — "+test.Title, "admin")
	data.CabinetTab = "tests"
	data.TestAdminView = true
	data.TestTemplate = test
	data.TestAssignment = assignment
	data.TestAttempt = attempt
	data.TestStarted = true
	data.TestFinished = attempt.Status == "submitted" || attempt.Status == "expired"
	if data.TestFinished {
		data.TestQuestions = makeTestQuestionViews(test.Questions, attempt.Answers, true)
		data.TestBreakdown = testBreakdown(test.Questions, attempt.Answers)
	}
	renderStudy(w, "study_test.html", data)
}

func (s *StudyApp) studyTest(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role == "admin" {
		http.Redirect(w, r, "/study/admin/tests", http.StatusSeeOther)
		return
	}
	assignment, err := s.studentAssignment(r, session)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	test, err := getTestTemplate(s.db, assignment.TestID)
	if err != nil {
		http.Error(w, "test unavailable", http.StatusInternalServerError)
		return
	}
	attempt, err := getTestAttemptByAssignment(s.db, assignment.ID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if attempt.ID != "" && attempt.Status == "in_progress" && !attempt.ExpiresAt.IsZero() && !time.Now().UTC().Before(attempt.ExpiresAt) {
		attempt, _ = finalizeTestAttempt(s.db, attempt.ID, "expired")
		assignment, _ = getTestAssignment(s.db, assignment.ID, s.loc)
	}

	data := s.baseData(w, r, test.Title+" — KIRILL.STUDY", "account")
	data.CabinetTab = "materials"
	data.TestTemplate = test
	data.TestAssignment = assignment
	data.TestAttempt = attempt
	data.TestStarted = attempt.ID != ""
	data.TestFinished = attempt.Status == "submitted" || attempt.Status == "expired"
	if data.TestStarted {
		data.TestQuestions = makeTestQuestionViews(test.Questions, attempt.Answers, data.TestFinished)
	}
	if data.TestFinished {
		data.TestBreakdown = testBreakdown(test.Questions, attempt.Answers)
	}
	if !data.TestStarted && assignment.HasDeadline && !time.Now().In(s.loc).Before(assignment.DeadlineAt) {
		data.Error = "Дедлайн задания уже истёк."
	}
	renderStudy(w, "study_test.html", data)
}

func (s *StudyApp) studentAssignment(r *http.Request, session *UserSession) (TestAssignment, error) {
	assignment, err := getTestAssignment(s.db, strings.TrimSpace(r.FormValue("assignment_id")), s.loc)
	if err != nil {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			return TestAssignment{}, err
		}
		assignment, err = getTestAssignment(s.db, id, s.loc)
	}
	if err != nil || assignment.UserID != session.User.ID || assignment.Status != "active" {
		return TestAssignment{}, errors.New("задание недоступно")
	}
	return assignment, nil
}

func (s *StudyApp) studyTestStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || session.User.Role != "student" {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	assignment, err := s.studentAssignment(r, session)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := startTestAttempt(s.db, assignment); err != nil {
		data := s.baseData(w, r, assignment.TestTitle+" — KIRILL.STUDY", "account")
		data.CabinetTab = "materials"
		data.TestAssignment = assignment
		data.TestTemplate, _ = getTestTemplate(s.db, assignment.TestID)
		data.Error = err.Error()
		renderStudy(w, "study_test.html", data)
		return
	}
	http.Redirect(w, r, "/study/test?id="+assignment.ID, http.StatusSeeOther)
}

func (s *StudyApp) studyTestSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || session.User.Role != "student" {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "forbidden", http.StatusForbidden)
		return
	}
	assignment, err := s.studentAssignment(r, session)
	if err != nil {
		writeJSONError(w, "задание недоступно", http.StatusNotFound)
		return
	}
	test, err := getTestTemplate(s.db, assignment.TestID)
	if err != nil {
		writeJSONError(w, "тест недоступен", http.StatusInternalServerError)
		return
	}
	attempt, err := saveTestAnswers(s.db, assignment, answersFromForm(r, test.Questions))
	if err != nil {
		status := http.StatusBadRequest
		if attempt.Status == "expired" || attempt.Status == "submitted" {
			status = http.StatusConflict
		}
		writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "status": attempt.Status}, status)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "status": attempt.Status, "saved_at": attempt.UpdatedAt.Unix()}, http.StatusOK)
}

func (s *StudyApp) studyTestSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil || session.User.Role != "student" {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	assignment, err := s.studentAssignment(r, session)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	test, err := getTestTemplate(s.db, assignment.TestID)
	if err != nil {
		http.Error(w, "test unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := submitTestAttempt(s.db, assignment, answersFromForm(r, test.Questions)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/study/test?id="+assignment.ID, http.StatusSeeOther)
}

func answersFromForm(r *http.Request, questions []TestQuestion) map[string][]string {
	answers := make(map[string][]string, len(questions))
	for _, q := range questions {
		values := append([]string(nil), r.Form["q_"+q.ID]...)
		if len(values) > 0 {
			answers[q.ID] = values
		}
	}
	return normalizeAnswers(answers)
}

func (s *StudyApp) populateAdminLessonTests(data *StudyPageData, bookingID string) {
	data.TestTemplates, _ = listTestTemplates(s.db)
	data.TestAssignments, _ = listTestAssignmentsForBooking(s.db, bookingID, s.loc)
}
