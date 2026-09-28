package app

// homeworkSeedTask is a built-in catalog item. Stable IDs make the seed idempotent:
// a task is inserted once and later edits made through the admin UI are preserved.
type homeworkSeedTask struct {
	ID         string
	Title      string
	Topic      string
	Language   string
	Difficulty string
	Statement  string
	Input      string
	Output     string
	Examples   string
}

var cHomeworkSeedTasks = []homeworkSeedTask{
	{
		ID:         "seed-c-easy-01-io-conditions",
		Title:      "Ввод, вывод и условия",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Попроси пользователя ввести возраст.\n\nЕсли возраст меньше `18`, выведи:\n\n`You are under 18`\n\nИначе выведи:\n\n`You are adult`",
		Input:      "Одно целое число — возраст пользователя.",
		Output:     "Если возраст меньше `18`, вывести `You are under 18`. Иначе вывести `You are adult`.",
		Examples:   "### Пример\n\n```text\nEnter age: 18\nYou are adult\n```",
	},
	{
		ID:         "seed-c-easy-02-even-odd",
		Title:      "Чётное или нечётное",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит число.\n\nПрограмма должна определить, является ли число чётным.\n\nЕсли число чётное, выведи `Even`, иначе — `Odd`.",
		Input:      "Одно целое число.",
		Output:     "`Even`, если число чётное, или `Odd`, если число нечётное.",
		Examples:   "### Пример\n\n```text\nEnter number: 17\nOdd\n```",
	},
	{
		ID:         "seed-c-easy-03-sign",
		Title:      "Положительное, отрицательное или ноль",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит число.\n\nНужно вывести одно из трёх сообщений:\n\n- `Positive` — если число положительное;\n- `Negative` — если число отрицательное;\n- `Zero` — если введён ноль.",
		Input:      "Одно целое число.",
		Output:     "Одно из сообщений: `Positive`, `Negative` или `Zero`.",
		Examples:   "",
	},
	{
		ID:         "seed-c-easy-04-max-two",
		Title:      "Максимум из двух",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит два числа `a` и `b`.\n\nВыведи большее из них. Если числа равны, выведи `Numbers are equal`.",
		Input:      "Два числа: `a` и `b`.",
		Output:     "Большее из двух чисел. Если числа равны — `Numbers are equal`.",
		Examples:   "### Пример\n\n```text\nEnter a: 15\nEnter b: 27\n27\n```",
	},
	{
		ID:         "seed-c-easy-05-max-three",
		Title:      "Максимум из трёх",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит три числа `a`, `b` и `c`.\n\nНайди максимальное из трёх чисел и выведи его в формате `Max = ...`.",
		Input:      "Три числа: `a`, `b` и `c`.",
		Output:     "Максимальное число в формате `Max = значение`.",
		Examples:   "### Пример\n\n```text\nEnter a: 10\nEnter b: 53\nEnter c: 24\nMax = 53\n```",
	},
	{
		ID:         "seed-c-easy-06-score",
		Title:      "Оценка",
		Topic:      "Ввод, вывод и условия",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит число от `0` до `100`.\n\nНужно вывести:\n\n- `90-100` → `Excellent`;\n- `70-89` → `Good`;\n- `50-69` → `Pass`;\n- `0-49` → `Fail`.\n\nЕсли пользователь ввёл число меньше `0` или больше `100`, выведи `Invalid score`.",
		Input:      "Одно целое число — оценка пользователя.",
		Output:     "`Excellent`, `Good`, `Pass` или `Fail` в зависимости от значения. Для числа вне диапазона `0..100` — `Invalid score`.",
		Examples:   "",
	},
	{
		ID:         "seed-c-easy-07-one-to-n",
		Title:      "От 1 до N",
		Topic:      "Циклы",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит положительное число `N`.\n\nВыведи все целые числа от `1` до `N`, каждое с новой строки.",
		Input:      "Одно положительное целое число `N`.",
		Output:     "Все числа от `1` до `N` включительно, каждое с новой строки.",
		Examples:   "### Пример\n\n```text\nEnter N: 7\n1\n2\n3\n4\n5\n6\n7\n```",
	},
	{
		ID:         "seed-c-easy-08-countdown",
		Title:      "Обратный отсчёт",
		Topic:      "Циклы",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит положительное число `N`.\n\nВыведи числа от `N` до `1` в обратном порядке, каждое с новой строки. После этого выведи `GO!`.",
		Input:      "Одно положительное целое число `N`.",
		Output:     "Числа от `N` до `1`, каждое с новой строки, затем строка `GO!`.",
		Examples:   "### Пример\n\n```text\nEnter N: 5\n5\n4\n3\n2\n1\nGO!\n```",
	},
	{
		ID:         "seed-c-easy-09-sum-one-to-n",
		Title:      "Сумма от 1 до N",
		Topic:      "Циклы",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит положительное число `N`.\n\nПосчитай сумму всех целых чисел от `1` до `N` включительно и выведи результат в формате `Sum = ...`.",
		Input:      "Одно положительное целое число `N`.",
		Output:     "Сумма чисел от `1` до `N` в формате `Sum = значение`.",
		Examples:   "### Пример\n\nДля `N = 5` нужно посчитать:\n\n`1 + 2 + 3 + 4 + 5`\n\n```text\nEnter N: 5\nSum = 15\n```",
	},
	{
		ID:         "seed-c-easy-10-multiplication-table",
		Title:      "Таблица умножения",
		Topic:      "Циклы",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит число.\n\nВыведи таблицу умножения для этого числа от `1` до `10`.",
		Input:      "Одно целое число.",
		Output:     "10 строк таблицы умножения от `число * 1` до `число * 10`.",
		Examples:   "### Пример\n\n```text\nEnter number: 7\n7 * 1 = 7\n7 * 2 = 14\n7 * 3 = 21\n...\n7 * 10 = 70\n```",
	},
	{
		ID:         "seed-c-easy-11-factorial",
		Title:      "Факториал",
		Topic:      "Циклы",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит число `N`.\n\nВычисли факториал числа `N`.\n\nНапример, для `5`:\n\n`1 * 2 * 3 * 4 * 5 = 120`",
		Input:      "Одно неотрицательное целое число `N`.",
		Output:     "Факториал числа `N` в формате `N! = значение`.",
		Examples:   "### Пример\n\n```text\nEnter N: 5\n5! = 120\n```",
	},
	{
		ID:         "seed-c-easy-12-digit-count",
		Title:      "Количество цифр",
		Topic:      "Работа с цифрами числа",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит целое число.\n\nОпредели количество цифр в числе и выведи результат в формате `Digits: ...`.\n\n**Не использовать строки.**",
		Input:      "Одно целое число.",
		Output:     "Количество цифр в формате `Digits: количество`.",
		Examples:   "### Пример\n\n```text\nEnter number: 12345\nDigits: 5\n```",
	},
	{
		ID:         "seed-c-easy-13-digit-sum",
		Title:      "Сумма цифр",
		Topic:      "Работа с цифрами числа",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит целое число.\n\nНайди сумму его цифр и выведи результат в формате `Sum of digits: ...`.\n\n**Подсказка:** подумай про операции `%` и `/`.",
		Input:      "Одно целое число.",
		Output:     "Сумма цифр числа в формате `Sum of digits: значение`.",
		Examples:   "### Пример\n\n```text\nEnter number: 12345\nSum of digits: 15\n```",
	},
	{
		ID:         "seed-c-easy-14-reverse-number",
		Title:      "Число наоборот",
		Topic:      "Работа с цифрами числа",
		Language:   "C",
		Difficulty: "easy",
		Statement:  "Пользователь вводит целое число.\n\nВыведи число, записав его цифры в обратном порядке.\n\n**Не использовать строки и массивы.**",
		Input:      "Одно целое число.",
		Output:     "То же число с цифрами в обратном порядке.",
		Examples:   "### Пример\n\n```text\n12345\n54321\n```",
	},
}

func seedCHomeworkTasks(db dbExecutor, now int64) error {
	for _, task := range cHomeworkSeedTasks {
		if err := db.exec(`INSERT INTO homework_tasks(id,title,topic,language,difficulty,statement_md,input_md,output_md,examples_md,active,created_by,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,1,NULL,?,?) ON CONFLICT(id) DO NOTHING`,
			task.ID, task.Title, task.Topic, task.Language, task.Difficulty, task.Statement, task.Input, task.Output, task.Examples, now, now); err != nil {
			return err
		}
		// Existing seeded rows get the new I/O metadata once. Teacher edits are preserved:
		// only empty fields are filled during subsequent starts.
		if err := db.exec(`UPDATE homework_tasks SET input_md=? WHERE id=? AND input_md=''`, task.Input, task.ID); err != nil {
			return err
		}
		if err := db.exec(`UPDATE homework_tasks SET output_md=? WHERE id=? AND output_md=''`, task.Output, task.ID); err != nil {
			return err
		}
	}
	return nil
}
