package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func migrateStudyDB(db *AppDB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            applied_at BIGINT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS users (
            id TEXT PRIMARY KEY,
            email TEXT NOT NULL,
            password_hash TEXT NOT NULL,
            name TEXT NOT NULL,
            telegram TEXT NOT NULL DEFAULT '',
            role TEXT NOT NULL CHECK(role IN ('student','admin')),
            created_at BIGINT NOT NULL
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_unique ON users ((lower(email)))`,
		`CREATE TABLE IF NOT EXISTS student_profiles (
            user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
            timezone TEXT NOT NULL DEFAULT 'Europe/Moscow',
            notes TEXT NOT NULL DEFAULT '',
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS sessions (
            token_hash TEXT PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            csrf_token TEXT NOT NULL,
            expires_at BIGINT NOT NULL,
            created_at BIGINT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS courses (
            id TEXT PRIMARY KEY,
            slug TEXT NOT NULL UNIQUE,
            title TEXT NOT NULL,
            description TEXT NOT NULL,
            level TEXT NOT NULL,
            duration TEXT NOT NULL,
            status TEXT NOT NULL CHECK(status IN ('open','soon','archived')),
            sort_order INTEGER NOT NULL DEFAULT 0,
            created_at BIGINT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS enrollments (
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            course_id TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
            status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','completed','paused')),
            progress INTEGER NOT NULL DEFAULT 0 CHECK(progress >= 0 AND progress <= 100),
            created_at BIGINT NOT NULL,
            PRIMARY KEY(user_id, course_id)
        )`,
		`CREATE TABLE IF NOT EXISTS tutoring_schedules (
            id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            weekday INTEGER NOT NULL CHECK(weekday BETWEEN 1 AND 7),
            time TEXT NOT NULL,
            timezone TEXT NOT NULL,
            topic TEXT NOT NULL DEFAULT '',
            meeting_url TEXT NOT NULL DEFAULT '',
            effective_from TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused')),
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS tutoring_schedules_active_slot_unique
            ON tutoring_schedules(weekday,time,timezone) WHERE status='active'`,
		`ALTER TABLE tutoring_schedules DROP CONSTRAINT IF EXISTS tutoring_schedules_user_id_key`,
		`CREATE UNIQUE INDEX IF NOT EXISTS tutoring_schedules_user_slot_unique
            ON tutoring_schedules(user_id,weekday,time,timezone) WHERE status='active'`,
		`CREATE INDEX IF NOT EXISTS tutoring_schedules_user_idx ON tutoring_schedules(user_id,status)`,
		`CREATE TABLE IF NOT EXISTS bookings (
            id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            slot_start BIGINT NOT NULL,
            date TEXT NOT NULL,
            time TEXT NOT NULL,
            timezone TEXT NOT NULL,
            topic TEXT NOT NULL DEFAULT '',
            meeting_url TEXT NOT NULL DEFAULT '',
            week_key TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','confirmed','cancelled','done')),
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL,
            schedule_id TEXT NOT NULL DEFAULT '',
            occurrence_key TEXT NOT NULL DEFAULT '',
            original_date TEXT NOT NULL DEFAULT '',
            original_time TEXT NOT NULL DEFAULT '',
            rescheduled INTEGER NOT NULL DEFAULT 0 CHECK(rescheduled IN (0,1))
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS bookings_active_slot_unique
            ON bookings(slot_start) WHERE status IN ('pending','confirmed')`,
		`CREATE INDEX IF NOT EXISTS bookings_user_idx ON bookings(user_id, slot_start DESC)`,
		`CREATE INDEX IF NOT EXISTS bookings_user_week_idx ON bookings(user_id, week_key, status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS bookings_schedule_occurrence_unique
            ON bookings(schedule_id,occurrence_key) WHERE schedule_id<>''`,
		`CREATE TABLE IF NOT EXISTS reschedule_requests (
            id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            schedule_id TEXT NOT NULL REFERENCES tutoring_schedules(id) ON DELETE CASCADE,
            booking_id TEXT REFERENCES bookings(id) ON DELETE SET NULL,
            kind TEXT NOT NULL CHECK(kind IN ('temporary','permanent')),
            requested_weekday INTEGER,
            requested_time TEXT NOT NULL,
            requested_slot_start BIGINT,
            requested_date TEXT NOT NULL DEFAULT '',
            month_key TEXT NOT NULL DEFAULT '',
            reason TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
            teacher_note TEXT NOT NULL DEFAULT '',
            created_at BIGINT NOT NULL,
            resolved_at BIGINT,
            resolved_by TEXT REFERENCES users(id) ON DELETE SET NULL
        )`,
		`CREATE INDEX IF NOT EXISTS reschedule_requests_user_idx ON reschedule_requests(user_id,created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS reschedule_requests_status_idx ON reschedule_requests(status,created_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS reschedule_requests_temp_pending_unique
            ON reschedule_requests(booking_id) WHERE kind='temporary' AND status='pending'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS reschedule_requests_perm_pending_unique
            ON reschedule_requests(schedule_id) WHERE kind='permanent' AND status='pending'`,
		`CREATE TABLE IF NOT EXISTS notifications (
            id TEXT PRIMARY KEY,
            recipient_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            type TEXT NOT NULL,
            title TEXT NOT NULL,
            body TEXT NOT NULL,
            booking_id TEXT REFERENCES bookings(id) ON DELETE SET NULL,
            reschedule_request_id TEXT REFERENCES reschedule_requests(id) ON DELETE SET NULL,
            read_at BIGINT,
            created_at BIGINT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS notifications_recipient_idx
            ON notifications(recipient_user_id, read_at, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS lesson_materials (
            booking_id TEXT PRIMARY KEY REFERENCES bookings(id) ON DELETE CASCADE,
            title TEXT NOT NULL DEFAULT '',
            theory TEXT NOT NULL DEFAULT '',
            homework TEXT NOT NULL DEFAULT '',
            video_url TEXT NOT NULL DEFAULT '',
            published INTEGER NOT NULL DEFAULT 0 CHECK(published IN (0,1)),
            published_at BIGINT,
            updated_at BIGINT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS lesson_materials_published_idx ON lesson_materials(published, published_at DESC)`,
		`CREATE TABLE IF NOT EXISTS test_templates (
            id TEXT PRIMARY KEY,
            slug TEXT NOT NULL UNIQUE,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            duration_minutes INTEGER NOT NULL DEFAULT 20 CHECK(duration_minutes BETWEEN 1 AND 480),
            status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('draft','active','archived')),
            created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS test_questions (
            id TEXT PRIMARY KEY,
            test_id TEXT NOT NULL REFERENCES test_templates(id) ON DELETE CASCADE,
            sort_order INTEGER NOT NULL,
            section TEXT NOT NULL DEFAULT '',
            prompt TEXT NOT NULL,
            code TEXT NOT NULL DEFAULT '',
            kind TEXT NOT NULL CHECK(kind IN ('single','multiple','survey_single','survey_multiple')),
            options_json TEXT NOT NULL,
            correct_json TEXT NOT NULL DEFAULT '[]',
            explanation TEXT NOT NULL DEFAULT '',
            category TEXT NOT NULL DEFAULT '',
            scored INTEGER NOT NULL DEFAULT 1 CHECK(scored IN (0,1))
        )`,
		`CREATE UNIQUE INDEX IF NOT EXISTS test_questions_order_unique ON test_questions(test_id,sort_order)`,
		`CREATE TABLE IF NOT EXISTS test_assignments (
            id TEXT PRIMARY KEY,
            test_id TEXT NOT NULL REFERENCES test_templates(id) ON DELETE CASCADE,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            booking_id TEXT REFERENCES bookings(id) ON DELETE SET NULL,
            attach_mode TEXT NOT NULL DEFAULT 'direct' CHECK(attach_mode IN ('direct','lesson','homework')),
            time_limit_minutes INTEGER NOT NULL CHECK(time_limit_minutes BETWEEN 1 AND 480),
            deadline_at BIGINT,
            assigned_by TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
            status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
            created_at BIGINT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS test_assignments_user_idx ON test_assignments(user_id,status,created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS test_assignments_booking_idx ON test_assignments(booking_id,created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS test_attempts (
            id TEXT PRIMARY KEY,
            assignment_id TEXT NOT NULL UNIQUE REFERENCES test_assignments(id) ON DELETE CASCADE,
            user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            status TEXT NOT NULL CHECK(status IN ('in_progress','submitted','expired')),
            started_at BIGINT NOT NULL,
            expires_at BIGINT,
            submitted_at BIGINT,
            answers_json TEXT NOT NULL DEFAULT '{}',
            result_json TEXT NOT NULL DEFAULT '{}',
            score INTEGER NOT NULL DEFAULT 0,
            max_score INTEGER NOT NULL DEFAULT 0,
            updated_at BIGINT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS test_attempts_user_idx ON test_attempts(user_id,status,updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS password_reset_events (
            id TEXT PRIMARY KEY,
            admin_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            target_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            created_at BIGINT NOT NULL
        )`,
		`INSERT INTO student_profiles(user_id,timezone,created_at,updated_at)
            SELECT id,'Europe/Moscow',created_at,created_at FROM users WHERE role='student'
            ON CONFLICT(user_id) DO NOTHING`,
	}
	return db.WithTx(func(tx *dbTx) error {
		for _, statement := range statements {
			if err := tx.exec(statement); err != nil {
				return fmt.Errorf("postgres migration failed: %w", err)
			}
		}
		if err := backfillBookingWeekKeys(tx); err != nil {
			return err
		}
		now := time.Now().UTC().Unix()
		if err := seedDiagnosticTest(tx, now); err != nil {
			return fmt.Errorf("seed diagnostic test: %w", err)
		}
		if err := tx.exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?) ON CONFLICT(version) DO NOTHING`, 1, now); err != nil {
			return err
		}
		if err := tx.exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?) ON CONFLICT(version) DO NOTHING`, 2, now); err != nil {
			return err
		}
		return tx.exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?) ON CONFLICT(version) DO NOTHING`, 3, now)
	})
}

func backfillBookingWeekKeys(db dbExecutor) error {
	rows, err := db.query(`SELECT id,date FROM bookings WHERE week_key='' OR week_key IS NULL`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		date, err := time.Parse("2006-01-02", row[1])
		if err != nil {
			continue
		}
		if err := db.exec(`UPDATE bookings SET week_key=? WHERE id=?`, bookingWeekKey(date), row[0]); err != nil {
			return err
		}
	}
	return nil
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
