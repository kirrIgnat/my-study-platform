package app

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func updateBookingStatus(db *AppDB, bookingID, status string) error {
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
		if rows[0][1] != status {
			return createNotificationTx(tx, rows[0][0], "lesson_status", "Статус занятия изменён", fmt.Sprintf("Занятие %s в %s: новый статус — %s.", rows[0][2], rows[0][3], status), bookingID, "", now)
		}
		return nil
	})
}

func updateBookingAdmin(db *AppDB, bookingID, status, meetingURL string) error {
	if status != "confirmed" && status != "cancelled" && status != "done" && status != "pending" {
		return errors.New("invalid booking status")
	}
	meetingURL = strings.TrimSpace(meetingURL)
	if len(meetingURL) > 2048 || (meetingURL != "" && !validHTTPURL(meetingURL)) {
		return errors.New("некорректная ссылка на конференцию")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT user_id,status,meeting_url,date,time FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		if err := tx.exec(`UPDATE bookings SET status=?,meeting_url=?,updated_at=? WHERE id=?`, status, meetingURL, now, bookingID); err != nil {
			return err
		}
		if rows[0][1] != status || rows[0][2] != meetingURL {
			body := fmt.Sprintf("Занятие %s в %s обновлено.", rows[0][3], rows[0][4])
			if meetingURL != "" && rows[0][2] != meetingURL {
				body += " Ссылка на конференцию доступна в личном кабинете."
			}
			return createNotificationTx(tx, rows[0][0], "lesson_updated", "Занятие обновлено", body, bookingID, "", now)
		}
		return nil
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
