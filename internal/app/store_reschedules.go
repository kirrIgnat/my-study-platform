package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func requestTemporaryReschedule(db *AppDB, user StudyUser, bookingID, date, clock, reason string, loc *time.Location, allowed map[time.Weekday][]string) (RescheduleRequest, error) {
	booking, err := getBookingForUser(db, bookingID, user.ID, loc)
	if err != nil {
		return RescheduleRequest{}, err
	}
	if booking.ScheduleID == "" || (booking.Status != "confirmed" && booking.Status != "pending") || !booking.SlotStart.After(time.Now().In(loc)) {
		return RescheduleRequest{}, errors.New("это занятие уже нельзя переносить")
	}
	parsedDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), loc)
	if err != nil {
		return RescheduleRequest{}, errors.New("некорректная дата")
	}
	if bookingWeekKey(parsedDate) != booking.WeekKey {
		return RescheduleRequest{}, errors.New("разовый перенос доступен только внутри недели этого занятия")
	}
	if err := validateStudySlot(date, clock, loc, allowed); err != nil {
		return RescheduleRequest{}, err
	}
	target, _ := combineDateAndTime(parsedDate, clock, loc)
	if target.UTC().Unix() == booking.SlotStart.UTC().Unix() {
		return RescheduleRequest{}, errors.New("выберите другое время")
	}
	claimed, err := slotClaimedByRecurringSchedule(db, weekdayNumber(parsedDate.Weekday()), clock, loc.String(), booking.ScheduleID)
	if err != nil {
		return RescheduleRequest{}, err
	}
	if claimed {
		return RescheduleRequest{}, errStudyRecurringSlotBusy
	}
	busy, err := activeBookingAtSlot(db, target.UTC().Unix(), booking.ID)
	if err != nil {
		return RescheduleRequest{}, err
	}
	if busy {
		return RescheduleRequest{}, errStudySlotBusy
	}
	monthKey := temporaryMoveMonthKey(booking)
	used, err := temporaryMoveUsage(db, user.ID, monthKey)
	if err != nil {
		return RescheduleRequest{}, err
	}
	if used >= 2 {
		return RescheduleRequest{}, errTemporaryMoveLimitReached
	}
	id, err := randomToken(12)
	if err != nil {
		return RescheduleRequest{}, err
	}
	now := time.Now().UTC()
	reason = strings.TrimSpace(reason)
	request := RescheduleRequest{ID: id, UserID: user.ID, UserName: user.Name, UserEmail: user.Email, ScheduleID: booking.ScheduleID, BookingID: booking.ID, Kind: "temporary", RequestedWeekday: weekdayNumber(parsedDate.Weekday()), RequestedDayName: weekdayNameRU(weekdayNumber(parsedDate.Weekday())), RequestedTime: clock, RequestedDate: date, RequestedSlot: target, MonthKey: monthKey, Reason: reason, Status: "pending", OriginalDate: booking.Date, OriginalTime: booking.Time, CreatedAt: now}
	err = db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT id FROM reschedule_requests WHERE booking_id=? AND kind='temporary' AND status='pending' LIMIT 1`, booking.ID)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return errRescheduleAlreadyPending
		}
		if err := tx.exec(`INSERT INTO reschedule_requests(id,user_id,schedule_id,booking_id,kind,requested_weekday,requested_time,requested_slot_start,requested_date,month_key,reason,status,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,'pending',?)`, request.ID, user.ID, booking.ScheduleID, booking.ID, "temporary", request.RequestedWeekday, clock, target.UTC().Unix(), date, monthKey, reason, now.Unix()); err != nil {
			if isDBConstraint(err) {
				return errRescheduleAlreadyPending
			}
			return err
		}
		body := fmt.Sprintf("%s просит разово перенести занятие %s %s на %s %s. Использовано переносов в %s: %d/2.", user.Name, booking.Date, booking.Time, date, clock, monthKey, used+1)
		if reason != "" {
			body += " Причина: " + reason
		}
		return notifyAdminsTx(tx, "temporary_reschedule", "Запрос на разовый перенос", body, booking.ID, request.ID, now.Unix())
	})
	return request, err
}

func listRescheduleRequestsForUser(db *AppDB, userID string, loc *time.Location) ([]RescheduleRequest, error) {
	return listRescheduleRequests(db, `WHERE r.user_id=?`, []any{userID}, loc)
}

func listPendingRescheduleRequests(db *AppDB, loc *time.Location) ([]RescheduleRequest, error) {
	return listRescheduleRequests(db, `WHERE r.status='pending'`, nil, loc)
}

func listRescheduleRequests(db *AppDB, where string, args []any, loc *time.Location) ([]RescheduleRequest, error) {
	query := `SELECT r.id,r.user_id,r.schedule_id,COALESCE(r.booking_id,''),r.kind,COALESCE(r.requested_weekday,0),r.requested_time,
		COALESCE(r.requested_slot_start,0),r.requested_date,r.month_key,r.reason,r.status,r.teacher_note,r.created_at,COALESCE(r.resolved_at,0),
		u.name,u.email,COALESCE(b.date,''),COALESCE(b.time,'')
		FROM reschedule_requests r JOIN users u ON u.id=r.user_id LEFT JOIN bookings b ON b.id=r.booking_id ` + where + ` ORDER BY r.created_at DESC LIMIT 100`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	result := make([]RescheduleRequest, 0, len(rows))
	for _, row := range rows {
		day := parseInt(row[5])
		req := RescheduleRequest{ID: row[0], UserID: row[1], ScheduleID: row[2], BookingID: row[3], Kind: row[4], RequestedWeekday: day, RequestedDayName: weekdayNameRU(day), RequestedTime: row[6], RequestedDate: row[8], MonthKey: row[9], Reason: row[10], Status: row[11], TeacherNote: row[12], CreatedAt: time.Unix(parseInt64(row[13]), 0).In(loc), UserName: row[15], UserEmail: row[16], OriginalDate: row[17], OriginalTime: row[18]}
		if ts := parseInt64(row[7]); ts > 0 {
			req.RequestedSlot = time.Unix(ts, 0).In(loc)
		}
		if ts := parseInt64(row[14]); ts > 0 {
			req.ResolvedAt = time.Unix(ts, 0).In(loc)
		}
		result = append(result, req)
	}
	return result, nil
}

func resolveRescheduleRequest(db *AppDB, adminID, requestID, decision, teacherNote string, loc *time.Location, allowed map[time.Weekday][]string) error {
	if decision != "approve" && decision != "reject" {
		return errors.New("некорректное решение")
	}
	rows, err := db.Query(`SELECT r.id,r.user_id,r.schedule_id,COALESCE(r.booking_id,''),r.kind,COALESCE(r.requested_weekday,0),r.requested_time,
		COALESCE(r.requested_slot_start,0),r.requested_date,r.status,u.name
		FROM reschedule_requests r JOIN users u ON u.id=r.user_id WHERE r.id=? LIMIT 1`, requestID)
	if err != nil {
		return err
	}
	if len(rows) == 0 || rows[0][9] != "pending" {
		return errors.New("запрос уже обработан или не найден")
	}
	r := rows[0]
	userID, scheduleID, bookingID, kind := r[1], r[2], r[3], r[4]
	weekday, clock := parseInt(r[5]), r[6]
	requestedUnix, requestedDate := parseInt64(r[7]), r[8]
	studentName := r[10]
	teacherNote = strings.TrimSpace(teacherNote)
	now := time.Now().UTC()
	if decision == "reject" {
		return db.WithTx(func(tx *dbTx) error {
			if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note=?,resolved_at=?,resolved_by=? WHERE id=? AND status='pending'`, teacherNote, now.Unix(), adminID, requestID); err != nil {
				return err
			}
			body := "Преподаватель отклонил запрос на перенос."
			if teacherNote != "" {
				body += " Комментарий: " + teacherNote
			}
			if err := createNotificationTx(tx, userID, "reschedule_rejected", "Перенос не согласован", body, bookingID, requestID, now.Unix()); err != nil {
				return err
			}
			return tx.exec(`UPDATE notifications SET read_at=? WHERE recipient_user_id=? AND reschedule_request_id=? AND read_at IS NULL`, now.Unix(), adminID, requestID)
		})
	}

	effectiveAllowed, err := effectiveTeacherSchedule(db, loc, allowed)
	if err != nil {
		return err
	}
	allowed = effectiveAllowed

	if kind == "temporary" {
		if !recurringSlotAllowed(allowed, weekday, clock) || requestedUnix <= time.Now().UTC().Unix() {
			return errors.New("запрошенный слот больше недоступен")
		}
		claimed, err := slotClaimedByRecurringSchedule(db, weekday, clock, loc.String(), scheduleID)
		if err != nil {
			return err
		}
		if claimed {
			return errors.New("слот уже закреплён за другим учеником")
		}
		busy, err := activeBookingAtSlot(db, requestedUnix, bookingID)
		if err != nil {
			return err
		}
		if busy {
			return errors.New("слот уже занят другим занятием")
		}
		err = db.WithTx(func(tx *dbTx) error {
			if err := tx.exec(`UPDATE bookings SET original_date=CASE WHEN original_date='' THEN date ELSE original_date END,
				original_time=CASE WHEN original_time='' THEN time ELSE original_time END,slot_start=?,date=?,time=?,rescheduled=1,updated_at=? WHERE id=? AND user_id=?`, requestedUnix, requestedDate, clock, now.Unix(), bookingID, userID); err != nil {
				if isDBConstraint(err) {
					return errStudySlotBusy
				}
				return err
			}
			if err := tx.exec(`UPDATE reschedule_requests SET status='approved',teacher_note=?,resolved_at=?,resolved_by=? WHERE id=? AND status='pending'`, teacherNote, now.Unix(), adminID, requestID); err != nil {
				return err
			}
			body := fmt.Sprintf("Разовый перенос согласован: %s в %s.", requestedDate, clock)
			if teacherNote != "" {
				body += " Комментарий: " + teacherNote
			}
			if err := createNotificationTx(tx, userID, "reschedule_approved", "Перенос занятия согласован", body, bookingID, requestID, now.Unix()); err != nil {
				return err
			}
			return tx.exec(`UPDATE notifications SET read_at=? WHERE recipient_user_id=? AND reschedule_request_id=? AND read_at IS NULL`, now.Unix(), adminID, requestID)
		})
		return err
	}

	if kind != "permanent" || !recurringSlotAllowed(allowed, weekday, clock) {
		return errors.New("некорректный постоянный слот")
	}
	claimed, err := slotClaimedByRecurringSchedule(db, weekday, clock, loc.String(), scheduleID)
	if err != nil {
		return err
	}
	if claimed {
		return errors.New("постоянный слот уже закреплён за другим учеником")
	}
	nowLocal := time.Now().In(loc)
	currentMonday, _ := time.ParseInLocation("2006-01-02", bookingWeekKey(nowLocal), loc)
	effective := currentMonday.AddDate(0, 0, 7)
	firstNewOccurrence := nextWeekdayOnOrAfter(effective, weekday)
	effectiveFrom := firstNewOccurrence.Format("2006-01-02")
	err = db.WithTx(func(tx *dbTx) error {
		if err := tx.exec(`DELETE FROM bookings WHERE schedule_id=? AND slot_start>=? AND status IN ('pending','confirmed')`, scheduleID, effective.UTC().Unix()); err != nil {
			return err
		}
		if err := tx.exec(`UPDATE tutoring_schedules SET weekday=?,time=?,effective_from=?,updated_at=? WHERE id=? AND user_id=?`, weekday, clock, effectiveFrom, now.Unix(), scheduleID, userID); err != nil {
			if isDBConstraint(err) {
				return errStudyRecurringSlotBusy
			}
			return err
		}
		if err := tx.exec(`UPDATE reschedule_requests SET status='approved',teacher_note=?,resolved_at=?,resolved_by=? WHERE id=? AND status='pending'`, teacherNote, now.Unix(), adminID, requestID); err != nil {
			return err
		}
		if err := tx.exec(`UPDATE reschedule_requests SET status='rejected',teacher_note='Отменено из-за постоянного изменения расписания',resolved_at=?,resolved_by=? WHERE schedule_id=? AND kind='temporary' AND status='pending'`, now.Unix(), adminID, scheduleID); err != nil {
			return err
		}
		body := fmt.Sprintf("Постоянный перенос согласован. Начиная со следующей полной недели ваше время: %s в %s.", weekdayNameRU(weekday), clock)
		if teacherNote != "" {
			body += " Комментарий: " + teacherNote
		}
		if err := createNotificationTx(tx, userID, "reschedule_approved", "Новое постоянное расписание", body, "", requestID, now.Unix()); err != nil {
			return err
		}
		return tx.exec(`UPDATE notifications SET read_at=? WHERE recipient_user_id=? AND reschedule_request_id=? AND read_at IS NULL`, now.Unix(), adminID, requestID)
	})
	if err != nil {
		return err
	}
	if err := ensureRecurringBookings(db, loc, 16); err != nil {
		return err
	}
	_ = studentName
	return nil
}
