package app

import (
	"errors"
	"time"
)

func seedCourses(db *AppDB) error {
	now := time.Now().UTC().Unix()
	courses := []StudyCourse{
		{ID: "course_go_backend", Slug: "go-backend", Title: "Go: от базы до backend", Description: "Язык, конкурентность, HTTP, PostgreSQL, тестирование и структура production-сервиса.", Level: "Beginner → Middle", Duration: "10 недель", Status: "open"},
		{ID: "course_linux", Slug: "linux-dev", Title: "Linux для разработчика", Description: "Процессы, сеть, systemd, shell, диагностика и инструменты, которыми реально пользуются на работе.", Level: "Beginner → Advanced", Duration: "6 недель", Status: "open"},
		{ID: "course_cpp", Slug: "cpp-systems", Title: "C/C++ и системное программирование", Description: "Память, ABI, Linux API, отладка и инженерный подход к системному коду.", Level: "Intermediate", Duration: "8 недель", Status: "soon"},
	}
	for i, course := range courses {
		if err := db.Exec(`INSERT INTO courses(id,slug,title,description,level,duration,status,sort_order,created_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
			course.ID, course.Slug, course.Title, course.Description, course.Level, course.Duration, course.Status, i+1, now); err != nil {
			return err
		}
	}
	return nil
}

func listCourses(db *AppDB, userID string) ([]StudyCourse, error) {
	rows, err := db.Query(`SELECT c.id,c.slug,c.title,c.description,c.level,c.duration,c.status,
		CASE WHEN e.user_id IS NULL THEN 0 ELSE 1 END,COALESCE(e.progress,0),
		(SELECT COUNT(*) FROM enrollments ec WHERE ec.course_id=c.id AND ec.status='active')
		FROM courses c LEFT JOIN enrollments e ON e.course_id=c.id AND e.user_id=?
		WHERE c.status <> 'archived' ORDER BY c.sort_order,c.title`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]StudyCourse, 0, len(rows))
	for _, row := range rows {
		result = append(result, StudyCourse{ID: row[0], Slug: row[1], Title: row[2], Description: row[3], Level: row[4], Duration: row[5], Status: row[6], Enrolled: row[7] == "1", Progress: parseInt(row[8]), Students: parseInt(row[9])})
	}
	return result, nil
}

func enrollCourse(db *AppDB, userID, courseID string) error {
	rows, err := db.Query(`SELECT status FROM courses WHERE id=? LIMIT 1`, courseID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("курс не найден")
	}
	if rows[0][0] != "open" {
		return errors.New("запись на этот курс пока закрыта")
	}
	return db.Exec(`INSERT INTO enrollments(user_id,course_id,status,progress,created_at) VALUES(?,?, 'active',0,?)
		ON CONFLICT(user_id,course_id) DO UPDATE SET status='active'`, userID, courseID, time.Now().UTC().Unix())
}

func weekdayNumber(day time.Weekday) int {
	if day == time.Sunday {
		return 7
	}
	return int(day)
}

func weekdayFromNumber(day int) time.Weekday {
	if day == 7 {
		return time.Sunday
	}
	return time.Weekday(day)
}

func weekdayNameRU(day int) string {
	return map[int]string{1: "Понедельник", 2: "Вторник", 3: "Среда", 4: "Четверг", 5: "Пятница", 6: "Суббота", 7: "Воскресенье"}[day]
}
