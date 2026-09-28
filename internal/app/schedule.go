package app

import (
	"errors"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBookingTimezone = "Europe/Moscow"
	defaultBookingDays     = 45
	maxBookingDays         = 62
	lessonDurationMinutes  = 90
	lessonDuration         = 90 * time.Minute
)

var defaultLessonSchedule = map[time.Weekday][]string{
	time.Tuesday:  {"18:00", "19:30", "21:00"},
	time.Thursday: {"18:00", "19:30", "21:00"},
	time.Saturday: {"12:00", "13:30", "15:00"},
}

type StudySlot struct {
	Time      string `json:"time"`
	EndTime   string `json:"end_time"`
	Available bool   `json:"available"`
}

type StudyAvailabilityDay struct {
	Date  string      `json:"date"`
	Slots []StudySlot `json:"slots"`
}

type StudyAvailabilityResponse struct {
	Timezone      string                 `json:"timezone"`
	Days          []StudyAvailabilityDay `json:"days"`
	HasSchedule   bool                   `json:"has_schedule,omitempty"`
	ScheduleCount int                    `json:"schedule_count,omitempty"`
	Remaining     int                    `json:"remaining,omitempty"`
	BookingID     string                 `json:"booking_id,omitempty"`
}

func bookingTimezone() *time.Location {
	name := strings.TrimSpace(os.Getenv("BOOKING_TIMEZONE"))
	if name == "" {
		name = defaultBookingTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("invalid BOOKING_TIMEZONE=%q, falling back to %s: %v", name, defaultBookingTimezone, err)
		loc, _ = time.LoadLocation(defaultBookingTimezone)
	}
	return loc
}

func bookingSchedule() map[time.Weekday][]string {
	raw := strings.TrimSpace(os.Getenv("BOOKING_SCHEDULE"))
	if raw == "" {
		return cloneSchedule(defaultLessonSchedule)
	}
	parsed := make(map[time.Weekday][]string)
	for _, item := range strings.Split(raw, ";") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 {
			continue
		}
		weekdayNumber, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || weekdayNumber < 1 || weekdayNumber > 7 {
			continue
		}
		weekday := time.Weekday(weekdayNumber % 7)
		for _, candidate := range strings.Split(parts[1], ",") {
			candidate = strings.TrimSpace(candidate)
			if _, err := time.Parse("15:04", candidate); err == nil {
				parsed[weekday] = append(parsed[weekday], candidate)
			}
		}
	}
	if len(parsed) == 0 {
		return cloneSchedule(defaultLessonSchedule)
	}
	for weekday := range parsed {
		sort.Strings(parsed[weekday])
	}
	return parsed
}

func cloneSchedule(src map[time.Weekday][]string) map[time.Weekday][]string {
	result := make(map[time.Weekday][]string, len(src))
	for weekday, slots := range src {
		result[weekday] = append([]string(nil), slots...)
	}
	return result
}

func bookingWeekKey(date time.Time) string {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := date.AddDate(0, 0, -(weekday - 1))
	return monday.Format("2006-01-02")
}

func combineDateAndTime(date time.Time, clock string, loc *time.Location) (time.Time, error) {
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc), nil
}

func lessonEndTime(clock string) string {
	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		return ""
	}
	return parsed.Add(lessonDuration).Format("15:04")
}

func lessonIntervalsOverlap(a, b time.Time) bool {
	return a.Before(b.Add(lessonDuration)) && b.Before(a.Add(lessonDuration))
}

func bookedStartsConflict(starts []int64, candidate time.Time) bool {
	for _, unix := range starts {
		if lessonIntervalsOverlap(time.Unix(unix, 0), candidate) {
			return true
		}
	}
	return false
}

type recurringOccupiedSlot struct {
	ScheduleID string
	UserID     string
	Weekday    int
	Time       string
}

func recurringSlotMinute(weekday int, clock string) (int, error) {
	parsed, err := time.Parse("15:04", clock)
	if err != nil || weekday < 1 || weekday > 7 {
		return 0, errors.New("invalid recurring slot")
	}
	return (weekday-1)*24*60 + parsed.Hour()*60 + parsed.Minute(), nil
}

func recurringSlotsOverlap(weekdayA int, clockA string, weekdayB int, clockB string) bool {
	a, errA := recurringSlotMinute(weekdayA, clockA)
	b, errB := recurringSlotMinute(weekdayB, clockB)
	if errA != nil || errB != nil {
		return false
	}
	duration := lessonDurationMinutes
	week := 7 * 24 * 60
	for _, shiftedB := range []int{b - week, b, b + week} {
		if a < shiftedB+duration && shiftedB < a+duration {
			return true
		}
	}
	return false
}

func recurringOccupiedSlots(db *AppDB, loc *time.Location) ([]recurringOccupiedSlot, error) {
	rows, err := db.Query(`SELECT id,user_id,weekday,time FROM tutoring_schedules WHERE status='active' AND timezone=?`, loc.String())
	if err != nil {
		return nil, err
	}
	result := make([]recurringOccupiedSlot, 0, len(rows))
	for _, row := range rows {
		result = append(result, recurringOccupiedSlot{ScheduleID: row[0], UserID: row[1], Weekday: parseInt(row[2]), Time: row[3]})
	}
	return result, nil
}

func recurringConflict(slots []recurringOccupiedSlot, weekday int, clock, excludeScheduleID string) bool {
	for _, slot := range slots {
		if slot.ScheduleID == excludeScheduleID {
			continue
		}
		if recurringSlotsOverlap(slot.Weekday, slot.Time, weekday, clock) {
			return true
		}
	}
	return false
}

func validateStudySlot(dateString, clock string, loc *time.Location, schedule map[time.Weekday][]string) error {
	date, err := time.ParseInLocation("2006-01-02", dateString, loc)
	if err != nil {
		return errors.New("некорректная дата")
	}
	if _, err := time.Parse("15:04", clock); err != nil {
		return errors.New("некорректное время")
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	lastDay := today.AddDate(0, 0, maxBookingDays-1)
	if date.Before(today) || date.After(lastDay) {
		return errors.New("дата вне доступного диапазона")
	}
	allowed := false
	for _, slot := range schedule[date.Weekday()] {
		if slot == clock {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("такого слота нет в расписании")
	}
	slot, err := combineDateAndTime(date, clock, loc)
	if err != nil || !slot.After(now.Add(2*time.Hour)) {
		return errors.New("этот слот уже недоступен")
	}
	return nil
}

func activeBookedStarts(db *AppDB, excludeBookingID string) ([]int64, error) {
	rows, err := db.Query(`SELECT slot_start FROM bookings WHERE status IN ('pending','confirmed') AND slot_start >= ? AND id<>?`, time.Now().UTC().Add(-lessonDuration).Unix(), excludeBookingID)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		result = append(result, parseInt64(row[0]))
	}
	return result, nil
}

func studyAvailability(db *AppDB, loc *time.Location, schedule map[time.Weekday][]string, days int, userID string) (StudyAvailabilityResponse, error) {
	if days < 1 {
		days = defaultBookingDays
	}
	if days > maxBookingDays {
		days = maxBookingDays
	}
	booked, err := activeBookedStarts(db, "")
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	recurring, err := recurringOccupiedSlots(db, loc)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	scheduleCount := 0
	if userID != "" {
		studentSchedules, err := getUserSchedules(db, userID)
		if err != nil {
			return StudyAvailabilityResponse{}, err
		}
		scheduleCount = len(studentSchedules)
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	response := StudyAvailabilityResponse{Timezone: loc.String(), Days: make([]StudyAvailabilityDay, 0, days), HasSchedule: scheduleCount > 0, ScheduleCount: scheduleCount}
	for offset := 0; offset < days; offset++ {
		date := today.AddDate(0, 0, offset)
		scheduled := schedule[date.Weekday()]
		if len(scheduled) == 0 {
			continue
		}
		day := StudyAvailabilityDay{Date: date.Format("2006-01-02"), Slots: make([]StudySlot, 0, len(scheduled))}
		for _, clock := range scheduled {
			slotTime, err := combineDateAndTime(date, clock, loc)
			if err != nil {
				continue
			}
			taken := bookedStartsConflict(booked, slotTime)
			permanentlyTaken := recurringConflict(recurring, weekdayNumber(date.Weekday()), clock, "")
			day.Slots = append(day.Slots, StudySlot{Time: clock, EndTime: lessonEndTime(clock), Available: !permanentlyTaken && !taken && slotTime.After(now.Add(2*time.Hour))})
		}
		response.Days = append(response.Days, day)
	}
	return response, nil
}

func rescheduleAvailability(db *AppDB, loc *time.Location, schedule map[time.Weekday][]string, userID, bookingID string) (StudyAvailabilityResponse, error) {
	booking, err := getBookingForUser(db, bookingID, userID, loc)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	if booking.ScheduleID == "" || !booking.SlotStart.After(time.Now().In(loc)) || (booking.Status != "confirmed" && booking.Status != "pending") {
		return StudyAvailabilityResponse{}, errors.New("это занятие нельзя перенести")
	}
	monthKey := temporaryMoveMonthKey(booking)
	used, err := temporaryMoveUsage(db, userID, monthKey)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	remaining := 2 - used
	if remaining < 0 {
		remaining = 0
	}
	booked, err := activeBookedStarts(db, bookingID)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	recurring, err := recurringOccupiedSlots(db, loc)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	monday, err := time.ParseInLocation("2006-01-02", booking.WeekKey, loc)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	now := time.Now().In(loc)
	response := StudyAvailabilityResponse{Timezone: loc.String(), Remaining: remaining, BookingID: booking.ID, Days: make([]StudyAvailabilityDay, 0, 7)}
	for offset := 0; offset < 7; offset++ {
		date := monday.AddDate(0, 0, offset)
		scheduled := schedule[date.Weekday()]
		if len(scheduled) == 0 {
			continue
		}
		day := StudyAvailabilityDay{Date: date.Format("2006-01-02"), Slots: make([]StudySlot, 0, len(scheduled))}
		for _, clock := range scheduled {
			slot, _ := combineDateAndTime(date, clock, loc)
			busy := bookedStartsConflict(booked, slot)
			permanentlyTaken := recurringConflict(recurring, weekdayNumber(date.Weekday()), clock, booking.ScheduleID)
			isSame := slot.UTC().Unix() == booking.SlotStart.UTC().Unix()
			available := remaining > 0 && !busy && !permanentlyTaken && !isSame && slot.After(now.Add(2*time.Hour))
			day.Slots = append(day.Slots, StudySlot{Time: clock, EndTime: lessonEndTime(clock), Available: available})
		}
		response.Days = append(response.Days, day)
	}
	return response, nil
}
