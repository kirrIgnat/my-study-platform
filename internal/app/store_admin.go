package app

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func updateBookingStatus(db *AppDB, adminID, bookingID, status string) error {
	if status != "confirmed" && status != "cancelled" && status != "done" && status != "pending" {
		return errors.New("invalid booking status")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT user_id,status,date,time FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil || len(rows) == 0 {
			return err
		}
		if err := tx.exec(`UPDATE bookings SET status=?,updated_at=? WHERE id=?`, status, now, bookingID); err != nil {
			return err
		}
		if status == "cancelled" {
			if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note='Занятие отменено преподавателем',resolved_at=?,resolved_by=? WHERE booking_id=? AND status='pending'`, now, adminID, bookingID); err != nil {
				return err
			}
		}
		if rows[0][1] != status {
			return createNotificationTx(tx, rows[0][0], "lesson_status", "Статус занятия изменён", fmt.Sprintf("Занятие %s в %s: новый статус — %s.", rows[0][2], rows[0][3], status), bookingID, "", now)
		}
		return nil
	})
}

func updateBookingAdmin(db *AppDB, adminID, bookingID, status, meetingURL, date, clock string, loc *time.Location, allowed map[time.Weekday][]string) error {
	if status != "confirmed" && status != "cancelled" && status != "done" && status != "pending" {
		return errors.New("invalid booking status")
	}
	meetingURL = strings.TrimSpace(meetingURL)
	date = strings.TrimSpace(date)
	clock = strings.TrimSpace(clock)
	if len(meetingURL) > 2048 || (meetingURL != "" && !validHTTPURL(meetingURL)) {
		return errors.New("некорректная ссылка на конференцию")
	}
	parsedDate, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return errors.New("некорректная дата занятия")
	}
	if _, err := time.Parse("15:04", clock); err != nil {
		return errors.New("некорректное время занятия")
	}
	target, err := combineDateAndTime(parsedDate, clock, loc)
	if err != nil {
		return errors.New("некорректное время занятия")
	}

	rows, err := db.Query(`SELECT user_id,status,meeting_url,date,time,COALESCE(schedule_id,''),COALESCE(occurrence_key,''),slot_start FROM bookings WHERE id=? LIMIT 1`, bookingID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("занятие не найдено")
	}
	current := rows[0]
	moving := current[3] != date || current[4] != clock
	if moving {
		effectiveAllowed, err := effectiveTeacherSchedule(db, loc, allowed)
		if err != nil {
			return err
		}
		if !recurringSlotAllowed(effectiveAllowed, weekdayNumber(parsedDate.Weekday()), clock) {
			return errors.New("это время не включено в ваши слоты преподавателя")
		}
		if target.Before(time.Now().In(loc).Add(-5 * time.Minute)) {
			return errors.New("нельзя перенести занятие в прошлое")
		}
	}
	resultActive := status == "pending" || status == "confirmed"
	if resultActive {
		claimed, err := slotClaimedByRecurringSchedule(db, weekdayNumber(parsedDate.Weekday()), clock, loc.String(), current[5])
		if err != nil {
			return err
		}
		if claimed {
			return errors.New("выбранное время пересекается с постоянным занятием другого ученика")
		}
		busy, err := activeBookingAtSlot(db, target.UTC().Unix(), bookingID)
		if err != nil {
			return err
		}
		if busy {
			return errors.New("выбранное время пересекается с другим занятием")
		}
	}

	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		if resultActive {
			recurringRows, err := tx.query(`SELECT id,weekday,time FROM tutoring_schedules WHERE timezone=? AND status='active' AND id<>?`, loc.String(), current[5])
			if err != nil {
				return err
			}
			for _, row := range recurringRows {
				if recurringSlotsOverlap(parseInt(row[1]), row[2], weekdayNumber(parsedDate.Weekday()), clock) {
					return errStudySlotBusy
				}
			}

			durationSeconds := int64(lessonDuration / time.Second)
			busyRows, err := tx.query(`SELECT id FROM bookings WHERE status IN ('pending','confirmed') AND id<>? AND slot_start>? AND slot_start<? LIMIT 1`, bookingID, target.UTC().Unix()-durationSeconds, target.UTC().Unix()+durationSeconds)
			if err != nil {
				return err
			}
			if len(busyRows) > 0 {
				return errStudySlotBusy
			}
		}
		rescheduledValue := 0
		if moving {
			rescheduledValue = 1
		}
		if err := tx.exec(`UPDATE bookings SET status=?,meeting_url=?,slot_start=?,date=?,time=?,week_key=?,
			original_date=CASE WHEN ?=1 AND original_date='' THEN ? ELSE original_date END,
			original_time=CASE WHEN ?=1 AND original_time='' THEN ? ELSE original_time END,
			rescheduled=CASE WHEN ?=1 THEN 1 ELSE rescheduled END,updated_at=? WHERE id=?`,
			status, meetingURL, target.UTC().Unix(), date, clock, bookingWeekKey(parsedDate),
			rescheduledValue, current[3], rescheduledValue, current[4], rescheduledValue, now, bookingID); err != nil {
			if isDBConstraint(err) {
				return errStudySlotBusy
			}
			return err
		}
		if status == "cancelled" {
			if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note='Занятие отменено преподавателем',resolved_at=?,resolved_by=? WHERE booking_id=? AND status='pending'`, now, adminID, bookingID); err != nil {
				return err
			}
		}
		if current[1] != status || current[2] != meetingURL || moving {
			body := fmt.Sprintf("Занятие %s в %s обновлено.", date, clock)
			if moving {
				body = fmt.Sprintf("Занятие перенесено: %s %s → %s %s. Длительность — %d минут.", current[3], current[4], date, clock, lessonDurationMinutes)
			}
			if meetingURL != "" && current[2] != meetingURL {
				body += " Ссылка на конференцию доступна в личном кабинете."
			}
			return createNotificationTx(tx, current[0], "lesson_updated", "Занятие обновлено", body, bookingID, "", now)
		}
		return nil
	})
}

func deleteBookingAdmin(db *AppDB, adminID, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не найдено")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT user_id,date,time,COALESCE(schedule_id,''),COALESCE(occurrence_key,''),COALESCE(original_date,''),COALESCE(original_time,'') FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		row := rows[0]
		scheduleID := row[3]
		occurrenceKey := row[4]

		if scheduleID != "" {
			if occurrenceKey == "" {
				originalDate := row[5]
				if originalDate == "" {
					originalDate = row[1]
				}
				parsedDate, parseErr := time.Parse("2006-01-02", originalDate)
				if parseErr != nil {
					return errors.New("не удалось определить регулярное занятие")
				}
				occurrenceKey = bookingWeekKey(parsedDate)
			}

			// Deleting one occurrence of a recurring lesson means ending this
			// recurring schedule from the selected occurrence onward. Past
			// lessons remain in history, while the paused schedule can no longer
			// generate new bookings.
			if err := tx.exec(`UPDATE tutoring_schedules SET status='paused',updated_at=? WHERE id=?`, now, scheduleID); err != nil {
				return err
			}
			if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note='Регулярное расписание завершено преподавателем',resolved_at=?,resolved_by=?
				WHERE status='pending' AND ((schedule_id=? AND kind='permanent') OR booking_id IN (
					SELECT id FROM bookings WHERE schedule_id=? AND occurrence_key>=?
				))`, now, adminID, scheduleID, scheduleID, occurrenceKey); err != nil {
				return err
			}
			if err := tx.exec(`DELETE FROM bookings WHERE schedule_id=? AND occurrence_key>=?`, scheduleID, occurrenceKey); err != nil {
				return err
			}

			originalTime := row[6]
			if originalTime == "" {
				originalTime = row[2]
			}
			body := fmt.Sprintf("Регулярное занятие с %s в %s удалено вместе со всеми последующими занятиями этого слота.", row[1], originalTime)
			return createNotificationTx(tx, row[0], "schedule_ended", "Регулярное расписание завершено", body, "", "", now)
		}

		if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note='Занятие удалено преподавателем',resolved_at=?,resolved_by=? WHERE booking_id=? AND status='pending'`, now, adminID, bookingID); err != nil {
			return err
		}
		if err := tx.exec(`DELETE FROM bookings WHERE id=?`, bookingID); err != nil {
			return err
		}
		body := fmt.Sprintf("Занятие %s в %s удалено преподавателем из расписания.", row[1], row[2])
		return createNotificationTx(tx, row[0], "lesson_deleted", "Занятие удалено", body, "", "", now)
	})
}

func validHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func listNotifications(db *AppDB, recipientID string) ([]StudyNotification, error) {
	rows, err := db.Query(`SELECT id,type,title,body,COALESCE(booking_id,''),COALESCE(reschedule_request_id,''),read_at,created_at FROM notifications
		WHERE recipient_user_id=? ORDER BY created_at DESC LIMIT 120`, recipientID)
	if err != nil {
		return nil, err
	}
	result := make([]StudyNotification, 0, len(rows))
	for _, row := range rows {
		result = append(result, StudyNotification{ID: row[0], Type: row[1], Title: row[2], Body: row[3], BookingID: row[4], RescheduleRequestID: row[5], Read: row[6] != "", CreatedAt: time.Unix(parseInt64(row[7]), 0).UTC()})
	}
	return result, nil
}

func unreadNotificationCount(db *AppDB, recipientID string) (int, error) {
	rows, err := db.Query(`SELECT COUNT(*) FROM notifications WHERE recipient_user_id=? AND read_at IS NULL`, recipientID)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return parseInt(rows[0][0]), nil
}

func markNotificationRead(db *AppDB, recipientID, notificationID string) error {
	return db.Exec(`UPDATE notifications SET read_at=? WHERE id=? AND recipient_user_id=?`, time.Now().UTC().Unix(), notificationID, recipientID)
}

func markAllNotificationsRead(db *AppDB, recipientID string) error {
	return db.Exec(`UPDATE notifications SET read_at=? WHERE recipient_user_id=? AND read_at IS NULL`, time.Now().UTC().Unix(), recipientID)
}

func adminStats(db *AppDB, now time.Time, adminID string) (AdminStats, error) {
	rows, err := db.Query(`SELECT
		(SELECT COUNT(*) FROM users WHERE role='student'),
		(SELECT COUNT(*) FROM bookings),
		(SELECT COUNT(*) FROM bookings WHERE slot_start>=? AND status IN ('pending','confirmed')),
		(SELECT COUNT(*) FROM notifications WHERE recipient_user_id=? AND read_at IS NULL),
		(SELECT COUNT(*) FROM lesson_materials WHERE published=1),
		(SELECT COUNT(*) FROM reschedule_requests WHERE status='pending')`, now.UTC().Unix(), adminID)
	if err != nil || len(rows) == 0 {
		return AdminStats{}, err
	}
	row := rows[0]
	return AdminStats{Students: parseInt(row[0]), Bookings: parseInt(row[1]), Upcoming: parseInt(row[2]), Unread: parseInt(row[3]), PublishedLessons: parseInt(row[4]), PendingChanges: parseInt(row[5])}, nil
}

func listStudents(db *AppDB) ([]StudyUser, error) {
	rows, err := db.Query(`SELECT id,email,name,telegram,role,created_at FROM users WHERE role='student' ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	result := make([]StudyUser, 0, len(rows))
	for _, row := range rows {
		result = append(result, StudyUser{ID: row[0], Email: row[1], Name: row[2], Telegram: row[3], Role: row[4], Created: time.Unix(parseInt64(row[5]), 0).UTC()})
	}
	return result, nil
}
