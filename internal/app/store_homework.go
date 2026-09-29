package app

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func homeworkDifficultyLabel(value string) string {
	switch value {
	case "easy":
		return "Лёгкая"
	case "medium":
		return "Средняя"
	case "hard":
		return "Сложная"
	default:
		return value
	}
}

func homeworkStatusLabel(value string) string {
	switch value {
	case "assigned":
		return "Не проверена"
	case "correct":
		return "Решена верно"
	case "incorrect":
		return "Была ошибка"
	default:
		return value
	}
}

func homeworkPublicIDNumber(value string) (int, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if !strings.HasPrefix(value, "id_t") {
		return 0, false
	}
	raw := strings.TrimPrefix(value, "id_t")
	if raw == "" || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func normalizeHomeworkPublicID(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if _, ok := homeworkPublicIDNumber(value); !ok {
		return "", errors.New("ID задачи должен быть в формате id_t1, id_t2, id_t3 ...")
	}
	return value, nil
}

func nextHomeworkPublicID(db dbExecutor) (string, error) {
	rows, err := db.query(`SELECT public_id FROM homework_tasks WHERE public_id<>''`)
	if err != nil {
		return "", err
	}
	maxNumber := 0
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if n, ok := homeworkPublicIDNumber(row[0]); ok && n > maxNumber {
			maxNumber = n
		}
	}
	return fmt.Sprintf("id_t%d", maxNumber+1), nil
}

func backfillHomeworkPublicIDs(db dbExecutor) error {
	rows, err := db.query(`SELECT id,public_id FROM homework_tasks ORDER BY created_at,id`)
	if err != nil {
		return err
	}
	maxNumber := 0
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		if n, ok := homeworkPublicIDNumber(row[1]); ok && n > maxNumber {
			maxNumber = n
		}
	}
	for _, row := range rows {
		if len(row) < 2 || strings.TrimSpace(row[1]) != "" {
			continue
		}
		maxNumber++
		if err := db.exec(`UPDATE homework_tasks SET public_id=? WHERE id=? AND public_id=''`, fmt.Sprintf("id_t%d", maxNumber), row[0]); err != nil {
			return err
		}
	}
	return nil
}

func homeworkTaskFromRow(row []string) HomeworkTask {
	task := HomeworkTask{
		ID:              row[0],
		PublicID:        row[1],
		Title:           row[2],
		Topic:           row[3],
		Language:        row[4],
		Difficulty:      row[5],
		DifficultyLabel: homeworkDifficultyLabel(row[5]),
		StatementMD:     row[6],
		InputMD:         row[7],
		OutputMD:        row[8],
		ExamplesMD:      row[9],
		Active:          row[10] == "1",
		CreatedAt:       time.Unix(parseInt64(row[11]), 0).UTC(),
		UpdatedAt:       time.Unix(parseInt64(row[12]), 0).UTC(),
	}
	task.StatementHTML = renderStudyMarkdown(task.StatementMD)
	task.InputHTML = renderStudyMarkdown(task.InputMD)
	task.OutputHTML = renderStudyMarkdown(task.OutputMD)
	task.ExamplesHTML = renderStudyMarkdown(task.ExamplesMD)
	return task
}

func listHomeworkTasks(db *AppDB, includeArchived bool) ([]HomeworkTask, error) {
	query := `SELECT id,public_id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_at,updated_at FROM homework_tasks`
	if !includeArchived {
		query += ` WHERE active=1`
	}
	query += ` ORDER BY lower(topic), CASE difficulty WHEN 'easy' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, lower(title)`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	result := make([]HomeworkTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, homeworkTaskFromRow(row))
	}
	return result, nil
}

func getHomeworkTask(db *AppDB, taskID string) (HomeworkTask, error) {
	rows, err := db.Query(`SELECT id,public_id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_at,updated_at FROM homework_tasks WHERE id=? LIMIT 1`, taskID)
	if err != nil {
		return HomeworkTask{}, err
	}
	if len(rows) == 0 {
		return HomeworkTask{}, errors.New("задача не найдена")
	}
	return homeworkTaskFromRow(rows[0]), nil
}

func saveHomeworkTask(db *AppDB, adminID, taskID, publicID, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD string) (string, error) {
	title = strings.TrimSpace(title)
	topic = strings.TrimSpace(topic)
	language = strings.TrimSpace(language)
	difficulty = strings.ToLower(strings.TrimSpace(difficulty))
	statementMD = strings.TrimSpace(statementMD)
	inputMD = strings.TrimSpace(inputMD)
	outputMD = strings.TrimSpace(outputMD)
	examplesMD = strings.TrimSpace(examplesMD)
	publicID = strings.TrimSpace(publicID)
	if title == "" || topic == "" || language == "" || statementMD == "" {
		return "", errors.New("укажите название, тему, язык и условие задачи")
	}
	if difficulty != "easy" && difficulty != "medium" && difficulty != "hard" {
		return "", errors.New("некорректная сложность задачи")
	}
	if len([]rune(title)) > 180 || len([]rune(topic)) > 120 || len([]rune(language)) > 40 || len([]rune(statementMD)) > 50000 || len([]rune(inputMD)) > 12000 || len([]rune(outputMD)) > 12000 || len([]rune(examplesMD)) > 30000 {
		return "", errors.New("слишком большой объём данных задачи")
	}
	now := time.Now().UTC().Unix()
	if taskID == "" {
		var newTaskID string
		err := db.WithTx(func(tx *dbTx) error {
			if err := tx.exec(`LOCK TABLE homework_tasks IN SHARE ROW EXCLUSIVE MODE`); err != nil {
				return err
			}
			if publicID == "" {
				var err error
				publicID, err = nextHomeworkPublicID(tx)
				if err != nil {
					return err
				}
			} else {
				var err error
				publicID, err = normalizeHomeworkPublicID(publicID)
				if err != nil {
					return err
				}
			}
			var err error
			newTaskID, err = randomToken(12)
			if err != nil {
				return err
			}
			if err := tx.exec(`INSERT INTO homework_tasks(id,public_id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_by,created_at,updated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,1,?,?,?)`, newTaskID, publicID, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD, adminID, now, now); err != nil {
				if isDBConstraint(err) {
					return errors.New("такой ID задачи уже используется")
				}
				return err
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		return newTaskID, nil
	}

	var err error
	publicID, err = normalizeHomeworkPublicID(publicID)
	if err != nil {
		return "", err
	}
	rows, err := db.Query(`SELECT id FROM homework_tasks WHERE public_id=? AND id<>? LIMIT 1`, publicID, taskID)
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		return "", errors.New("такой ID задачи уже используется")
	}
	if err := db.Exec(`UPDATE homework_tasks SET public_id=?,title=?,topic=?,language=?,difficulty=?,statement_md=?,input_md=?,output_md=?,examples_md=?,updated_at=? WHERE id=?`, publicID, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD, now, taskID); err != nil {
		if isDBConstraint(err) {
			return "", errors.New("такой ID задачи уже используется")
		}
		return "", err
	}
	return taskID, nil
}

func setHomeworkTaskActive(db *AppDB, taskID string, active bool) error {
	value := 0
	if active {
		value = 1
	}
	return db.Exec(`UPDATE homework_tasks SET active=?,updated_at=? WHERE id=?`, value, time.Now().UTC().Unix(), taskID)
}

func homeworkTaskChoicesFromRows(tasks []HomeworkTask, rows [][]string, bookingID string) []HomeworkTaskChoice {
	type history struct {
		count              int
		everCorrect        bool
		hasPending         bool
		lastResolvedStatus string
		lastResolvedAt     int64
		thisLesson         bool
	}
	historyByTask := make(map[string]history)
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		h := historyByTask[row[0]]
		h.count++
		status := row[1]
		assignedAt := parseInt64(row[3])
		updatedAt := parseInt64(row[4])
		if updatedAt < assignedAt {
			updatedAt = assignedAt
		}
		switch status {
		case "correct":
			h.everCorrect = true
			if updatedAt >= h.lastResolvedAt {
				h.lastResolvedAt = updatedAt
				h.lastResolvedStatus = status
			}
		case "incorrect":
			if updatedAt >= h.lastResolvedAt {
				h.lastResolvedAt = updatedAt
				h.lastResolvedStatus = status
			}
		case "assigned":
			h.hasPending = true
			if bookingID != "" && row[2] == bookingID {
				h.thisLesson = true
			}
		}
		historyByTask[row[0]] = h
	}

	choices := make([]HomeworkTaskChoice, 0, len(tasks))
	for _, task := range tasks {
		h := historyByTask[task.ID]
		// Once a task is solved correctly it leaves this student's future homework bank.
		// A task that is still awaiting review is also hidden to prevent duplicate attempts.
		if h.everCorrect || h.hasPending {
			continue
		}
		choices = append(choices, HomeworkTaskChoice{
			Task: task, TimesAssigned: h.count, LastStatus: h.lastResolvedStatus,
			LastStatusLabel: homeworkStatusLabel(h.lastResolvedStatus), AlreadyThisLesson: h.thisLesson,
		})
	}
	return choices
}

func listHomeworkTaskChoicesForStudent(db *AppDB, userID, bookingID string) ([]HomeworkTaskChoice, error) {
	tasks, err := listHomeworkTasks(db, false)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT task_id,status,COALESCE(booking_id,''),assigned_at,updated_at FROM homework_assignments WHERE user_id=? ORDER BY assigned_at`, userID)
	if err != nil {
		return nil, err
	}
	return homeworkTaskChoicesFromRows(tasks, rows, bookingID), nil
}

func assignHomeworkTasks(db *AppDB, adminID, userID, bookingID string, taskIDs []string) (int, error) {
	if userID == "" || bookingID == "" || len(taskIDs) == 0 {
		return 0, errors.New("выберите хотя бы одну задачу")
	}
	unique := make([]string, 0, len(taskIDs))
	seen := make(map[string]bool)
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID != "" && !seen[taskID] {
			seen[taskID] = true
			unique = append(unique, taskID)
		}
	}
	if len(unique) == 0 {
		return 0, errors.New("выберите хотя бы одну задачу")
	}

	now := time.Now().UTC().Unix()
	assigned := 0
	var assignedTitles []string
	err := db.WithTx(func(tx *dbTx) error {
		bookingRows, err := tx.query(`SELECT user_id,date,time FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(bookingRows) == 0 || bookingRows[0][0] != userID {
			return errors.New("занятие ученика не найдено")
		}
		for _, taskID := range unique {
			taskRows, err := tx.query(`SELECT title FROM homework_tasks WHERE id=? AND active=1 LIMIT 1`, taskID)
			if err != nil {
				return err
			}
			if len(taskRows) == 0 {
				return errors.New("одна из выбранных задач недоступна")
			}
			historyRows, err := tx.query(`SELECT status FROM homework_assignments WHERE task_id=? AND user_id=?`, taskID, userID)
			if err != nil {
				return err
			}
			hasPending := false
			for _, historyRow := range historyRows {
				if len(historyRow) == 0 {
					continue
				}
				switch historyRow[0] {
				case "correct":
					return fmt.Errorf("задача «%s» уже решена учеником правильно", taskRows[0][0])
				case "assigned":
					hasPending = true
				}
			}
			if hasPending {
				return fmt.Errorf("задача «%s» уже выдана и ждёт проверки", taskRows[0][0])
			}
			id, err := randomToken(12)
			if err != nil {
				return err
			}
			if err := tx.exec(`INSERT INTO homework_assignments(id,task_id,user_id,booking_id,assigned_by,status,teacher_note,assigned_at,updated_at)
				VALUES(?,?,?,?,?,'assigned','',?,?)`, id, taskID, userID, bookingID, adminID, now, now); err != nil {
				return err
			}
			assigned++
			assignedTitles = append(assignedTitles, taskRows[0][0])
		}
		if assigned == 0 {
			return errors.New("выбранные задачи уже назначены на это занятие")
		}
		body := fmt.Sprintf("К занятию %s в %s назначено задач: %d.", bookingRows[0][1], bookingRows[0][2], assigned)
		if assigned == 1 {
			body = "Новое ДЗ: " + assignedTitles[0] + "."
		}
		return createNotificationTx(tx, userID, "homework_assigned", "Новое домашнее задание", body, bookingID, "", now)
	})
	return assigned, err
}

func normalizeHomeworkCodeLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "c":
		return "C", nil
	case "c++", "cpp", "cxx":
		return "C++", nil
	case "go", "golang":
		return "Go", nil
	case "python", "python3", "py":
		return "Python", nil
	default:
		return "", errors.New("выберите язык: C, C++, Go или Python")
	}
}

func defaultHomeworkCodeLanguage(taskLanguage string) string {
	language, err := normalizeHomeworkCodeLanguage(taskLanguage)
	if err == nil {
		return language
	}
	return "C"
}

func homeworkSourceExtension(language string) string {
	language, _ = normalizeHomeworkCodeLanguage(language)
	switch language {
	case "C++":
		return ".cpp"
	case "Go":
		return ".go"
	case "Python":
		return ".py"
	default:
		return ".c"
	}
}

func homeworkAssignmentFromRow(row []string) HomeworkAssignment {
	assignment := HomeworkAssignment{
		ID: row[0], TaskID: row[1], UserID: row[2], BookingID: row[3], AssignedBy: row[4], Status: row[5],
		StatusLabel: homeworkStatusLabel(row[5]), TeacherNote: row[6], AssignedAt: time.Unix(parseInt64(row[7]), 0).UTC(), UpdatedAt: time.Unix(parseInt64(row[8]), 0).UTC(),
	}
	assignment.Task = homeworkTaskFromRow(row[9:22])
	return assignment
}

const homeworkAssignmentSelect = `SELECT a.id,a.task_id,a.user_id,COALESCE(a.booking_id,''),COALESCE(a.assigned_by,''),a.status,a.teacher_note,a.assigned_at,a.updated_at,
	t.id,t.public_id,t.title,t.topic,t.language,t.difficulty,t.statement_md,t.input_md,t.output_md,t.examples_md,t.active,t.created_at,t.updated_at
	FROM homework_assignments a JOIN homework_tasks t ON t.id=a.task_id`

func homeworkSubmissionFromRow(row []string) HomeworkSubmission {
	submission := HomeworkSubmission{
		ID: row[0], AssignmentID: row[1], UserID: row[2], Language: row[3], SourceCode: row[4], Status: row[5], ResultText: row[6],
		CreatedAt: time.Unix(parseInt64(row[7]), 0).UTC(), UpdatedAt: time.Unix(parseInt64(row[8]), 0).UTC(),
	}
	if submittedAt := parseInt64(row[9]); submittedAt > 0 {
		submission.SubmittedAt = time.Unix(submittedAt, 0).UTC()
	}
	return submission
}

func attachLatestHomeworkSubmissions(db *AppDB, assignments []HomeworkAssignment, where string, arg string) error {
	if len(assignments) == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT s.id,s.assignment_id,s.user_id,s.language,s.source_code,s.status,s.result_text,s.created_at,s.updated_at,COALESCE(s.submitted_at,0)
		FROM homework_submissions s JOIN homework_assignments a ON a.id=s.assignment_id WHERE `+where+` ORDER BY s.created_at DESC`, arg)
	if err != nil {
		return err
	}
	byID := make(map[string]int, len(assignments))
	for i := range assignments {
		byID[assignments[i].ID] = i
	}
	for _, row := range rows {
		if len(row) < 10 {
			continue
		}
		idx, ok := byID[row[1]]
		if !ok || assignments[idx].Submission != nil {
			continue
		}
		submission := homeworkSubmissionFromRow(row)
		submission.Filename = assignments[idx].Task.PublicID + homeworkSourceExtension(submission.Language)
		assignments[idx].Submission = &submission
		if assignments[idx].Status == "assigned" && (submission.Status == "submitted" || submission.Status == "checking") {
			assignments[idx].StatusLabel = "На проверке"
		}
	}
	return nil
}

func listHomeworkAssignmentsForStudent(db *AppDB, userID string) ([]HomeworkAssignment, error) {
	rows, err := db.Query(homeworkAssignmentSelect+` WHERE a.user_id=? ORDER BY a.assigned_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]HomeworkAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, homeworkAssignmentFromRow(row))
	}
	if err := attachLatestHomeworkSubmissions(db, result, `a.user_id=?`, userID); err != nil {
		return nil, err
	}
	return result, nil
}

func listHomeworkAssignmentsForBooking(db *AppDB, bookingID string) ([]HomeworkAssignment, error) {
	rows, err := db.Query(homeworkAssignmentSelect+` WHERE a.booking_id=? ORDER BY a.assigned_at DESC`, bookingID)
	if err != nil {
		return nil, err
	}
	result := make([]HomeworkAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, homeworkAssignmentFromRow(row))
	}
	if err := attachLatestHomeworkSubmissions(db, result, `a.booking_id=?`, bookingID); err != nil {
		return nil, err
	}
	return result, nil
}

func getHomeworkAssignmentForStudent(db *AppDB, assignmentID, userID string) (HomeworkAssignment, error) {
	rows, err := db.Query(homeworkAssignmentSelect+` WHERE a.id=? AND a.user_id=? LIMIT 1`, assignmentID, userID)
	if err != nil {
		return HomeworkAssignment{}, err
	}
	if len(rows) == 0 {
		return HomeworkAssignment{}, errors.New("домашнее задание не найдено")
	}
	result := []HomeworkAssignment{homeworkAssignmentFromRow(rows[0])}
	if err := attachLatestHomeworkSubmissions(db, result, `a.id=?`, assignmentID); err != nil {
		return HomeworkAssignment{}, err
	}
	return result[0], nil
}

func submitHomeworkSolution(db *AppDB, userID, assignmentID, language, sourceCode string) error {
	assignmentID = strings.TrimSpace(assignmentID)
	if assignmentID == "" {
		return errors.New("домашнее задание не найдено")
	}
	var err error
	language, err = normalizeHomeworkCodeLanguage(language)
	if err != nil {
		return err
	}
	if strings.TrimSpace(sourceCode) == "" {
		return errors.New("вставьте код решения")
	}
	if len(sourceCode) > 250000 {
		return errors.New("код слишком большой: максимум 250 КБ")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT a.status,COALESCE(a.booking_id,''),t.title,t.public_id,u.name
			FROM homework_assignments a JOIN homework_tasks t ON t.id=a.task_id JOIN users u ON u.id=a.user_id
			WHERE a.id=? AND a.user_id=? LIMIT 1`, assignmentID, userID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("домашнее задание не найдено")
		}
		if rows[0][0] == "correct" {
			return errors.New("эта задача уже зачтена преподавателем")
		}
		submissionID, err := randomToken(12)
		if err != nil {
			return err
		}
		if err := tx.exec(`INSERT INTO homework_submissions(id,assignment_id,user_id,language,source_code,status,result_text,created_at,updated_at,submitted_at)
			VALUES(?,?,?,?,?,'submitted','',?,?,?)`, submissionID, assignmentID, userID, language, sourceCode, now, now, now); err != nil {
			return err
		}
		if err := tx.exec(`UPDATE homework_assignments SET status='assigned',updated_at=? WHERE id=?`, now, assignmentID); err != nil {
			return err
		}
		filename := rows[0][3] + homeworkSourceExtension(language)
		body := fmt.Sprintf("%s отправил решение задачи %s «%s» (%s, файл %s).", rows[0][4], rows[0][3], rows[0][2], language, filename)
		return notifyAdminsTx(tx, "homework_submitted", "Код отправлен на проверку", body, rows[0][1], "", now)
	})
}

func updateHomeworkAssignmentStatus(db *AppDB, adminID, assignmentID, status, note string) error {
	if status != "assigned" && status != "correct" && status != "incorrect" {
		return errors.New("некорректный статус задачи")
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 2000 {
		return errors.New("слишком длинный комментарий")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT a.user_id,COALESCE(a.booking_id,''),t.title,a.status FROM homework_assignments a JOIN homework_tasks t ON t.id=a.task_id WHERE a.id=? LIMIT 1`, assignmentID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("назначение не найдено")
		}
		if err := tx.exec(`UPDATE homework_assignments SET status=?,teacher_note=?,updated_at=? WHERE id=?`, status, note, now, assignmentID); err != nil {
			return err
		}
		if status == "correct" || status == "incorrect" {
			submissionStatus := "accepted"
			if status == "incorrect" {
				submissionStatus = "wrong_answer"
			}
			if err := tx.exec(`UPDATE homework_submissions SET status=?,result_text=?,updated_at=? WHERE id=(SELECT id FROM homework_submissions WHERE assignment_id=? ORDER BY created_at DESC LIMIT 1)`, submissionStatus, note, now, assignmentID); err != nil {
				return err
			}
		}
		if rows[0][3] == status {
			return nil
		}
		title := "Статус домашней задачи изменён"
		body := fmt.Sprintf("Задача «%s»: %s.", rows[0][2], homeworkStatusLabel(status))
		if status == "correct" {
			title = "Задача зачтена"
		} else if status == "incorrect" {
			title = "Задачу нужно повторить"
			body = fmt.Sprintf("В задаче «%s» есть ошибка. Исправьте код и отправьте решение повторно.", rows[0][2])
		}
		if note != "" {
			body += " Комментарий: " + note
		}
		return createNotificationTx(tx, rows[0][0], "homework_status", title, body, rows[0][1], "", now)
	})
}

// splitStudentHomeworkAssignments turns assignment history into the student's current task lists.
// A task that has been solved correctly is removed from active homework entirely, even if
// it has older incorrect attempts. For unfinished tasks only the latest assignment is shown.
func splitStudentHomeworkAssignments(assignments []HomeworkAssignment) (active, completed []HomeworkAssignment) {
	correctTasks := make(map[string]bool)
	completedSeen := make(map[string]bool)

	// listHomeworkAssignmentsForStudent returns newest assignments first. Keep the newest
	// correct assignment as the representative item in the completed archive.
	for _, assignment := range assignments {
		if assignment.Status != "correct" || completedSeen[assignment.TaskID] {
			continue
		}
		correctTasks[assignment.TaskID] = true
		completedSeen[assignment.TaskID] = true
		completed = append(completed, assignment)
	}

	activeSeen := make(map[string]bool)
	for _, assignment := range assignments {
		if correctTasks[assignment.TaskID] || activeSeen[assignment.TaskID] {
			continue
		}
		if assignment.Status != "assigned" && assignment.Status != "incorrect" {
			continue
		}
		activeSeen[assignment.TaskID] = true
		active = append(active, assignment)
	}
	return active, completed
}

func attachHomeworkAssignmentsToLessons(lessons []StudyLesson, assignments []HomeworkAssignment) []StudyLesson {
	byBooking := make(map[string][]HomeworkAssignment)
	for _, assignment := range assignments {
		if assignment.BookingID != "" {
			byBooking[assignment.BookingID] = append(byBooking[assignment.BookingID], assignment)
		}
	}
	for i := range lessons {
		lessons[i].HomeworkAssignments = byBooking[lessons[i].BookingID]
	}
	return lessons
}

func attachHomeworkAssignmentsToBookings(bookings []StudyBooking, assignments []HomeworkAssignment) []StudyBooking {
	byBooking := make(map[string][]HomeworkAssignment)
	for _, assignment := range assignments {
		if assignment.BookingID != "" {
			byBooking[assignment.BookingID] = append(byBooking[assignment.BookingID], assignment)
		}
	}
	for i := range bookings {
		bookings[i].HomeworkAssignments = byBooking[bookings[i].ID]
	}
	return bookings
}

func homeworkTopics(tasks []HomeworkTask) []string {
	seen := make(map[string]bool)
	var topics []string
	for _, task := range tasks {
		if !seen[task.Topic] {
			seen[task.Topic] = true
			topics = append(topics, task.Topic)
		}
	}
	sort.Strings(topics)
	return topics
}
