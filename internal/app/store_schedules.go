package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func getUserSchedules(db *AppDB, userID string) ([]TutoringSchedule, error) {
	rows, err := db.Query(`SELECT id,user_id,weekday,time,timezone,topic,meeting_url,COALESCE(effective_from,''),status,created_at,updated_at
		FROM tutoring_schedules WHERE user_id=? AND status='active' ORDER BY weekday ASC,time ASC`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]TutoringSchedule, 0, len(rows))
	for _, r := range rows {
		weekday := parseInt(r[2])
		result = append(result, TutoringSchedule{ID: r[0], UserID: r[1], Weekday: weekday, WeekdayName: weekdayNameRU(weekday), Time: r[3], Timezone: r[4], Topic: r[5], MeetingURL: r[6], EffectiveFrom: r[7], Status: r[8], CreatedAt: time.Unix(parseInt64(r[9]), 0).UTC(), UpdatedAt: time.Unix(parseInt64(r[10]), 0).UTC()})
	}
	return result, nil
}

func getUserSchedule(db *AppDB, userID string) (*TutoringSchedule, error) {
	schedules, err := getUserSchedules(db, userID)
	if err != nil || len(schedules) == 0 {
		return nil, err
	}
	return &schedules[0], nil
}

func getUserScheduleByID(db *AppDB, userID, scheduleID string) (*TutoringSchedule, error) {
	rows, err := db.Query(`SELECT id,user_id,weekday,time,timezone,topic,meeting_url,COALESCE(effective_from,''),status,created_at,updated_at
		FROM tutoring_schedules WHERE id=? AND user_id=? AND status='active' LIMIT 1`, scheduleID, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	weekday := parseInt(r[2])
	return &TutoringSchedule{ID: r[0], UserID: r[1], Weekday: weekday, WeekdayName: weekdayNameRU(weekday), Time: r[3], Timezone: r[4], Topic: r[5], MeetingURL: r[6], EffectiveFrom: r[7], Status: r[8], CreatedAt: time.Unix(parseInt64(r[9]), 0).UTC(), UpdatedAt: time.Unix(parseInt64(r[10]), 0).UTC()}, nil
}

func listRecurringSlotOptions(db *AppDB, schedule map[time.Weekday][]string, loc *time.Location, userID, currentScheduleID string) ([]RecurringSlotOption, error) {
	rows, err := db.Query(`SELECT id,user_id,weekday,time FROM tutoring_schedules WHERE status='active' AND timezone=?`, loc.String())
	if err != nil {
		return nil, err
	}
	type ownerInfo struct{ scheduleID, userID string }
	occupied := map[string]ownerInfo{}
	for _, row := range rows {
		occupied[fmt.Sprintf("%s/%s", row[2], row[3])] = ownerInfo{scheduleID: row[0], userID: row[1]}
	}
	weekdays := make([]int, 0, len(schedule))
	for wd := range schedule {
		weekdays = append(weekdays, weekdayNumber(wd))
	}
	sort.Ints(weekdays)
	result := make([]RecurringSlotOption, 0)
	for _, day := range weekdays {
		times := append([]string(nil), schedule[weekdayFromNumber(day)]...)
		sort.Strings(times)
		for _, clock := range times {
			owner := occupied[fmt.Sprintf("%d/%s", day, clock)]
			isCurrent := currentScheduleID != "" && owner.scheduleID == currentScheduleID
			available := owner.scheduleID == "" || isCurrent
			result = append(result, RecurringSlotOption{Weekday: day, WeekdayName: weekdayNameRU(day), Time: clock, Available: available, Current: isCurrent})
		}
	}
	return result, nil
}

func listUserBookings(db *AppDB, userID string, loc *time.Location) ([]StudyBooking, error) {
	rows, err := db.Query(`SELECT id,user_id,slot_start,date,time,timezone,topic,meeting_url,week_key,status,created_at,
		COALESCE(schedule_id,''),COALESCE(occurrence_key,''),COALESCE(original_date,''),COALESCE(original_time,''),COALESCE(rescheduled,0)
		FROM bookings WHERE user_id=? ORDER BY slot_start DESC`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]StudyBooking, 0, len(rows))
	for _, row := range rows {
		result = append(result, bookingFromRow(row, loc))
	}
	return result, nil
}

func listAdminBookings(db *AppDB, loc *time.Location) ([]StudyBooking, error) {
	nowUnix := time.Now().UTC().Unix()
	rows, err := db.Query(`SELECT b.id,b.user_id,b.slot_start,b.date,b.time,b.timezone,b.topic,b.meeting_url,b.week_key,b.status,b.created_at,
		COALESCE(b.schedule_id,''),COALESCE(b.occurrence_key,''),COALESCE(b.original_date,''),COALESCE(b.original_time,''),COALESCE(b.rescheduled,0),
		u.name,u.email,u.telegram
		FROM bookings b JOIN users u ON u.id=b.user_id
		ORDER BY CASE WHEN b.slot_start>=? THEN 0 ELSE 1 END,
		CASE WHEN b.slot_start>=? THEN b.slot_start END ASC,
		CASE WHEN b.slot_start<? THEN b.slot_start END DESC LIMIT 500`, nowUnix, nowUnix, nowUnix)
	if err != nil {
		return nil, err
	}
	result := make([]StudyBooking, 0, len(rows))
	for _, row := range rows {
		booking := bookingFromRow(row[:16], loc)
		booking.UserName, booking.UserEmail, booking.Telegram = row[16], row[17], row[18]
		result = append(result, booking)
	}
	return result, nil
}

func bookingFromRow(row []string, loc *time.Location) StudyBooking {
	return StudyBooking{
		ID: row[0], UserID: row[1], SlotStart: time.Unix(parseInt64(row[2]), 0).In(loc), Date: row[3], Time: row[4], Timezone: row[5],
		Topic: row[6], MeetingURL: row[7], WeekKey: row[8], Status: row[9], CreatedAt: time.Unix(parseInt64(row[10]), 0).UTC(),
		ScheduleID: row[11], OccurrenceKey: row[12], OriginalDate: row[13], OriginalTime: row[14], Rescheduled: row[15] == "1",
	}
}

func nextWeekdayOnOrAfter(start time.Time, weekday int) time.Time {
	target := weekdayFromNumber(weekday)
	delta := (int(target) - int(start.Weekday()) + 7) % 7
	return start.AddDate(0, 0, delta)
}

func ensureRecurringBookings(db *AppDB, loc *time.Location, weeks int) error {
	if weeks < 4 {
		weeks = 4
	}
	rows, err := db.Query(`SELECT id,user_id,weekday,time,timezone,topic,meeting_url,COALESCE(effective_from,'') FROM tutoring_schedules WHERE status='active'`)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for _, row := range rows {
		weekday := parseInt(row[2])
		start := today
		if row[7] != "" {
			if effectiveFrom, parseErr := time.ParseInLocation("2006-01-02", row[7], loc); parseErr == nil && effectiveFrom.After(start) {
				start = effectiveFrom
			}
		}
		first := nextWeekdayOnOrAfter(start, weekday)
		for i := 0; i < weeks; i++ {
			date := first.AddDate(0, 0, i*7)
			slot, err := combineDateAndTime(date, row[3], loc)
			if err != nil || !slot.After(now) {
				continue
			}
			id, err := randomToken(12)
			if err != nil {
				return err
			}
			dateString := date.Format("2006-01-02")
			weekKey := bookingWeekKey(date)
			nowUnix := time.Now().UTC().Unix()
			if err := db.Exec(`INSERT INTO bookings(id,user_id,slot_start,date,time,timezone,topic,meeting_url,week_key,status,created_at,updated_at,schedule_id,occurrence_key,original_date,original_time,rescheduled)
				VALUES(?,?,?,?,?,?,?,?,?,'confirmed',?,?,?,?,?,?,0) ON CONFLICT DO NOTHING`, id, row[1], slot.UTC().Unix(), dateString, row[3], row[4], row[5], row[6], weekKey, nowUnix, nowUnix, row[0], weekKey, dateString, row[3]); err != nil {
				return err
			}
		}
	}
	return nil
}

func createNotificationTx(tx dbExecutor, recipientID, notificationType, title, body, bookingID, requestID string, now int64) error {
	id, err := randomToken(12)
	if err != nil {
		return err
	}
	var bookingArg any
	if bookingID != "" {
		bookingArg = bookingID
	}
	var requestArg any
	if requestID != "" {
		requestArg = requestID
	}
	return tx.exec(`INSERT INTO notifications(id,recipient_user_id,type,title,body,booking_id,reschedule_request_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, id, recipientID, notificationType, title, body, bookingArg, requestArg, now)
}

func notifyAdminsTx(tx dbExecutor, notificationType, title, body, bookingID, requestID string, now int64) error {
	admins, err := tx.query(`SELECT id FROM users WHERE role='admin'`)
	if err != nil {
		return err
	}
	for _, admin := range admins {
		if err := createNotificationTx(tx, admin[0], notificationType, title, body, bookingID, requestID, now); err != nil {
			return err
		}
	}
	return nil
}

func createRecurringSchedule(db *AppDB, user StudyUser, date, clock, topic, defaultMeetingURL string, loc *time.Location, allowed map[time.Weekday][]string) (TutoringSchedule, error) {
	parsedDate, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return TutoringSchedule{}, errors.New("некорректная дата")
	}
	if err := validateStudySlot(date, clock, loc, allowed); err != nil {
		return TutoringSchedule{}, err
	}
	weekday := weekdayNumber(parsedDate.Weekday())
	meetingURL := strings.TrimSpace(defaultMeetingURL)
	if meetingURL != "" && !validHTTPURL(meetingURL) {
		meetingURL = ""
	}
	id, err := randomToken(12)
	if err != nil {
		return TutoringSchedule{}, err
	}
	now := time.Now().UTC()
	schedule := TutoringSchedule{ID: id, UserID: user.ID, UserName: user.Name, UserEmail: user.Email, Weekday: weekday, WeekdayName: weekdayNameRU(weekday), Time: clock, Timezone: loc.String(), Topic: strings.TrimSpace(topic), MeetingURL: meetingURL, EffectiveFrom: date, Status: "active", CreatedAt: now, UpdatedAt: now}
	slot, _ := combineDateAndTime(parsedDate, clock, loc)
	bookingID, err := randomToken(12)
	if err != nil {
		return TutoringSchedule{}, err
	}
	weekKey := bookingWeekKey(parsedDate)
	err = db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT id FROM tutoring_schedules WHERE weekday=? AND time=? AND timezone=? AND status='active' LIMIT 1`, weekday, clock, loc.String())
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return errStudyRecurringSlotBusy
		}
		if err := tx.exec(`INSERT INTO tutoring_schedules(id,user_id,weekday,time,timezone,topic,meeting_url,effective_from,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,'active',?,?)`, schedule.ID, schedule.UserID, schedule.Weekday, schedule.Time, schedule.Timezone, schedule.Topic, schedule.MeetingURL, schedule.EffectiveFrom, now.Unix(), now.Unix()); err != nil {
			if isDBConstraint(err) {
				return errStudyRecurringSlotBusy
			}
			return err
		}
		if err := tx.exec(`INSERT INTO bookings(id,user_id,slot_start,date,time,timezone,topic,meeting_url,week_key,status,created_at,updated_at,schedule_id,occurrence_key,original_date,original_time,rescheduled)
			VALUES(?,?,?,?,?,?,?,?,?,'confirmed',?,?,?,?,?,?,0)`, bookingID, user.ID, slot.UTC().Unix(), date, clock, loc.String(), schedule.Topic, meetingURL, weekKey, now.Unix(), now.Unix(), schedule.ID, weekKey, date, clock); err != nil {
			if isDBConstraint(err) {
				return errStudySlotBusy
			}
			return err
		}
		body := fmt.Sprintf("%s закрепил постоянный слот: %s, %s (%s).", user.Name, weekdayNameRU(weekday), clock, loc.String())
		if err := notifyAdminsTx(tx, "schedule_created", "Новый постоянный слот", body, bookingID, "", now.Unix()); err != nil {
			return err
		}
		return createNotificationTx(tx, user.ID, "schedule_created", "Постоянное расписание создано", fmt.Sprintf("За вами закреплено: %s в %s каждую неделю.", weekdayNameRU(weekday), clock), bookingID, "", now.Unix())
	})
	if err != nil {
		return TutoringSchedule{}, err
	}
	if err := ensureRecurringBookings(db, loc, 16); err != nil {
		return schedule, err
	}
	return schedule, nil
}

func recurringSlotAllowed(allowed map[time.Weekday][]string, weekday int, clock string) bool {
	for _, candidate := range allowed[weekdayFromNumber(weekday)] {
		if candidate == clock {
			return true
		}
	}
	return false
}

func requestPermanentReschedule(db *AppDB, user StudyUser, scheduleID string, weekday int, clock, reason string, loc *time.Location, allowed map[time.Weekday][]string) (RescheduleRequest, error) {
	if !recurringSlotAllowed(allowed, weekday, clock) {
		return RescheduleRequest{}, errors.New("такого постоянного слота нет в расписании")
	}
	scheduleID = strings.TrimSpace(scheduleID)
	var schedule *TutoringSchedule
	var err error
	if scheduleID != "" {
		schedule, err = getUserScheduleByID(db, user.ID, scheduleID)
	} else {
		schedules, listErr := getUserSchedules(db, user.ID)
		if listErr != nil {
			err = listErr
		} else if len(schedules) == 1 {
			schedule = &schedules[0]
		} else if len(schedules) > 1 {
			return RescheduleRequest{}, errors.New("выберите, какое постоянное занятие переносите")
		}
	}
	if err != nil || schedule == nil {
		if err != nil {
			return RescheduleRequest{}, err
		}
		return RescheduleRequest{}, errors.New("постоянное занятие не найдено")
	}
	if schedule.Weekday == weekday && schedule.Time == clock {
		return RescheduleRequest{}, errors.New("это уже ваш постоянный слот")
	}
	requestID, err := randomToken(12)
	if err != nil {
		return RescheduleRequest{}, err
	}
	now := time.Now().UTC()
	reason = strings.TrimSpace(reason)
	request := RescheduleRequest{ID: requestID, UserID: user.ID, UserName: user.Name, UserEmail: user.Email, ScheduleID: schedule.ID, Kind: "permanent", RequestedWeekday: weekday, RequestedDayName: weekdayNameRU(weekday), RequestedTime: clock, Reason: reason, Status: "pending", CreatedAt: now}
	err = db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT id FROM reschedule_requests WHERE schedule_id=? AND kind='permanent' AND status='pending' LIMIT 1`, schedule.ID)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return errRescheduleAlreadyPending
		}
		rows, err = tx.query(`SELECT id FROM tutoring_schedules WHERE weekday=? AND time=? AND timezone=? AND status='active' AND id<>? LIMIT 1`, weekday, clock, loc.String(), schedule.ID)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return errStudyRecurringSlotBusy
		}
		if err := tx.exec(`INSERT INTO reschedule_requests(id,user_id,schedule_id,kind,requested_weekday,requested_time,reason,status,created_at) VALUES(?,?,?,?,?,?,?,'pending',?)`, request.ID, user.ID, schedule.ID, "permanent", weekday, clock, reason, now.Unix()); err != nil {
			if isDBConstraint(err) {
				return errRescheduleAlreadyPending
			}
			return err
		}
		body := fmt.Sprintf("%s просит постоянно перенести занятия с %s %s на %s %s.", user.Name, schedule.WeekdayName, schedule.Time, weekdayNameRU(weekday), clock)
		if reason != "" {
			body += " Причина: " + reason
		}
		return notifyAdminsTx(tx, "permanent_reschedule", "Запрос на постоянный перенос", body, "", request.ID, now.Unix())
	})
	return request, err
}

func temporaryMoveMonthKey(booking StudyBooking) string {
	date := booking.OriginalDate
	if date == "" {
		date = booking.Date
	}
	if len(date) >= 7 {
		return date[:7]
	}
	return ""
}

func temporaryMoveUsage(db *AppDB, userID, monthKey string) (int, error) {
	rows, err := db.Query(`SELECT COUNT(*) FROM reschedule_requests WHERE user_id=? AND kind='temporary' AND month_key=? AND status IN ('pending','approved')`, userID, monthKey)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return parseInt(rows[0][0]), nil
}

func getBookingForUser(db *AppDB, bookingID, userID string, loc *time.Location) (StudyBooking, error) {
	rows, err := db.Query(`SELECT id,user_id,slot_start,date,time,timezone,topic,meeting_url,week_key,status,created_at,
		COALESCE(schedule_id,''),COALESCE(occurrence_key,''),COALESCE(original_date,''),COALESCE(original_time,''),COALESCE(rescheduled,0)
		FROM bookings WHERE id=? AND user_id=? LIMIT 1`, bookingID, userID)
	if err != nil {
		return StudyBooking{}, err
	}
	if len(rows) == 0 {
		return StudyBooking{}, errors.New("занятие не найдено")
	}
	return bookingFromRow(rows[0], loc), nil
}

func slotClaimedByRecurringSchedule(db *AppDB, weekday int, clock, timezone, excludeScheduleID string) (bool, error) {
	rows, err := db.Query(`SELECT id FROM tutoring_schedules WHERE weekday=? AND time=? AND timezone=? AND status='active' AND id<>? LIMIT 1`, weekday, clock, timezone, excludeScheduleID)
	return len(rows) > 0, err
}

func activeBookingAtSlot(db *AppDB, unix int64, excludeBookingID string) (bool, error) {
	rows, err := db.Query(`SELECT id FROM bookings WHERE slot_start=? AND status IN ('pending','confirmed') AND id<>? LIMIT 1`, unix, excludeBookingID)
	return len(rows) > 0, err
}
