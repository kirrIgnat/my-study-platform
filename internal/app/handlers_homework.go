package app

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func decorateHomeworkAssignment(assignment *HomeworkAssignment, loc *time.Location) {
	if assignment == nil {
		return
	}
	if loc == nil {
		loc = time.UTC
	}
	assignment.HomeworkDir = "homework/hw_" + assignment.AssignedAt.In(loc).Format("02012006")
	language := defaultHomeworkCodeLanguage(assignment.Task.Language)
	if assignment.Submission != nil && assignment.Submission.Language != "" {
		if submittedLanguage, err := normalizeHomeworkCodeLanguage(assignment.Submission.Language); err == nil {
			language = submittedLanguage
		}
	}
	assignment.CodeLanguage = language
	assignment.SourceFilename = assignment.Task.PublicID + homeworkSourceExtension(language)
	if assignment.Submission != nil {
		assignment.Submission.Filename = assignment.Task.PublicID + homeworkSourceExtension(assignment.Submission.Language)
	}
}

func (s *StudyApp) populateAdminLessonHomework(data *StudyPageData, bookingID, userID string) {
	data.HomeworkTaskChoices, _ = listHomeworkTaskChoicesForStudent(s.db, userID, bookingID)
	data.HomeworkAssignments, _ = listHomeworkAssignmentsForBooking(s.db, bookingID)
	for i := range data.HomeworkAssignments {
		decorateHomeworkAssignment(&data.HomeworkAssignments[i], s.loc)
	}
	data.Lesson.HomeworkAssignments = data.HomeworkAssignments
}

func (s *StudyApp) adminHomeworkBank(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	if s.requireAdmin(w, r) == nil {
		return
	}
	data := s.baseData(w, r, "База задач — MY.STUDY", "admin")
	data.CabinetTab = "homework"
	data.HomeworkTasks, _ = listHomeworkTasks(s.db, true)
	if editID := strings.TrimSpace(r.URL.Query().Get("edit")); editID != "" {
		if task, err := getHomeworkTask(s.db, editID); err == nil {
			data.HomeworkTask = task
		}
	}
	switch r.URL.Query().Get("saved") {
	case "created":
		data.Success = "Задача добавлена в базу. Теперь её можно назначать ученикам из карточки занятия."
	case "updated":
		data.Success = "Задача обновлена. Изменения видны во всех назначениях этой задачи."
	}
	renderStudy(w, "study_admin_homework.html", data)
}

func (s *StudyApp) adminHomeworkTaskSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	taskID := strings.TrimSpace(r.FormValue("task_id"))
	_, err := saveHomeworkTask(s.db, session.User.ID, taskID, r.FormValue("public_id"), r.FormValue("title"), r.FormValue("topic"), r.FormValue("language"), r.FormValue("difficulty"), r.FormValue("statement_md"), r.FormValue("input_md"), r.FormValue("output_md"), r.FormValue("examples_md"))
	if err != nil {
		data := s.baseData(w, r, "База задач — MY.STUDY", "admin")
		data.CabinetTab = "homework"
		data.Error = err.Error()
		data.HomeworkTasks, _ = listHomeworkTasks(s.db, true)
		data.HomeworkTask = HomeworkTask{ID: taskID, PublicID: r.FormValue("public_id"), Title: r.FormValue("title"), Topic: r.FormValue("topic"), Language: r.FormValue("language"), Difficulty: r.FormValue("difficulty"), StatementMD: r.FormValue("statement_md"), InputMD: r.FormValue("input_md"), OutputMD: r.FormValue("output_md"), ExamplesMD: r.FormValue("examples_md"), Active: true}
		renderStudy(w, "study_admin_homework.html", data)
		return
	}
	state := "created"
	if taskID != "" {
		state = "updated"
	}
	http.Redirect(w, r, "/study/admin/homework?saved="+state, http.StatusSeeOther)
}

func (s *StudyApp) adminHomeworkAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	userID := strings.TrimSpace(r.FormValue("student_id"))
	if _, err := assignHomeworkTasks(s.db, session.User.ID, userID, bookingID, r.Form["task_id"]); err != nil {
		lesson, lessonErr := getAdminLesson(s.db, bookingID, s.loc)
		if lessonErr != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data := s.baseData(w, r, "Занятие — MY.STUDY", "admin")
		data.CabinetTab = "schedule"
		data.Error = err.Error()
		data.Lesson = lesson
		s.populateAdminLessonTests(&data, bookingID)
		s.populateAdminLessonHomework(&data, bookingID, lesson.UserID)
		renderStudy(w, "study_admin_lesson.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=homework", http.StatusSeeOther)
}

func (s *StudyApp) adminHomeworkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := updateHomeworkAssignmentStatus(s.db, session.User.ID, strings.TrimSpace(r.FormValue("assignment_id")), strings.TrimSpace(r.FormValue("status")), r.FormValue("teacher_note")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	http.Redirect(w, r, "/study/admin/lesson?id="+bookingID+"&saved=homework-status", http.StatusSeeOther)
}

func (s *StudyApp) studentHomeworkView(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role != "student" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	assignment, err := getHomeworkAssignmentForStudent(s.db, strings.TrimSpace(r.URL.Query().Get("id")), session.User.ID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := s.baseData(w, r, assignment.Task.Title+" — MY.STUDY", "account")
	decorateHomeworkAssignment(&assignment, s.loc)
	data.CabinetTab = "materials"
	data.HomeworkAssignment = assignment
	if r.URL.Query().Get("sent") == "1" {
		data.Success = "Код отправлен преподавателю на проверку."
	}
	renderStudy(w, "study_homework.html", data)
}

func (s *StudyApp) studentHomeworkSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role != "student" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 300<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	assignmentID := strings.TrimSpace(r.FormValue("assignment_id"))
	language := r.FormValue("language")
	sourceCode := r.FormValue("source_code")
	if err := submitHomeworkSolution(s.db, session.User.ID, assignmentID, language, sourceCode); err != nil {
		assignment, loadErr := getHomeworkAssignmentForStudent(s.db, assignmentID, session.User.ID)
		if loadErr != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		assignment.Submission = &HomeworkSubmission{Language: language, SourceCode: sourceCode}
		decorateHomeworkAssignment(&assignment, s.loc)
		data := s.baseData(w, r, assignment.Task.Title+" — MY.STUDY", "account")
		data.CabinetTab = "materials"
		data.HomeworkAssignment = assignment
		data.Error = err.Error()
		renderStudy(w, "study_homework.html", data)
		return
	}
	http.Redirect(w, r, "/study/homework?id="+assignmentID+"&sent=1", http.StatusSeeOther)
}

func readMarkdownUpload(r *http.Request, field string, maxBytes int64) (string, bool, error) {
	file, header, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, multipart.ErrMessageTooLarge) {
		if errors.Is(err, multipart.ErrMessageTooLarge) {
			return "", false, errors.New("Markdown-файл слишком большой")
		}
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".md" && ext != ".markdown" && ext != ".txt" {
		return "", false, errors.New("можно загрузить только .md, .markdown или .txt")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", false, err
	}
	if int64(len(data)) > maxBytes {
		return "", false, errors.New("Markdown-файл слишком большой")
	}
	return string(data), true, nil
}

type adminHomeworkAssignAPIRequest struct {
	UserID    string   `json:"user_id"`
	BookingID string   `json:"booking_id"`
	TaskIDs   []string `json:"task_ids"`
}

func (s *StudyApp) adminHomeworkAssignAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "csrf validation failed", http.StatusForbidden)
		return
	}
	var req adminHomeworkAssignAPIRequest
	if !s.decodeJSON(w, r, &req, 64<<10) {
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.BookingID = strings.TrimSpace(req.BookingID)
	if req.UserID == "" || req.BookingID == "" {
		writeJSONError(w, "не выбран ученик или занятие", http.StatusBadRequest)
		return
	}
	assigned, err := assignHomeworkTasks(s.db, session.User.ID, req.UserID, req.BookingID, req.TaskIDs)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "assigned": assigned}, http.StatusOK)
}

type adminHomeworkStatusAPIRequest struct {
	AssignmentID string `json:"assignment_id"`
	Status       string `json:"status"`
	TeacherNote  string `json:"teacher_note"`
}

func (s *StudyApp) adminHomeworkStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		writeJSONError(w, "csrf validation failed", http.StatusForbidden)
		return
	}
	var req adminHomeworkStatusAPIRequest
	if !s.decodeJSON(w, r, &req, 16<<10) {
		return
	}
	if err := updateHomeworkAssignmentStatus(s.db, session.User.ID, strings.TrimSpace(req.AssignmentID), strings.TrimSpace(req.Status), req.TeacherNote); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true}, http.StatusOK)
}
