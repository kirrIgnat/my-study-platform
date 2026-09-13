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
)

var defaultLessonSchedule = map[time.Weekday][]string{
	time.Tuesday:  {"18:00", "19:30", "21:00"},
	time.Thursday: {"18:00", "19:30", "21:00"},
	time.Saturday: {"12:00", "13:30", "15:00"},
}

type StudySlot struct {
	Time      string `json:"time"`
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

func recurringOccupiedMap(db *AppDB, loc *time.Location) (map[string]string, error) {
	rows, err := db.Query(`SELECT user_id,weekday,time FROM tutoring_schedules WHERE status='active' AND timezone=?`, loc.String())
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row[1]+"/"+row[2]] = row[0]
	}
	return result, nil
}

func activeBookedSlots(db *AppDB, excludeBookingID string) (map[int64]struct{}, error) {
	rows, err := db.Query(`SELECT slot_start FROM bookings WHERE status IN ('pending','confirmed') AND slot_start >= ? AND id<>?`, time.Now().UTC().Unix(), excludeBookingID)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		result[parseInt64(row[0])] = struct{}{}
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
	booked, err := activeBookedSlots(db, "")
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	recurring, err := recurringOccupiedMap(db, loc)
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
			_, taken := booked[slotTime.UTC().Unix()]
			_, permanentlyTaken := recurring[strconv.Itoa(weekdayNumber(date.Weekday()))+"/"+clock]
			day.Slots = append(day.Slots, StudySlot{Time: clock, Available: !permanentlyTaken && !taken && slotTime.After(now.Add(2*time.Hour))})
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
	booked, err := activeBookedSlots(db, bookingID)
	if err != nil {
		return StudyAvailabilityResponse{}, err
	}
	recurring, err := recurringOccupiedMap(db, loc)
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
			_, busy := booked[slot.UTC().Unix()]
			owner := recurring[strconv.Itoa(weekdayNumber(date.Weekday()))+"/"+clock]
			permanentlyTaken := owner != ""
			isSame := slot.UTC().Unix() == booking.SlotStart.UTC().Unix()
			available := remaining > 0 && !busy && !permanentlyTaken && !isSame && slot.After(now.Add(2*time.Hour))
			day.Slots = append(day.Slots, StudySlot{Time: clock, Available: available})
		}
		response.Days = append(response.Days, day)
	}
	return response, nil
}
