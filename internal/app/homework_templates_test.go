package app

import (
	"io"
	"testing"
	"time"
)

func TestHomeworkTemplatesRender(t *testing.T) {
	task := HomeworkTask{
		ID: "task-1", Title: "Два указателя", Topic: "Массивы", Language: "C", Difficulty: "medium", DifficultyLabel: "Средняя",
		StatementMD: "# Условие", StatementHTML: renderStudyMarkdown("# Условие"),
		InputMD: "Два числа.", InputHTML: renderStudyMarkdown("Два числа."),
		OutputMD: "Их сумма.", OutputHTML: renderStudyMarkdown("Их сумма."),
		ExamplesMD: "```text\n1 2 3\n```", ExamplesHTML: renderStudyMarkdown("```text\n1 2 3\n```"), Active: true,
	}
	assignment := HomeworkAssignment{
		ID: "assignment-1", TaskID: task.ID, UserID: "student-1", BookingID: "booking-1", Status: "incorrect", StatusLabel: "Была ошибка",
		AssignedAt: time.Now(), Task: task,
	}
	lesson := StudyLesson{
		BookingID: "booking-1", UserID: "student-1", UserName: "Student", UserEmail: "student@example.com", Date: "2026-09-28", Time: "18:00", EndTime: "19:30",
		Title: "Lesson", Theory: "# Notes", TheoryHTML: renderStudyMarkdown("# Notes"), HomeworkAssignments: []HomeworkAssignment{assignment},
	}
	user := &StudyUser{ID: "admin-1", Name: "Admin", Role: "admin"}
	adminData := StudyPageData{
		Title: "Admin", User: user, CSRF: "csrf", CabinetTab: "schedule", Lesson: lesson,
		HomeworkTaskChoices: []HomeworkTaskChoice{{Task: task, TimesAssigned: 1, LastStatus: "incorrect", LastStatusLabel: "Была ошибка"}},
		HomeworkAssignments: []HomeworkAssignment{assignment}, HomeworkTasks: []HomeworkTask{task}, HomeworkTask: task,
	}
	if err := templates.ExecuteTemplate(io.Discard, "study_admin_lesson.html", adminData); err != nil {
		t.Fatalf("render admin lesson: %v", err)
	}
	adminData.CabinetTab = "homework"
	if err := templates.ExecuteTemplate(io.Discard, "study_admin_homework.html", adminData); err != nil {
		t.Fatalf("render homework bank: %v", err)
	}

	student := &StudyUser{ID: "student-1", Name: "Student", Role: "student"}
	studentData := StudyPageData{Title: "Homework", User: student, CabinetTab: "materials", HomeworkAssignment: assignment, HomeworkAssignments: []HomeworkAssignment{assignment}, Lessons: []StudyLesson{lesson}}
	if err := templates.ExecuteTemplate(io.Discard, "study_homework.html", studentData); err != nil {
		t.Fatalf("render student homework: %v", err)
	}
	if err := templates.ExecuteTemplate(io.Discard, "study_account.html", studentData); err != nil {
		t.Fatalf("render student materials: %v", err)
	}
}
