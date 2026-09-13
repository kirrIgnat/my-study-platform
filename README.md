# Cosmic Portfolio + KIRILL.STUDY

Go-сайт с публичным портфолио и учебным сервисом KIRILL.STUDY. Учебная часть поддерживает регистрацию студентов, несколько постоянных еженедельных занятий, календари студента и преподавателя, согласование переносов, материалы после уроков, домашние задания, конференции, кабинеты и внутренние уведомления.

## PostgreSQL

Новая версия использует **PostgreSQL как единственный источник данных**. SQLite больше не используется, поэтому `study.db` приложению не нужен.

Физически отдельную базу на каждого ученика приложение не создаёт: для production это неудобно для миграций, резервных копий и связей между сущностями. Вместо этого используется одна база `kirill_study` с нормализованными таблицами и внешними ключами.

Основные таблицы:

- `users` — учётные записи, email, роль и пароль-хэш;
- `student_profiles` — отдельные данные профиля ученика;
- `sessions` — серверные сессии;
- `tutoring_schedules` — несколько закреплённых еженедельных слотов ученика;
- `bookings` — конкретные занятия;
- `reschedule_requests` — разовые и постоянные переносы;
- `notifications` — уведомления студента и преподавателя;
- `lesson_materials` — теория, домашнее задание и видео занятия;
- `password_reset_events` — аудит сбросов пароля;
- `schema_migrations` — версия схемы.

`courses` и `enrollments` сохранены под будущий раздел курсов, который пока показывает `Coming Soon`.

Схема также лежит в `deploy/postgres-schema.sql`. Приложение создаёт нужные таблицы автоматически на старте.

## Пароли

Пароли **не шифруются обратимо и не хранятся открытым текстом**. И студент, и администратор хранятся в `users`, а поле `password_hash` содержит PBKDF2-HMAC-SHA256 с уникальной случайной солью и 600 000 итераций.

Это означает:

- пароль нельзя получить из PostgreSQL;
- при входе введённый пароль проверяется против хэша;
- старые PBKDF2-хэши с меньшим числом итераций автоматически усиливаются после успешного входа;
- админ может сбросить пароль ученика и получить одноразовый временный пароль;
- на формах есть обычная кнопка «Показать / Скрыть» для пароля, который пользователь вводит прямо сейчас.

## Основные маршруты

- `/profile` — портфолио;
- `/study` — учебный сервис;
- `/study/login`, `/study/register` — вход и регистрация;
- `/study/booking` — выбор и добавление постоянных еженедельных слотов;
- `/study/courses` — `Coming Soon`;
- `/study/account` — кабинет ученика;
- `/study/notifications` — уведомления;
- `/study/admin` — кабинет преподавателя;
- `/study/admin/lesson?id=...` — управление занятием, конференцией, материалом и ДЗ.

## Расписание и переносы

Студент выбирает один или несколько свободных еженедельных слотов. Каждый выбранный слот закрепляется отдельно, а будущие занятия по нему создаются автоматически. Один и тот же постоянный слот нельзя закрепить за двумя студентами. На странице записи есть подсказка: 2 занятия в неделю обычно дают более стабильный темп, а формат 1 раз в неделю предполагает больше самостоятельной работы.

В ЛК доступны:

- запрос постоянной смены еженедельного времени;
- разовый перенос конкретного занятия;
- максимум 2 согласованных/запрошенных разовых переноса в месяц;
- выбор только реально свободного времени;
- применение переноса только после решения преподавателя.

Запросы и решения автоматически создают уведомления обеим сторонам.

## Календари и карточка ученика

В кабинете студента есть месячный календарь со всеми занятиями, статусами и ссылками на конференции. В кабинете преподавателя есть общий календарь всех учеников. В разделе «Ученики» клик по карточке открывает модальное окно с регулярным расписанием ученика, его календарём, всеми материалами и редактором теории/ДЗ/видео. Публикация из этого окна создаёт уведомление студенту.

## Материалы после урока

Преподаватель может для конкретного занятия сохранить и опубликовать:

- название урока;
- теорию / конспект;
- домашнее задание;
- ссылку на видео занятия;
- ссылку на конференцию;
- статус занятия.

После публикации студент видит материал в кабинете и получает внутреннее уведомление.

## Локальная установка PostgreSQL (Debian/Ubuntu)

```bash
sudo apt update
sudo apt install -y postgresql postgresql-contrib
```

Создайте пользователя и базу:

```bash
sudo -u postgres psql
```

В `psql`:

```sql
CREATE ROLE kirill_study WITH LOGIN PASSWORD 'CHANGE_THIS_DATABASE_PASSWORD';
CREATE DATABASE kirill_study OWNER kirill_study;
\q
```

Для локального сервера:

```bash
export DATABASE_URL='postgres://kirill_study:CHANGE_THIS_DATABASE_PASSWORD@127.0.0.1:5432/kirill_study?sslmode=disable'
export ADMIN_NAME='Кирилл'
export ADMIN_EMAIL='kirr.ignat@gmail.com'
export ADMIN_PASSWORD='CHANGE_TO_A_LONG_ADMIN_PASSWORD'
export COOKIE_SECURE=false
export PORT=3000

go mod download
go run .
```

После первого старта таблицы создадутся автоматически. `ADMIN_PASSWORD` используется только для первичного создания администратора; его пароль также сохраняется только в виде хэша.

Если база уже создана предыдущей PostgreSQL-версией, удалять её не нужно: при старте приложение автоматически снимает старое ограничение «один постоянный слот на ученика» и добавляет индексы для нескольких регулярных занятий в неделю. Существующие пользователи, занятия, материалы и уведомления сохраняются.

## Production environment

Пример находится в `deploy/cosmic-portfolio.env.example`:

```bash
APP_ENV=production
COOKIE_SECURE=true
DATABASE_URL='postgres://kirill_study:DB_PASSWORD@127.0.0.1:5432/kirill_study?sslmode=disable'
DB_MAX_CONNS=6

ADMIN_NAME=Кирилл
ADMIN_EMAIL=kirr.ignat@gmail.com
ADMIN_PASSWORD=LONG_RANDOM_ADMIN_PASSWORD

BOOKING_TIMEZONE=Europe/Moscow
BOOKING_SCHEDULE='2=18:00,19:30,21:00;4=18:00,19:30,21:00;6=12:00,13:30,15:00'
```

После создания админа `ADMIN_PASSWORD` рекомендуется удалить из env-файла. Сам хэш останется в PostgreSQL.

Если PostgreSQL расположен на том же VPS, не открывайте порт `5432` в интернет. Для удалённого PostgreSQL включайте TLS (`sslmode=require`/`verify-full`) и ограничивайте доступ firewall/private network.

## Проверки

Без тестовой PostgreSQL:

```bash
go test ./...
go vet ./...
go build -o cosmic-portfolio .
```

`github.com/lib/pq` — Go-драйвер PostgreSQL; системные `libpq-dev`, `pkg-config` и CGO для этой версии не нужны.

Интеграционные тесты PostgreSQL запускаются, если задана отдельная тестовая база:

```bash
TEST_DATABASE_URL='postgres://kirill_study_test:password@127.0.0.1:5432/kirill_study_test?sslmode=disable' go test ./...
```

**Не указывайте production-базу в `TEST_DATABASE_URL`: интеграционные тесты очищают тестовые таблицы.**

## Backup PostgreSQL

```bash
sudo install -d -m 0700 /var/backups/kirill-study
sudo -u postgres pg_dump -Fc kirill_study > /var/backups/kirill-study/kirill_study-$(date +%F-%H%M).dump
```

Восстановление в пустую базу:

```bash
sudo -u postgres pg_restore --clean --if-exists -d kirill_study /path/to/backup.dump
```

Полная инструкция: `deploy/SERVER_SETUP.md`.


## Separate cabinets

Student and admin dashboards use a separate cabinet layout with a direct link back to `/study`. Student tabs: overview, schedule, materials, transfers, security, notifications. Admin tabs: overview, schedule, transfers, students, notifications.
