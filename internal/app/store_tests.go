package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const diagnosticTestID = "diagnostic-entry-v1"

func seedDiagnosticTest(db dbExecutor, now int64) error {
	if err := db.exec(`INSERT INTO test_templates(id,slug,title,description,duration_minutes,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
		diagnosticTestID,
		"entry-diagnostic",
		"Входной диагностический тест",
		"Ориентир по времени: 15–20 минут. Цель теста — не получить максимальный балл, а понять текущий уровень и выбрать направление дальнейшего обучения. Вопросы 1–5 помогают понять интересы и опыт и не оцениваются. Если технического ответа не знаешь, выбирай E — «Не знаю».",
		20,
		"active",
		now,
		now,
	); err != nil {
		return err
	}

	type seedQ struct {
		n       int
		section string
		prompt  string
		code    string
		kind    string
		options []TestOption
		correct []string
		explain string
		cat     string
		scored  bool
	}
	opt := func(items ...string) []TestOption {
		result := make([]TestOption, 0, len(items))
		for i, item := range items {
			result = append(result, TestOption{Key: string(rune('A' + i)), Text: item})
		}
		return result
	}
	unknown := func(items ...string) []TestOption {
		items = append(items, "Не знаю")
		return opt(items...)
	}
	questions := []seedQ{
		{1, "Часть 1. Что тебе интересно", "Какие направления тебе сейчас кажутся наиболее интересными? Можно выбрать несколько вариантов.", "", "survey_multiple", opt("Linux и понимание того, как работает операционная система", "C / C++ и системное программирование", "Go и разработка серверных приложений", "DevOps: Docker, серверы, CI/CD, автоматизация", "Linux-администрирование", "Сети", "Информационная безопасность", "Пока не знаю — хочу попробовать разное"), nil, "", "survey", false},
		{2, "Часть 1. Что тебе интересно", "Какой результат занятий тебе был бы интереснее всего?", "", "survey_single", opt("Уметь хорошо программировать и писать серьёзные программы", "Понимать, как компьютер и ОС работают «изнутри»", "Уметь настраивать Linux-серверы", "Научиться Docker, CI/CD, DevOps-инструментам", "Подготовиться к будущей работе в IT", "Делать собственные проекты", "Пока не знаю"), nil, "", "survey", false},
		{3, "Часть 1. Что тебе интересно", "Что из этого ты уже когда-либо делал? Можно выбрать несколько вариантов.", "", "survey_multiple", opt("Пользовался терминалом Linux", "Устанавливал Linux самостоятельно", "Писал программы на C или C++", "Писал программы на Go", "Использовал Git", "Подключался к компьютеру по SSH", "Пользовался Docker", "Настраивал сервер или виртуальную машину", "Компилировал программы через командную строку", "Почти ничего из перечисленного"), nil, "", "survey", false},
		{4, "Часть 1. Что тебе интересно", "Какой тип задач кажется тебе наиболее интересным?", "", "survey_single", opt("Написать программу с нуля", "Найти ошибку в чужой программе", "Разобраться, почему Linux или сервер работает неправильно", "Настроить несколько сервисов так, чтобы они работали вместе", "Разобраться, как что-то устроено внутри", "Автоматизировать рутинную работу", "Пока не знаю"), nil, "", "survey", false},
		{5, "Часть 1. Что тебе интересно", "Как ты сам оцениваешь свой уровень?", "", "survey_single", opt("Практически начинающий", "Что-то пробовал, но знания пока разрозненные", "Знаю базу и могу самостоятельно решать простые задачи", "Уже могу писать программы / работать с Linux без постоянной помощи", "Сложно оценить"), nil, "", "survey", false},

		{6, "Часть 2. Linux и командная строка", "Какая команда показывает текущую директорию в Linux?", "", "single", unknown("ls", "pwd", "cd", "where"), []string{"B"}, "pwd — print working directory, показывает текущую директорию.", "linux", true},
		{7, "Часть 2. Linux и командная строка", "Выполнили команду chmod 640 notes.txt. Какие права получит файл?", "chmod 640 notes.txt", "single", unknown("Владелец: чтение и запись; группа: чтение; остальные: никаких прав", "Владелец: чтение, запись и выполнение; группа: чтение; остальные: никаких прав", "Все пользователи смогут читать и записывать файл", "Только root сможет открыть файл"), []string{"A"}, "6 = rw-, 4 = r--, 0 = ---.", "linux", true},
		{8, "Часть 2. Linux и командная строка", "Что делает команда?", "grep -R \"TODO\" .", "single", unknown("Удаляет строки с TODO", "Ищет файлы с именем TODO", "Рекурсивно ищет текст TODO в текущей директории и её поддиректориях", "Показывает только переменные окружения с именем TODO"), []string{"C"}, "grep -R выполняет рекурсивный поиск текста.", "linux", true},
		{9, "Часть 2. Linux и командная строка", "Что произойдёт при выполнении?", "command1 && command2", "single", unknown("Обе команды всегда выполнятся одновременно", "command2 выполнится только в случае успешного завершения command1", "command2 выполнится только при ошибке command1", "Результат command1 передастся на вход command2"), []string{"B"}, "&& запускает следующую команду, если предыдущая завершилась успешно, то есть обычно вернула код 0.", "linux", true},
		{10, "Часть 2. Linux и командная строка", "Что обычно означает команда вида?", "sudo systemctl restart nginx", "single", unknown("Перезагрузка всего компьютера", "Запуск команды с повышенными привилегиями, если у пользователя есть соответствующие права", "Вход в систему под пользователем nginx", "Компиляция nginx"), []string{"B"}, "sudo позволяет выполнить разрешённую команду с повышенными привилегиями; часто это привилегии root.", "linux", true},

		{11, "Часть 3. Программирование, C и C++", "Что выведет программа?", "int x = 4;\nint* p = &x;\n*p += 3;\n\nstd::cout << x;", "single", unknown("3", "4", "7", "Адрес переменной x"), []string{"C"}, "p содержит адрес x, поэтому изменение *p изменяет непосредственно x. Результат — 7.", "cpp", true},
		{12, "Часть 3. Программирование, C и C++", "В чём проблема этого кода?", "for (size_t i = 0; i <= v.size(); ++i) {\n    std::cout << v[i] << '\\n';\n}", "single", unknown("Никакой проблемы нет", "Последняя итерация обращается к элементу за границей vector", "Цикл никогда не закончится", "size_t нельзя использовать в цикле"), []string{"B"}, "Допустимые индексы vector идут от 0 до size() - 1. При i == v.size() происходит выход за границы.", "cpp", true},
		{13, "Часть 3. Программирование, C и C++", "Что можно сказать об этом коде?", "int a[3] = {10, 20, 30};\nstd::cout << a[3];", "single", unknown("Он гарантированно выведет 30", "Он гарантированно выведет 0", "Происходит обращение за границы массива, поведение программы не определено", "Программа не скомпилируется из-за размера массива"), []string{"C"}, "a[3] находится за пределами массива из трёх элементов. В C/C++ это undefined behavior.", "cpp", true},
		{14, "Часть 3. Программирование, C и C++", "Что означает объявление?", "const int* p;", "single", unknown("Нельзя изменять сам указатель p", "Через p нельзя изменять значение объекта, на который он указывает", "p обязательно равен nullptr", "p может указывать только на число 0"), []string{"B"}, "const int* — указатель на значение, которое нельзя изменять через этот указатель. Сам указатель при этом можно переназначить.", "cpp", true},
		{15, "Часть 3. Программирование, C и C++", "Что примерно делает команда?", "g++ main.cpp -o app", "single", unknown("Запускает файл main.cpp", "Удаляет исходный файл после запуска", "Компилирует и линкует main.cpp, создавая исполняемый файл app", "Преобразует C++ в Python"), []string{"C"}, "g++ компилирует исходный файл и создаёт исполняемый файл app.", "cpp", true},

		{16, "Часть 4. Go", "Что такое goroutine в Go?", "", "single", unknown("Отдельная операционная система", "Механизм лёгкого конкурентного выполнения функции, управляемый Go runtime", "Специальный тип массива", "Аналог файла .cpp"), []string{"B"}, "Goroutine — лёгкая единица конкурентного выполнения, которой управляет Go runtime.", "go", true},

		{17, "Часть 5. Системы, Git и DevOps", "Какое утверждение лучше всего описывает обычный Docker-контейнер?", "", "single", unknown("Это всегда полноценная виртуальная машина со своим ядром ОС", "Контейнер изолирует приложение и его окружение, обычно используя ядро хостовой ОС", "Docker нужен только для компиляции C++", "Контейнер — это просто ZIP-архив"), []string{"B"}, "Контейнеры изолируют приложение и процессы, но обычно используют ядро хостовой ОС, в отличие от обычной полноценной VM с отдельной гостевой ОС.", "systems", true},
		{18, "Часть 5. Системы, Git и DevOps", "Что обычно делает git pull?", "git pull", "single", unknown("Только показывает историю Git", "Получает изменения из удалённого репозитория и интегрирует их в текущую ветку", "Удаляет локальные изменения", "Создаёт новый Git-репозиторий"), []string{"B"}, "git pull получает изменения из удалённого репозитория и интегрирует их в локальную ветку. Конкретный способ интеграции зависит от конфигурации Git.", "systems", true},
		{19, "Часть 5. Системы, Git и DevOps", "Что обычно обозначает IP-адрес 127.0.0.1?", "127.0.0.1", "single", unknown("Адрес роутера", "Адрес DNS-сервера Google", "Текущий компьютер — loopback / localhost", "Любой компьютер в локальной сети"), []string{"C"}, "127.0.0.1 — loopback-адрес текущего компьютера.", "systems", true},
		{20, "Часть 5. Системы, Git и DevOps", "Что делает команда?", "grep \"ERROR\" app.log | wc -l", "single", unknown("Удаляет все ошибки из файла", "Показывает весь файл app.log", "Считает количество строк, содержащих ERROR", "Считает количество файлов с именем ERROR"), []string{"C"}, "grep выбирает подходящие строки, а wc -l считает их количество.", "systems", true},
		{21, "Часть 5. Системы, Git и DevOps", "На Linux-сервере перестал открываться сайт. Что логичнее сделать в первую очередь?", "", "single", unknown("Сразу переустановить Linux", "Купить новый сервер", "Проверить состояние сервиса, процессы, прослушиваемые порты и логи", "Удалить конфигурацию сервера и создать её заново"), []string{"C"}, "Сначала имеет смысл диагностировать проблему: проверить состояние процесса/сервиса, порт и логи.", "systems", true},
	}

	for _, q := range questions {
		optionsJSON, _ := json.Marshal(q.options)
		correctJSON, _ := json.Marshal(q.correct)
		scored := 0
		if q.scored {
			scored = 1
		}
		id := fmt.Sprintf("%s-q%02d", diagnosticTestID, q.n)
		if err := db.exec(`INSERT INTO test_questions(id,test_id,sort_order,section,prompt,code,kind,options_json,correct_json,explanation,category,scored)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
			id, diagnosticTestID, q.n, q.section, q.prompt, q.code, q.kind, string(optionsJSON), string(correctJSON), q.explain, q.cat, scored); err != nil {
			return err
		}
	}
	return nil
}

func listTestTemplates(db *AppDB) ([]TestTemplate, error) {
	rows, err := db.Query(`SELECT t.id,t.slug,t.title,t.description,t.duration_minutes,t.status,
		COUNT(q.id),COALESCE(SUM(CASE WHEN q.scored=1 THEN 1 ELSE 0 END),0)
		FROM test_templates t LEFT JOIN test_questions q ON q.test_id=t.id
		WHERE t.status<>'archived' GROUP BY t.id,t.slug,t.title,t.description,t.duration_minutes,t.status
		ORDER BY t.created_at,t.title`)
	if err != nil {
		return nil, err
	}
	result := make([]TestTemplate, 0, len(rows))
	for _, row := range rows {
		result = append(result, TestTemplate{ID: row[0], Slug: row[1], Title: row[2], Description: row[3], DurationMinutes: parseInt(row[4]), Status: row[5], QuestionCount: parseInt(row[6]), ScoredQuestions: parseInt(row[7]), MaxScore: parseInt(row[7])})
	}
	return result, nil
}

func getTestTemplate(db *AppDB, testID string) (TestTemplate, error) {
	rows, err := db.Query(`SELECT id,slug,title,description,duration_minutes,status FROM test_templates WHERE id=? LIMIT 1`, testID)
	if err != nil {
		return TestTemplate{}, err
	}
	if len(rows) == 0 {
		return TestTemplate{}, errors.New("тест не найден")
	}
	r := rows[0]
	test := TestTemplate{ID: r[0], Slug: r[1], Title: r[2], Description: r[3], DurationMinutes: parseInt(r[4]), Status: r[5]}
	questions, err := loadTestQuestions(db, test.ID)
	if err != nil {
		return TestTemplate{}, err
	}
	test.Questions = questions
	test.QuestionCount = len(questions)
	for _, q := range questions {
		if q.Scored {
			test.ScoredQuestions++
			test.MaxScore++
		}
	}
	return test, nil
}

func loadTestQuestions(db dbExecutor, testID string) ([]TestQuestion, error) {
	rows, err := db.query(`SELECT id,test_id,sort_order,section,prompt,code,kind,options_json,correct_json,explanation,category,scored
		FROM test_questions WHERE test_id=? ORDER BY sort_order`, testID)
	if err != nil {
		return nil, err
	}
	result := make([]TestQuestion, 0, len(rows))
	for _, row := range rows {
		var options []TestOption
		var correct []string
		_ = json.Unmarshal([]byte(row[7]), &options)
		_ = json.Unmarshal([]byte(row[8]), &correct)
		result = append(result, TestQuestion{ID: row[0], TestID: row[1], SortOrder: parseInt(row[2]), Section: row[3], Prompt: row[4], Code: row[5], Kind: row[6], Options: options, Correct: correct, Explanation: row[9], Category: row[10], Scored: row[11] == "1"})
	}
	return result, nil
}

func assignTest(db *AppDB, adminID, testID, studentID, bookingID, attachMode string, deadline time.Time, timeLimit int, loc *time.Location) (TestAssignment, error) {
	attachMode = strings.TrimSpace(attachMode)
	if attachMode != "direct" && attachMode != "lesson" && attachMode != "homework" {
		return TestAssignment{}, errors.New("некорректный режим выдачи")
	}
	test, err := getTestTemplate(db, testID)
	if err != nil || test.Status != "active" {
		return TestAssignment{}, errors.New("тест недоступен")
	}
	student, err := getStudentByID(db, studentID)
	if err != nil || student == nil {
		return TestAssignment{}, errors.New("ученик не найден")
	}
	if timeLimit <= 0 {
		timeLimit = test.DurationMinutes
	}
	if timeLimit < 1 || timeLimit > 480 {
		return TestAssignment{}, errors.New("время теста должно быть от 1 до 480 минут")
	}
	if attachMode != "direct" {
		rows, qErr := db.Query(`SELECT user_id FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if qErr != nil || len(rows) == 0 || rows[0][0] != studentID {
			return TestAssignment{}, errors.New("занятие ученика не найдено")
		}
	} else {
		bookingID = ""
	}
	if !deadline.IsZero() && !deadline.After(time.Now().In(loc)) {
		return TestAssignment{}, errors.New("дедлайн должен быть в будущем")
	}
	id, err := randomToken(12)
	if err != nil {
		return TestAssignment{}, err
	}
	now := time.Now().UTC()
	var bookingArg any
	if bookingID != "" {
		bookingArg = bookingID
	}
	var deadlineArg any
	if !deadline.IsZero() {
		deadlineArg = deadline.UTC().Unix()
	}
	err = db.WithTx(func(tx *dbTx) error {
		if err := tx.exec(`INSERT INTO test_assignments(id,test_id,user_id,booking_id,attach_mode,time_limit_minutes,deadline_at,assigned_by,status,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`, id, testID, studentID, bookingArg, attachMode, timeLimit, deadlineArg, adminID, "active", now.Unix()); err != nil {
			return err
		}
		body := fmt.Sprintf("Вам назначен тест «%s». На выполнение: %d мин.", test.Title, timeLimit)
		if !deadline.IsZero() {
			body += " Дедлайн: " + deadline.In(loc).Format("02.01.2006 15:04") + "."
		}
		return createNotificationTx(tx, studentID, "test_assigned", "Новое задание", body, bookingID, "", now.Unix())
	})
	if err != nil {
		return TestAssignment{}, err
	}
	return getTestAssignment(db, id, loc)
}

func getTestAssignment(db *AppDB, assignmentID string, loc *time.Location) (TestAssignment, error) {
	rows, err := db.Query(testAssignmentSelect+` WHERE a.id=? LIMIT 1`, assignmentID)
	if err != nil {
		return TestAssignment{}, err
	}
	if len(rows) == 0 {
		return TestAssignment{}, errors.New("задание не найдено")
	}
	return parseTestAssignmentRow(rows[0], loc), nil
}

const testAssignmentSelect = `SELECT a.id,a.test_id,t.title,a.user_id,u.name,u.email,COALESCE(a.booking_id,''),COALESCE(b.date,''),COALESCE(b.time,''),
	a.attach_mode,a.status,a.time_limit_minutes,COALESCE(a.deadline_at,0),a.created_at,
	COALESCE(at.id,''),COALESCE(at.status,''),COALESCE(at.started_at,0),COALESCE(at.expires_at,0),COALESCE(at.submitted_at,0),COALESCE(at.score,0),COALESCE(at.max_score,0)
	FROM test_assignments a JOIN test_templates t ON t.id=a.test_id JOIN users u ON u.id=a.user_id
	LEFT JOIN bookings b ON b.id=a.booking_id LEFT JOIN test_attempts at ON at.assignment_id=a.id`

func parseTestAssignmentRow(row []string, loc *time.Location) TestAssignment {
	a := TestAssignment{ID: row[0], TestID: row[1], TestTitle: row[2], UserID: row[3], UserName: row[4], UserEmail: row[5], BookingID: row[6], BookingDate: row[7], BookingTime: row[8], AttachMode: row[9], Status: row[10], TimeLimitMinutes: parseInt(row[11]), CreatedAt: time.Unix(parseInt64(row[13]), 0).In(loc), AttemptID: row[14], AttemptStatus: row[15], Score: parseInt(row[19]), MaxScore: parseInt(row[20])}
	if ts := parseInt64(row[12]); ts > 0 {
		a.HasDeadline = true
		a.DeadlineAt = time.Unix(ts, 0).In(loc)
	}
	if a.AttemptID != "" {
		a.HasAttempt = true
		if ts := parseInt64(row[16]); ts > 0 {
			a.StartedAt = time.Unix(ts, 0).In(loc)
		}
		if ts := parseInt64(row[17]); ts > 0 {
			a.ExpiresAt = time.Unix(ts, 0).In(loc)
		}
		if ts := parseInt64(row[18]); ts > 0 {
			a.SubmittedAt = time.Unix(ts, 0).In(loc)
		}
	}
	if a.AttemptStatus == "" && a.HasDeadline && time.Now().In(loc).After(a.DeadlineAt) {
		a.AttemptStatus = "expired"
	}
	return a
}

func listStudentTestAssignments(db *AppDB, userID string, loc *time.Location) ([]TestAssignment, error) {
	_ = finalizeExpiredTestAttempts(db)
	rows, err := db.Query(testAssignmentSelect+` WHERE a.user_id=? AND a.status='active' ORDER BY a.created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]TestAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, parseTestAssignmentRow(row, loc))
	}
	return result, nil
}

func listAdminTestAssignments(db *AppDB, loc *time.Location) ([]TestAssignment, error) {
	_ = finalizeExpiredTestAttempts(db)
	rows, err := db.Query(testAssignmentSelect + ` ORDER BY a.created_at DESC LIMIT 120`)
	if err != nil {
		return nil, err
	}
	result := make([]TestAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, parseTestAssignmentRow(row, loc))
	}
	return result, nil
}

func listTestAssignmentsForBooking(db *AppDB, bookingID string, loc *time.Location) ([]TestAssignment, error) {
	_ = finalizeExpiredTestAttempts(db)
	rows, err := db.Query(testAssignmentSelect+` WHERE a.booking_id=? AND a.status='active' ORDER BY a.created_at`, bookingID)
	if err != nil {
		return nil, err
	}
	result := make([]TestAssignment, 0, len(rows))
	for _, row := range rows {
		result = append(result, parseTestAssignmentRow(row, loc))
	}
	return result, nil
}

func attachTestsToStudentData(bookings []StudyBooking, lessons []StudyLesson, assignments []TestAssignment) ([]StudyBooking, []StudyLesson) {
	byBookingLesson := map[string][]TestAssignment{}
	byBookingHomework := map[string][]TestAssignment{}
	for _, a := range assignments {
		if a.BookingID == "" {
			continue
		}
		if a.AttachMode == "homework" {
			byBookingHomework[a.BookingID] = append(byBookingHomework[a.BookingID], a)
		} else if a.AttachMode == "lesson" {
			byBookingLesson[a.BookingID] = append(byBookingLesson[a.BookingID], a)
		}
	}
	for i := range bookings {
		bookings[i].Tests = append([]TestAssignment(nil), byBookingLesson[bookings[i].ID]...)
	}
	for i := range lessons {
		lessons[i].Tests = append([]TestAssignment(nil), byBookingHomework[lessons[i].BookingID]...)
	}
	return bookings, lessons
}

func startTestAttempt(db *AppDB, assignment TestAssignment) (TestAttempt, error) {
	if assignment.Status != "active" {
		return TestAttempt{}, errors.New("задание недоступно")
	}
	if assignment.HasDeadline && !time.Now().Before(assignment.DeadlineAt) {
		return TestAttempt{}, errors.New("дедлайн уже истёк")
	}
	if existing, err := getTestAttemptByAssignment(db, assignment.ID); err == nil && existing.ID != "" {
		return existing, nil
	}
	id, err := randomToken(12)
	if err != nil {
		return TestAttempt{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(time.Duration(assignment.TimeLimitMinutes) * time.Minute)
	if assignment.HasDeadline && assignment.DeadlineAt.UTC().Before(expires) {
		expires = assignment.DeadlineAt.UTC()
	}
	if err := db.Exec(`INSERT INTO test_attempts(id,assignment_id,user_id,status,started_at,expires_at,answers_json,result_json,score,max_score,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, assignment.ID, assignment.UserID, "in_progress", now.Unix(), expires.Unix(), "{}", "{}", 0, 0, now.Unix()); err != nil {
		if isDBConstraint(err) {
			return getTestAttemptByAssignment(db, assignment.ID)
		}
		return TestAttempt{}, err
	}
	return getTestAttemptByAssignment(db, assignment.ID)
}

func getTestAttemptByAssignment(db *AppDB, assignmentID string) (TestAttempt, error) {
	rows, err := db.Query(`SELECT id,assignment_id,user_id,status,started_at,COALESCE(expires_at,0),COALESCE(submitted_at,0),score,max_score,answers_json,updated_at
		FROM test_attempts WHERE assignment_id=? LIMIT 1`, assignmentID)
	if err != nil {
		return TestAttempt{}, err
	}
	if len(rows) == 0 {
		return TestAttempt{}, nil
	}
	r := rows[0]
	attempt := TestAttempt{ID: r[0], AssignmentID: r[1], UserID: r[2], Status: r[3], StartedAt: time.Unix(parseInt64(r[4]), 0).UTC(), Score: parseInt(r[7]), MaxScore: parseInt(r[8]), Answers: decodeAnswers(r[9]), UpdatedAt: time.Unix(parseInt64(r[10]), 0).UTC()}
	if ts := parseInt64(r[5]); ts > 0 {
		attempt.ExpiresAt = time.Unix(ts, 0).UTC()
	}
	if ts := parseInt64(r[6]); ts > 0 {
		attempt.SubmittedAt = time.Unix(ts, 0).UTC()
	}
	return attempt, nil
}

func decodeAnswers(raw string) map[string][]string {
	answers := map[string][]string{}
	if strings.TrimSpace(raw) == "" {
		return answers
	}
	_ = json.Unmarshal([]byte(raw), &answers)
	return normalizeAnswers(answers)
}

func normalizeAnswers(answers map[string][]string) map[string][]string {
	result := make(map[string][]string, len(answers))
	for id, values := range answers {
		seen := map[string]bool{}
		clean := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(strings.ToUpper(value))
			if value == "" || len(value) > 4 || seen[value] {
				continue
			}
			seen[value] = true
			clean = append(clean, value)
		}
		sort.Strings(clean)
		if len(clean) > 0 {
			result[id] = clean
		}
	}
	return result
}

func saveTestAnswers(db *AppDB, assignment TestAssignment, answers map[string][]string) (TestAttempt, error) {
	attempt, err := getTestAttemptByAssignment(db, assignment.ID)
	if err != nil || attempt.ID == "" {
		return TestAttempt{}, errors.New("тест ещё не начат")
	}
	if attempt.Status != "in_progress" {
		return attempt, errors.New("попытка уже завершена")
	}
	if !attempt.ExpiresAt.IsZero() && !time.Now().UTC().Before(attempt.ExpiresAt) {
		return finalizeTestAttempt(db, attempt.ID, "expired")
	}
	answers = normalizeAnswers(answers)
	encoded, _ := json.Marshal(answers)
	if err := db.Exec(`UPDATE test_attempts SET answers_json=?,updated_at=? WHERE id=? AND status='in_progress'`, string(encoded), time.Now().UTC().Unix(), attempt.ID); err != nil {
		return TestAttempt{}, err
	}
	return getTestAttemptByAssignment(db, assignment.ID)
}

func submitTestAttempt(db *AppDB, assignment TestAssignment, answers map[string][]string) (TestAttempt, error) {
	attempt, err := getTestAttemptByAssignment(db, assignment.ID)
	if err != nil || attempt.ID == "" {
		return TestAttempt{}, errors.New("тест ещё не начат")
	}
	if attempt.Status != "in_progress" {
		return attempt, nil
	}
	if !attempt.ExpiresAt.IsZero() && !time.Now().UTC().Before(attempt.ExpiresAt) {
		return finalizeTestAttempt(db, attempt.ID, "expired")
	}
	answers = normalizeAnswers(answers)
	encoded, _ := json.Marshal(answers)
	if err := db.Exec(`UPDATE test_attempts SET answers_json=?,updated_at=? WHERE id=? AND status='in_progress'`, string(encoded), time.Now().UTC().Unix(), attempt.ID); err != nil {
		return TestAttempt{}, err
	}
	return finalizeTestAttempt(db, attempt.ID, "submitted")
}

func finalizeExpiredTestAttempts(db *AppDB) error {
	rows, err := db.Query(`SELECT id FROM test_attempts WHERE status='in_progress' AND expires_at IS NOT NULL AND expires_at>0 AND expires_at<=? LIMIT 100`, time.Now().UTC().Unix())
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := finalizeTestAttempt(db, row[0], "expired"); err != nil {
			return err
		}
	}
	return nil
}

func finalizeTestAttempt(db *AppDB, attemptID, finalStatus string) (TestAttempt, error) {
	if finalStatus != "submitted" && finalStatus != "expired" {
		return TestAttempt{}, errors.New("некорректный статус попытки")
	}
	var assignmentID string
	err := db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT at.assignment_id,at.status,at.answers_json,a.test_id,a.assigned_by,COALESCE(a.booking_id,''),u.name,t.title
			FROM test_attempts at JOIN test_assignments a ON a.id=at.assignment_id JOIN users u ON u.id=a.user_id JOIN test_templates t ON t.id=a.test_id
			WHERE at.id=? LIMIT 1`, attemptID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("попытка не найдена")
		}
		r := rows[0]
		assignmentID = r[0]
		if r[1] != "in_progress" {
			return nil
		}
		questions, err := loadTestQuestions(tx, r[3])
		if err != nil {
			return err
		}
		answers := decodeAnswers(r[2])
		score, maxScore, breakdown := evaluateTest(questions, answers)
		resultJSON, _ := json.Marshal(map[string]any{"breakdown": breakdown})
		now := time.Now().UTC().Unix()
		if err := tx.exec(`UPDATE test_attempts SET status=?,submitted_at=?,score=?,max_score=?,result_json=?,updated_at=? WHERE id=? AND status='in_progress'`, finalStatus, now, score, maxScore, string(resultJSON), now, attemptID); err != nil {
			return err
		}
		body := fmt.Sprintf("%s завершил тест «%s»: %d/%d.", r[6], r[7], score, maxScore)
		if finalStatus == "expired" {
			body = fmt.Sprintf("У %s закончилось время теста «%s». Результат: %d/%d.", r[6], r[7], score, maxScore)
		}
		return createNotificationTx(tx, r[4], "test_submitted", "Результат теста", body, r[5], "", now)
	})
	if err != nil {
		return TestAttempt{}, err
	}
	return getTestAttemptByAssignment(db, assignmentID)
}

func evaluateTest(questions []TestQuestion, answers map[string][]string) (int, int, []TestBreakdown) {
	type totals struct{ score, max int }
	byCategory := map[string]*totals{
		"linux":   {},
		"cpp":     {},
		"go":      {},
		"systems": {},
	}
	score, maxScore := 0, 0
	for _, q := range questions {
		if !q.Scored {
			continue
		}
		maxScore++
		t := byCategory[q.Category]
		if t != nil {
			t.max++
		}
		if sameAnswers(answers[q.ID], q.Correct) {
			score++
			if t != nil {
				t.score++
			}
		}
	}
	order := []struct{ key, label string }{{"linux", "Linux"}, {"cpp", "C / C++"}, {"go", "Go"}, {"systems", "Systems / DevOps"}}
	result := make([]TestBreakdown, 0, len(order))
	for _, item := range order {
		t := byCategory[item.key]
		if t == nil || t.max == 0 {
			continue
		}
		result = append(result, TestBreakdown{Key: item.key, Label: item.label, Score: t.score, MaxScore: t.max, Level: diagnosticLevel(item.key, t.score, t.max)})
	}
	return score, maxScore, result
}

func diagnosticLevel(category string, score, max int) string {
	if category == "go" {
		if score == 1 {
			return "Правильный ответ ещё не означает знание Go; практический уровень стоит проверить отдельной задачей."
		}
		return "Знакомство с терминологией Go по этому вопросу не подтверждено."
	}
	if max != 5 {
		return ""
	}
	switch category {
	case "linux":
		switch score {
		case 0, 1:
			return "Практически нет базы."
		case 2:
			return "Знаком с отдельными командами и понятиями."
		case 3:
			return "Начальный практический уровень."
		case 4:
			return "Хорошая базовая подготовка."
		case 5:
			return "Уверенная база; можно быстро уходить от простых команд к процессам, файловой системе, shell и устройству Linux."
		}
	case "cpp":
		switch score {
		case 0, 1:
			return "Программирование нужно начинать почти с основ."
		case 2:
			return "Основные понятия знакомы, но знания фрагментарные."
		case 3:
			return "Есть рабочая база."
		case 4:
			return "Хороший базовый уровень."
		case 5:
			return "Базовые вопросы слишком просты — стоит проверять практической задачей, памятью, структурами данных, RAII, компиляцией и отладкой."
		}
	case "systems":
		switch score {
		case 0, 1:
			return "Направление практически новое."
		case 2:
			return "Слышал основные термины."
		case 3:
			return "Есть начальное понимание инструментов и систем."
		case 4:
			return "Хорошая база."
		case 5:
			return "Можно проверять реальный практический опыт: Dockerfile, systemd, сеть, SSH, Git, процессы, логи и диагностику."
		}
	}
	return ""
}

func sameAnswers(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func testBreakdown(questions []TestQuestion, answers map[string][]string) []TestBreakdown {
	_, _, result := evaluateTest(questions, answers)
	return result
}

func makeTestQuestionViews(questions []TestQuestion, answers map[string][]string, finished bool) []TestQuestionView {
	views := make([]TestQuestionView, 0, len(questions))
	for _, q := range questions {
		selected := map[string]bool{}
		for _, answer := range answers[q.ID] {
			selected[answer] = true
		}
		correct := map[string]bool{}
		for _, answer := range q.Correct {
			correct[answer] = true
		}
		view := TestQuestionView{ID: q.ID, SortOrder: q.SortOrder, Section: q.Section, Prompt: q.Prompt, Code: q.Code, Kind: q.Kind, Scored: q.Scored, Explanation: q.Explanation, Answered: len(answers[q.ID]) > 0}
		if q.Scored && finished {
			view.IsCorrect = sameAnswers(answers[q.ID], q.Correct)
		}
		for _, option := range q.Options {
			ov := TestOptionView{Key: option.Key, Text: option.Text, Selected: selected[option.Key]}
			if finished && q.Scored {
				ov.Correct = correct[option.Key]
				switch {
				case ov.Selected && ov.Correct:
					ov.State = "selected-correct"
				case ov.Selected && !ov.Correct:
					ov.State = "selected-wrong"
				case !ov.Selected && ov.Correct:
					ov.State = "correct-answer"
				}
			}
			view.Options = append(view.Options, ov)
		}
		views = append(views, view)
	}
	return views
}
