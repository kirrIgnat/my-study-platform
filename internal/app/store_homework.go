package app

import (
	"errors"
	"fmt"
	"sort"
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

func homeworkTaskFromRow(row []string) HomeworkTask {
	task := HomeworkTask{
		ID:              row[0],
		Title:           row[1],
		Topic:           row[2],
		Language:        row[3],
		Difficulty:      row[4],
		DifficultyLabel: homeworkDifficultyLabel(row[4]),
		StatementMD:     row[5],
		InputMD:         row[6],
		OutputMD:        row[7],
		ExamplesMD:      row[8],
		Active:          row[9] == "1",
		CreatedAt:       time.Unix(parseInt64(row[10]), 0).UTC(),
		UpdatedAt:       time.Unix(parseInt64(row[11]), 0).UTC(),
	}
	task.StatementHTML = renderStudyMarkdown(task.StatementMD)
	task.InputHTML = renderStudyMarkdown(task.InputMD)
	task.OutputHTML = renderStudyMarkdown(task.OutputMD)
	task.ExamplesHTML = renderStudyMarkdown(task.ExamplesMD)
	return task
}

func listHomeworkTasks(db *AppDB, includeArchived bool) ([]HomeworkTask, error) {
	query := `SELECT id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_at,updated_at FROM homework_tasks`
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
	rows, err := db.Query(`SELECT id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_at,updated_at FROM homework_tasks WHERE id=? LIMIT 1`, taskID)
	if err != nil {
		return HomeworkTask{}, err
	}
	if len(rows) == 0 {
		return HomeworkTask{}, errors.New("задача не найдена")
	}
	return homeworkTaskFromRow(rows[0]), nil
}

func saveHomeworkTask(db *AppDB, adminID, taskID, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD string) (string, error) {
	title = strings.TrimSpace(title)
	topic = strings.TrimSpace(topic)
	language = strings.TrimSpace(language)
	difficulty = strings.ToLower(strings.TrimSpace(difficulty))
	statementMD = strings.TrimSpace(statementMD)
	inputMD = strings.TrimSpace(inputMD)
	outputMD = strings.TrimSpace(outputMD)
	examplesMD = strings.TrimSpace(examplesMD)
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
		var err error
		taskID, err = randomToken(12)
		if err != nil {
			return "", err
		}
		if err := db.Exec(`INSERT INTO homework_tasks(id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_by,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,1,?,?,?)`, taskID, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD, adminID, now, now); err != nil {
			return "", err
		}
		return taskID, nil
	}
	if err := db.Exec(`UPDATE homework_tasks SET title=?,topic=?,language=?,difficulty=?,statement_md=?,input_md=?,output_md=?,examples_md=?,updated_at=? WHERE id=?`, title, topic, language, difficulty, statementMD, inputMD, outputMD, examplesMD, now, taskID); err != nil {
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

func homeworkAssignmentFromRow(row []string) HomeworkAssignment {
	assignment := HomeworkAssignment{
		ID: row[0], TaskID: row[1], UserID: row[2], BookingID: row[3], AssignedBy: row[4], Status: row[5],
		StatusLabel: homeworkStatusLabel(row[5]), TeacherNote: row[6], AssignedAt: time.Unix(parseInt64(row[7]), 0).UTC(), UpdatedAt: time.Unix(parseInt64(row[8]), 0).UTC(),
	}
	assignment.Task = homeworkTaskFromRow(row[9:21])
	return assignment
}

const homeworkAssignmentSelect = `SELECT a.id,a.task_id,a.user_id,COALESCE(a.booking_id,''),COALESCE(a.assigned_by,''),a.status,a.teacher_note,a.assigned_at,a.updated_at,
	t.id,t.title,t.topic,t.language,t.difficulty,t.statement_md,t.input_md,t.output_md,t.examples_md,t.active,t.created_at,t.updated_at
	FROM homework_assignments a JOIN homework_tasks t ON t.id=a.task_id`

func listHomeworkAssignmentsForStudent(db *AppDB, userID string) ([]HomeworkAssignment, error) {
	rows, err := db.Query(homeworkAssignmentSelect+` WHERE a.user_id=? ORDER BY a.assigned_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]HomeworkAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, homeworkAssignmentFromRow(row))
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
	return homeworkAssignmentFromRow(rows[0]), nil
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
		if rows[0][3] == status {
			return nil
		}
		title := "Статус домашней задачи изменён"
		body := fmt.Sprintf("Задача «%s»: %s.", rows[0][2], homeworkStatusLabel(status))
		if status == "correct" {
			title = "Задача зачтена"
		} else if status == "incorrect" {
			title = "Задачу нужно повторить"
			body = fmt.Sprintf("В задаче «%s» есть ошибка. Её можно решить повторно.", rows[0][2])
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
