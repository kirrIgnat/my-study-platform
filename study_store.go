package main

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type StudyCourse struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Level       string
	Duration    string
	Status      string
	Enrolled    bool
	Progress    int
	Students    int
}

type StudyBooking struct {
	ID            string
	UserID        string
	UserName      string
	UserEmail     string
	Telegram      string
	SlotStart     time.Time
	Date          string
	Time          string
	Timezone      string
	Topic         string
	MeetingURL    string
	WeekKey       string
	ScheduleID    string
	OccurrenceKey string
	OriginalDate  string
	OriginalTime  string
	Rescheduled   bool
	Status        string
	CreatedAt     time.Time
}

type TutoringSchedule struct {
	ID            string
	UserID        string
	UserName      string
	UserEmail     string
	Weekday       int
	WeekdayName   string
	Time          string
	Timezone      string
	Topic         string
	MeetingURL    string
	EffectiveFrom string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RecurringSlotOption struct {
	Weekday     int    `json:"weekday"`
	WeekdayName string `json:"weekday_name"`
	Time        string `json:"time"`
	Available   bool   `json:"available"`
	Current     bool   `json:"current"`
}

type RescheduleRequest struct {
	ID               string
	UserID           string
	UserName         string
	UserEmail        string
	ScheduleID       string
	BookingID        string
	Kind             string
	RequestedWeekday int
	RequestedDayName string
	RequestedTime    string
	RequestedDate    string
	RequestedSlot    time.Time
	MonthKey         string
	Reason           string
	Status           string
	TeacherNote      string
	OriginalDate     string
	OriginalTime     string
	CreatedAt        time.Time
	ResolvedAt       time.Time
}

type StudyLesson struct {
	BookingID   string
	UserID      string
	UserName    string
	UserEmail   string
	Date        string
	Time        string
	Timezone    string
	Topic       string
	MeetingURL  string
	Status      string
	Title       string
	Theory      string
	Homework    string
	VideoURL    string
	Published   bool
	PublishedAt time.Time
	UpdatedAt   time.Time
}

type StudyNotification struct {
	ID                  string
	Type                string
	Title               string
	Body                string
	BookingID           string
	RescheduleRequestID string
	Read                bool
	CreatedAt           time.Time
}

type AdminStats struct {
	Students         int
	Bookings         int
	Upcoming         int
	Unread           int
	PublishedLessons int
	PendingChanges   int
	CourseMembers    int
}

var (
	errStudySlotBusy             = errors.New("study slot already booked")
	errStudyRecurringSlotBusy    = errors.New("recurring slot already assigned")
	errTemporaryMoveLimitReached = errors.New("temporary move monthly limit reached")
	errRescheduleAlreadyPending  = errors.New("reschedule request already pending")
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

func getAdminLesson(db *AppDB, bookingID string, loc *time.Location) (StudyLesson, error) {
	rows, err := db.Query(`SELECT b.id,b.user_id,b.date,b.time,b.timezone,b.topic,b.meeting_url,b.status,
		u.name,u.email,COALESCE(l.title,''),COALESCE(l.theory,''),COALESCE(l.homework,''),COALESCE(l.video_url,''),
		COALESCE(l.published,0),COALESCE(l.published_at,0),COALESCE(l.updated_at,0)
		FROM bookings b JOIN users u ON u.id=b.user_id
		LEFT JOIN lesson_materials l ON l.booking_id=b.id WHERE b.id=? LIMIT 1`, bookingID)
	if err != nil {
		return StudyLesson{}, err
	}
	if len(rows) == 0 {
		return StudyLesson{}, errors.New("занятие не найдено")
	}
	row := rows[0]
	lesson := StudyLesson{BookingID: row[0], UserID: row[1], Date: row[2], Time: row[3], Timezone: row[4], Topic: row[5], MeetingURL: row[6], Status: row[7], UserName: row[8], UserEmail: row[9], Title: row[10], Theory: row[11], Homework: row[12], VideoURL: row[13], Published: row[14] == "1"}
	if ts := parseInt64(row[15]); ts > 0 {
		lesson.PublishedAt = time.Unix(ts, 0).In(loc)
	}
	if ts := parseInt64(row[16]); ts > 0 {
		lesson.UpdatedAt = time.Unix(ts, 0).In(loc)
	}
	return lesson, nil
}

func saveLessonMaterial(db *AppDB, bookingID, title, theory, homework, videoURL string, publish bool) error {
	title, theory, homework, videoURL = strings.TrimSpace(title), strings.TrimSpace(theory), strings.TrimSpace(homework), strings.TrimSpace(videoURL)
	if len([]rune(title)) > 180 || len([]rune(theory)) > 30000 || len([]rune(homework)) > 20000 {
		return errors.New("слишком большой объём материалов")
	}
	if len(videoURL) > 2048 || (videoURL != "" && !validHTTPURL(videoURL)) {
		return errors.New("некорректная ссылка на видео")
	}
	if publish && title == "" {
		title = "Материалы занятия"
	}
	now := time.Now().UTC().Unix()
	publishedAt := int64(0)
	if publish {
		publishedAt = now
	}
	return db.WithTx(func(tx *dbTx) error {
		bookingRows, err := tx.query(`SELECT user_id,date,time FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(bookingRows) == 0 {
			return errors.New("занятие не найдено")
		}
		oldRows, err := tx.query(`SELECT title,theory,homework,video_url,published FROM lesson_materials WHERE booking_id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		changed := len(oldRows) == 0
		wasPublished := false
		if len(oldRows) > 0 {
			wasPublished = oldRows[0][4] == "1"
			changed = oldRows[0][0] != title || oldRows[0][1] != theory || oldRows[0][2] != homework || oldRows[0][3] != videoURL
		}
		publishedValue := 0
		if publish {
			publishedValue = 1
		}
		if err := tx.exec(`INSERT INTO lesson_materials(booking_id,title,theory,homework,video_url,published,published_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?)
			ON CONFLICT(booking_id) DO UPDATE SET title=excluded.title,theory=excluded.theory,homework=excluded.homework,
			video_url=excluded.video_url,published=excluded.published,published_at=CASE WHEN excluded.published=1 AND lesson_materials.published=0 THEN excluded.published_at ELSE lesson_materials.published_at END,updated_at=excluded.updated_at`,
			bookingID, title, theory, homework, videoURL, publishedValue, publishedAt, now); err != nil {
			return err
		}
		if publish {
			if err := tx.exec(`UPDATE bookings SET status='done',updated_at=? WHERE id=?`, now, bookingID); err != nil {
				return err
			}
			if !wasPublished || changed {
				notificationTitle := "Материалы урока опубликованы"
				if wasPublished {
					notificationTitle = "Материалы урока обновлены"
				}
				body := fmt.Sprintf("Урок %s %s: %s.", bookingRows[0][1], bookingRows[0][2], title)
				if homework != "" {
					body += " Добавлено домашнее задание."
				}
				return createNotificationTx(tx, bookingRows[0][0], "lesson_materials", notificationTitle, body, bookingID, "", now)
			}
		}
		return nil
	})
}

func listPublishedLessonsForStudent(db *AppDB, userID string, loc *time.Location) ([]StudyLesson, error) {
	rows, err := db.Query(`SELECT b.id,b.user_id,b.date,b.time,b.timezone,b.topic,b.meeting_url,b.status,
		u.name,u.email,l.title,l.theory,l.homework,l.video_url,l.published,l.published_at,l.updated_at
		FROM lesson_materials l JOIN bookings b ON b.id=l.booking_id JOIN users u ON u.id=b.user_id
		WHERE b.user_id=? AND l.published=1 ORDER BY b.slot_start DESC LIMIT 80`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]StudyLesson, 0, len(rows))
	for _, row := range rows {
		lesson := StudyLesson{BookingID: row[0], UserID: row[1], Date: row[2], Time: row[3], Timezone: row[4], Topic: row[5], MeetingURL: row[6], Status: row[7], UserName: row[8], UserEmail: row[9], Title: row[10], Theory: row[11], Homework: row[12], VideoURL: row[13], Published: row[14] == "1"}
		if ts := parseInt64(row[15]); ts > 0 {
			lesson.PublishedAt = time.Unix(ts, 0).In(loc)
		}
		if ts := parseInt64(row[16]); ts > 0 {
			lesson.UpdatedAt = time.Unix(ts, 0).In(loc)
		}
		result = append(result, lesson)
	}
	return result, nil
}

func getStudentByID(db *AppDB, userID string) (*StudyUser, error) {
	rows, err := db.Query(`SELECT id,email,name,telegram,role,created_at FROM users WHERE id=? AND role='student' LIMIT 1`, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	user := &StudyUser{ID: r[0], Email: r[1], Name: r[2], Telegram: r[3], Role: r[4], Created: time.Unix(parseInt64(r[5]), 0).UTC()}
	return user, nil
}

func listLessonsForStudentAdmin(db *AppDB, userID string, loc *time.Location) ([]StudyLesson, error) {
	rows, err := db.Query(`SELECT b.id,b.user_id,b.date,b.time,b.timezone,b.topic,b.meeting_url,b.status,
		u.name,u.email,COALESCE(l.title,''),COALESCE(l.theory,''),COALESCE(l.homework,''),COALESCE(l.video_url,''),
		COALESCE(l.published,0),COALESCE(l.published_at,0),COALESCE(l.updated_at,0)
		FROM bookings b JOIN users u ON u.id=b.user_id
		LEFT JOIN lesson_materials l ON l.booking_id=b.id
		WHERE b.user_id=? ORDER BY b.slot_start DESC`, userID)
	if err != nil {
		return nil, err
	}
	result := make([]StudyLesson, 0, len(rows))
	for _, row := range rows {
		lesson := StudyLesson{BookingID: row[0], UserID: row[1], Date: row[2], Time: row[3], Timezone: row[4], Topic: row[5], MeetingURL: row[6], Status: row[7], UserName: row[8], UserEmail: row[9], Title: row[10], Theory: row[11], Homework: row[12], VideoURL: row[13], Published: row[14] == "1"}
		if ts := parseInt64(row[15]); ts > 0 {
			lesson.PublishedAt = time.Unix(ts, 0).In(loc)
		}
		if ts := parseInt64(row[16]); ts > 0 {
			lesson.UpdatedAt = time.Unix(ts, 0).In(loc)
		}
		result = append(result, lesson)
	}
	return result, nil
}
