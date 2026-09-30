package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxPaymentQRBytes = 1024 * 1024

func paymentStatusLabel(status string) string {
	switch strings.TrimSpace(status) {
	case "approved":
		return "Оплачено"
	case "pending":
		return "На подтверждении"
	case "rejected":
		return "Не подтверждено"
	default:
		return "Не оплачено"
	}
}

func qrDataURLFromBytes(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("выберите изображение QR-кода")
	}
	if len(data) > maxPaymentQRBytes {
		return "", errors.New("QR-код должен быть не больше 1 МБ")
	}
	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return "", errors.New("поддерживаются только PNG, JPEG и WebP")
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func saveLessonPaymentQRCode(db *AppDB, adminID, bookingID, dataURL string) error {
	bookingID = strings.TrimSpace(bookingID)
	dataURL = strings.TrimSpace(dataURL)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	if dataURL == "" {
		return errors.New("QR-код пуст")
	}
	rows, err := db.Query(`SELECT b.status,CASE WHEN x.booking_id IS NULL THEN 0 ELSE 1 END FROM bookings b LEFT JOIN lesson_payment_exclusions x ON x.booking_id=b.id WHERE b.id=? LIMIT 1`, bookingID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("занятие не найдено")
	}
	if rows[0][1] == "1" {
		return errors.New("это занятие исключено из оплаты")
	}
	if rows[0][0] != "done" {
		return errors.New("QR-код можно добавить только после того, как занятие отмечено как проведённое")
	}
	return db.Exec(`INSERT INTO lesson_payment_qr(booking_id,qr_data_url,updated_by,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(booking_id) DO UPDATE SET qr_data_url=excluded.qr_data_url,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, bookingID, dataURL, adminID, time.Now().UTC().Unix())
}

func clearLessonPaymentQRCode(db *AppDB, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	return db.Exec(`DELETE FROM lesson_payment_qr WHERE booking_id=?`, bookingID)
}

func submitLessonPayment(db *AppDB, user StudyUser, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT b.date,b.time,b.topic,b.status,COALESCE(q.qr_data_url,''),CASE WHEN x.booking_id IS NULL THEN 0 ELSE 1 END
			FROM bookings b LEFT JOIN lesson_payment_qr q ON q.booking_id=b.id
			LEFT JOIN lesson_payment_exclusions x ON x.booking_id=b.id
			WHERE b.id=? AND b.user_id=? LIMIT 1`, bookingID, user.ID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		if rows[0][5] == "1" {
			return errors.New("это занятие исключено из оплаты")
		}
		if rows[0][3] != "done" {
			return errors.New("оплата становится доступна только после проведённого занятия")
		}
		if strings.TrimSpace(rows[0][4]) == "" {
			return errors.New("преподаватель ещё не загрузил QR-код для этого занятия")
		}
		existing, err := tx.query(`SELECT id,status FROM lesson_payments WHERE booking_id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			switch existing[0][1] {
			case "approved":
				return errors.New("это занятие уже отмечено как оплаченное")
			case "pending":
				return nil
			default:
				if err := tx.exec(`UPDATE lesson_payments SET status='pending',teacher_note='',requested_at=?,reviewed_at=NULL,reviewed_by=NULL,updated_at=? WHERE id=?`, now, now, existing[0][0]); err != nil {
					return err
				}
			}
		} else {
			id, err := randomToken(12)
			if err != nil {
				return err
			}
			if err := tx.exec(`INSERT INTO lesson_payments(id,booking_id,user_id,status,requested_at,updated_at) VALUES(?,?,?,'pending',?,?)`, id, bookingID, user.ID, now, now); err != nil {
				return err
			}
		}
		topic := strings.TrimSpace(rows[0][2])
		if topic == "" {
			topic = "персональное занятие"
		}
		body := fmt.Sprintf("%s отметил оплату занятия %s в %s (%s). Подтвердите поступление оплаты.", user.Name, rows[0][0], rows[0][1], topic)
		return notifyAdminsTx(tx, "payment_pending", "Оплата ожидает подтверждения", body, bookingID, "", now)
	})
}

func deleteLessonPayment(db *AppDB, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT id FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		payments, err := tx.query(`SELECT id FROM lesson_payments WHERE booking_id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(payments) == 0 {
			return errors.New("у этого занятия нет сохранённой оплаты")
		}
		if err := tx.exec(`DELETE FROM lesson_payments WHERE booking_id=?`, bookingID); err != nil {
			return err
		}
		return tx.exec(`DELETE FROM notifications WHERE booking_id=? AND type IN ('payment_pending','payment_approved','payment_rejected')`, bookingID)
	})
}

func excludeLessonFromPayments(db *AppDB, adminID, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT id FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		if err := tx.exec(`INSERT INTO lesson_payment_exclusions(booking_id,excluded_by,excluded_at) VALUES(?,?,?)
			ON CONFLICT(booking_id) DO UPDATE SET excluded_by=excluded.excluded_by,excluded_at=excluded.excluded_at`, bookingID, adminID, now); err != nil {
			return err
		}
		if err := tx.exec(`DELETE FROM lesson_payments WHERE booking_id=?`, bookingID); err != nil {
			return err
		}
		if err := tx.exec(`DELETE FROM lesson_payment_qr WHERE booking_id=?`, bookingID); err != nil {
			return err
		}
		return tx.exec(`DELETE FROM notifications WHERE booking_id=? AND type IN ('payment_pending','payment_approved','payment_rejected')`, bookingID)
	})
}

func resolveLessonPayment(db *AppDB, adminID, paymentID, decision, teacherNote string) error {
	paymentID = strings.TrimSpace(paymentID)
	decision = strings.TrimSpace(decision)
	teacherNote = strings.TrimSpace(teacherNote)
	if paymentID == "" {
		return errors.New("заявка на оплату не выбрана")
	}
	if decision != "approve" && decision != "reject" {
		return errors.New("неизвестное решение")
	}
	if len(teacherNote) > 1000 {
		return errors.New("комментарий слишком длинный")
	}
	status := "approved"
	title := "Оплата подтверждена"
	if decision == "reject" {
		status = "rejected"
		title = "Оплата не подтверждена"
	}
	now := time.Now().UTC().Unix()
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT p.user_id,p.booking_id,p.status,b.date,b.time FROM lesson_payments p JOIN bookings b ON b.id=p.booking_id WHERE p.id=? LIMIT 1`, paymentID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("заявка на оплату не найдена")
		}
		if rows[0][2] != "pending" {
			return errors.New("эта заявка уже обработана")
		}
		if err := tx.exec(`UPDATE lesson_payments SET status=?,teacher_note=?,reviewed_at=?,reviewed_by=?,updated_at=? WHERE id=?`, status, teacherNote, now, adminID, now, paymentID); err != nil {
			return err
		}
		body := fmt.Sprintf("Занятие %s в %s: %s.", rows[0][3], rows[0][4], strings.ToLower(title))
		if teacherNote != "" {
			body += " Комментарий преподавателя: " + teacherNote
		}
		if err := createNotificationTx(tx, rows[0][0], "payment_"+status, title, body, rows[0][1], "", now); err != nil {
			return err
		}
		return tx.exec(`UPDATE notifications SET read_at=? WHERE recipient_user_id=? AND booking_id=? AND type='payment_pending' AND read_at IS NULL`, now, adminID, rows[0][1])
	})
}

func listLessonPaymentsForAdmin(db *AppDB, loc *time.Location) ([]LessonPayment, error) {
	rows, err := db.Query(`SELECT p.id,p.booking_id,p.user_id,u.name,u.email,b.date,b.time,b.topic,p.status,p.teacher_note,p.requested_at,COALESCE(p.reviewed_at,0)
		FROM lesson_payments p JOIN bookings b ON b.id=p.booking_id JOIN users u ON u.id=p.user_id
		LEFT JOIN lesson_payment_exclusions x ON x.booking_id=p.booking_id
		WHERE x.booking_id IS NULL
		ORDER BY CASE WHEN p.status='pending' THEN 0 ELSE 1 END,p.requested_at DESC LIMIT 300`)
	if err != nil {
		return nil, err
	}
	result := make([]LessonPayment, 0, len(rows))
	for _, row := range rows {
		payment := LessonPayment{
			ID:          row[0],
			BookingID:   row[1],
			UserID:      row[2],
			UserName:    row[3],
			UserEmail:   row[4],
			Date:        row[5],
			Time:        row[6],
			EndTime:     lessonEndTime(row[6]),
			Topic:       row[7],
			Status:      row[8],
			StatusLabel: paymentStatusLabel(row[8]),
			TeacherNote: row[9],
			RequestedAt: time.Unix(parseInt64(row[10]), 0).In(loc),
		}
		if ts := parseInt64(row[11]); ts > 0 {
			payment.ReviewedAt = time.Unix(ts, 0).In(loc)
			payment.Reviewed = true
		}
		result = append(result, payment)
	}
	return result, nil
}

func sortStudentPaymentBookings(bookings []StudyBooking, now time.Time) []StudyBooking {
	result := make([]StudyBooking, 0, len(bookings))
	for _, booking := range bookings {
		if booking.Status != "done" || booking.PaymentExcluded {
			continue
		}
		result = append(result, booking)
	}
	sort.SliceStable(result, func(i, j int) bool {
		iFuture := !result[i].SlotStart.Before(now)
		jFuture := !result[j].SlotStart.Before(now)
		if iFuture != jFuture {
			return iFuture
		}
		if iFuture {
			return result[i].SlotStart.Before(result[j].SlotStart)
		}
		return result[i].SlotStart.After(result[j].SlotStart)
	})
	return result
}
