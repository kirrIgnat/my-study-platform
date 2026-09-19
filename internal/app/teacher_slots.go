package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type teacherSlotItem struct {
	Weekday     int    `json:"weekday"`
	WeekdayName string `json:"weekday_name"`
	Time        string `json:"time"`
	Active      bool   `json:"active"`
	Occupied    bool   `json:"occupied"`
	OccupiedBy  string `json:"occupied_by,omitempty"`
}

type teacherSlotInput struct {
	Weekday int    `json:"weekday"`
	Time    string `json:"time"`
}

type teacherSlotsPayload struct {
	Timezone   string            `json:"timezone"`
	Configured bool              `json:"configured"`
	Times      []string          `json:"times"`
	Slots      []teacherSlotItem `json:"slots"`
}

type teacherSlotsHTTP struct {
	study *StudyApp
	db    *AppDB
}

func RegisterTeacherSlotRoutes(mux *http.ServeMux, study *StudyApp, db *AppDB) {
	h := &teacherSlotsHTTP{study: study, db: db}
	mux.HandleFunc("/study/admin/slots", h.page)
	mux.HandleFunc("/api/study/admin/slots", h.api)
}

func ensureTeacherSlotsTables(db *AppDB) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS teacher_slot_settings (
		timezone TEXT PRIMARY KEY,
		configured_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create teacher_slot_settings: %w", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS teacher_slots (
		id TEXT PRIMARY KEY,
		weekday INTEGER NOT NULL CHECK (weekday BETWEEN 1 AND 7),
		time TEXT NOT NULL,
		timezone TEXT NOT NULL,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		UNIQUE (weekday, time, timezone)
	)`); err != nil {
		return fmt.Errorf("create teacher_slots: %w", err)
	}
	return nil
}

func teacherSlotsLocation() (*time.Location, error) {
	name := strings.TrimSpace(os.Getenv("BOOKING_TIMEZONE"))
	if name == "" {
		name = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load booking timezone %q: %w", name, err)
	}
	return loc, nil
}

func teacherSlotKey(weekday int, clock string) string {
	return fmt.Sprintf("%d/%s", weekday, clock)
}

func normalizeTeacherSlotClock(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return "", fmt.Errorf("invalid time %q", value)
	}
	return parsed.Format("15:04"), nil
}

func teacherSlotsScheduleMap(db *AppDB, timezone string) (map[time.Weekday][]string, bool, error) {
	if err := ensureTeacherSlotsTables(db); err != nil {
		return nil, false, err
	}
	settings, err := db.Query(`SELECT timezone FROM teacher_slot_settings WHERE timezone=? LIMIT 1`, timezone)
	if err != nil {
		return nil, false, err
	}
	configured := len(settings) > 0
	result := make(map[time.Weekday][]string)
	if !configured {
		return result, false, nil
	}
	rows, err := db.Query(`SELECT weekday,time FROM teacher_slots WHERE timezone=? ORDER BY weekday ASC,time ASC`, timezone)
	if err != nil {
		return nil, false, err
	}
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		day := parseInt(row[0])
		if day < 1 || day > 7 {
			continue
		}
		clock, err := normalizeTeacherSlotClock(row[1])
		if err != nil {
			continue
		}
		wd := weekdayFromNumber(day)
		result[wd] = append(result[wd], clock)
	}
	for wd := range result {
		sort.Strings(result[wd])
	}
	return result, true, nil
}

func effectiveTeacherSchedule(db *AppDB, loc *time.Location, fallback map[time.Weekday][]string) (map[time.Weekday][]string, error) {
	schedule, configured, err := teacherSlotsScheduleMap(db, loc.String())
	if err != nil {
		return nil, err
	}
	if configured {
		return schedule, nil
	}
	return fallback, nil
}

// currentBookingSchedule returns the teacher-managed availability after the
// teacher has saved it at least once. Before that, BOOKING_SCHEDULE/defaults
// remain a backwards-compatible fallback.
func (s *StudyApp) currentBookingSchedule() (map[time.Weekday][]string, error) {
	return effectiveTeacherSchedule(s.db, s.loc, s.schedule)
}

func listTeacherSlotItems(db *AppDB, timezone string) (teacherSlotsPayload, error) {
	if err := ensureTeacherSlotsTables(db); err != nil {
		return teacherSlotsPayload{}, err
	}

	settings, err := db.Query(`SELECT timezone FROM teacher_slot_settings WHERE timezone=? LIMIT 1`, timezone)
	if err != nil {
		return teacherSlotsPayload{}, err
	}
	configured := len(settings) > 0

	activeRows, err := db.Query(`SELECT weekday,time FROM teacher_slots WHERE timezone=? ORDER BY weekday ASC,time ASC`, timezone)
	if err != nil {
		return teacherSlotsPayload{}, err
	}
	active := make(map[string]bool)
	for _, row := range activeRows {
		if len(row) < 2 {
			continue
		}
		day := parseInt(row[0])
		clock, err := normalizeTeacherSlotClock(row[1])
		if err != nil || day < 1 || day > 7 {
			continue
		}
		active[teacherSlotKey(day, clock)] = true
	}

	occupiedRows, err := db.Query(`SELECT s.weekday,s.time,COALESCE(u.name,''),COALESCE(u.email,'')
		FROM tutoring_schedules s
		LEFT JOIN users u ON u.id=s.user_id
		WHERE s.status='active' AND s.timezone=?
		ORDER BY s.weekday ASC,s.time ASC`, timezone)
	if err != nil {
		return teacherSlotsPayload{}, err
	}
	type occupiedInfo struct{ name, email string }
	occupied := make(map[string]occupiedInfo)
	for _, row := range occupiedRows {
		if len(row) < 4 {
			continue
		}
		day := parseInt(row[0])
		clock, err := normalizeTeacherSlotClock(row[1])
		if err != nil || day < 1 || day > 7 {
			continue
		}
		occupied[teacherSlotKey(day, clock)] = occupiedInfo{name: row[2], email: row[3]}
	}

	keys := make(map[string]teacherSlotItem)
	for key := range active {
		var day int
		var clock string
		if _, err := fmt.Sscanf(key, "%d/%s", &day, &clock); err != nil {
			continue
		}
		keys[key] = teacherSlotItem{Weekday: day, WeekdayName: weekdayNameRU(day), Time: clock, Active: true}
	}
	for key, owner := range occupied {
		item, ok := keys[key]
		if !ok {
			var day int
			var clock string
			if _, err := fmt.Sscanf(key, "%d/%s", &day, &clock); err != nil {
				continue
			}
			item = teacherSlotItem{Weekday: day, WeekdayName: weekdayNameRU(day), Time: clock}
		}
		item.Occupied = true
		item.Active = true
		item.OccupiedBy = strings.TrimSpace(owner.name)
		if item.OccupiedBy == "" {
			item.OccupiedBy = strings.TrimSpace(owner.email)
		}
		keys[key] = item
	}

	items := make([]teacherSlotItem, 0, len(keys))
	for _, item := range keys {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Weekday != items[j].Weekday {
			return items[i].Weekday < items[j].Weekday
		}
		return items[i].Time < items[j].Time
	})

	timesSet := make(map[string]bool)
	for hour := 9; hour <= 22; hour++ {
		for _, minute := range []int{0, 30} {
			if hour == 22 && minute == 30 {
				continue
			}
			timesSet[fmt.Sprintf("%02d:%02d", hour, minute)] = true
		}
	}
	for _, item := range items {
		timesSet[item.Time] = true
	}
	times := make([]string, 0, len(timesSet))
	for clock := range timesSet {
		times = append(times, clock)
	}
	sort.Strings(times)

	return teacherSlotsPayload{Timezone: timezone, Configured: configured, Times: times, Slots: items}, nil
}

func replaceTeacherSlots(db *AppDB, timezone string, slots []teacherSlotInput) error {
	if err := ensureTeacherSlotsTables(db); err != nil {
		return err
	}
	if len(slots) > 400 {
		return fmt.Errorf("too many slots")
	}

	desired := make(map[string]teacherSlotInput)
	for _, slot := range slots {
		if slot.Weekday < 1 || slot.Weekday > 7 {
			return fmt.Errorf("invalid weekday")
		}
		clock, err := normalizeTeacherSlotClock(slot.Time)
		if err != nil {
			return err
		}
		slot.Time = clock
		desired[teacherSlotKey(slot.Weekday, slot.Time)] = slot
	}

	return db.WithTx(func(tx *dbTx) error {
		occupiedRows, err := tx.query(`SELECT weekday,time FROM tutoring_schedules WHERE status='active' AND timezone=?`, timezone)
		if err != nil {
			return err
		}
		// Нельзя удалить слот, который уже закреплён за учеником.
		for _, row := range occupiedRows {
			if len(row) < 2 {
				continue
			}
			day := parseInt(row[0])
			clock, err := normalizeTeacherSlotClock(row[1])
			if err != nil || day < 1 || day > 7 {
				continue
			}
			desired[teacherSlotKey(day, clock)] = teacherSlotInput{Weekday: day, Time: clock}
		}

		if err := tx.exec(`DELETE FROM teacher_slots WHERE timezone=?`, timezone); err != nil {
			return err
		}

		now := time.Now().UTC().Unix()
		ordered := make([]teacherSlotInput, 0, len(desired))
		for _, slot := range desired {
			ordered = append(ordered, slot)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].Weekday != ordered[j].Weekday {
				return ordered[i].Weekday < ordered[j].Weekday
			}
			return ordered[i].Time < ordered[j].Time
		})

		for _, slot := range ordered {
			id, err := randomToken(12)
			if err != nil {
				return err
			}
			if err := tx.exec(`INSERT INTO teacher_slots(id,weekday,time,timezone,created_at,updated_at) VALUES(?,?,?,?,?,?)`,
				id, slot.Weekday, slot.Time, timezone, now, now); err != nil {
				return err
			}
		}

		return tx.exec(`INSERT INTO teacher_slot_settings(timezone,configured_at,updated_at)
			VALUES(?,?,?)
			ON CONFLICT(timezone) DO UPDATE SET updated_at=EXCLUDED.updated_at`, timezone, now, now)
	})
}

func (h *teacherSlotsHTTP) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := h.study.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	data := h.study.baseData(w, r, "Слоты преподавателя — KIRILL.STUDY", "admin")
	data.CabinetTab = "slots"
	renderStudy(w, "study_admin_slots.html", data)
}

func (h *teacherSlotsHTTP) api(w http.ResponseWriter, r *http.Request) {
	session := h.study.session(r)
	if session == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if session.User.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	loc, err := teacherSlotsLocation()
	if err != nil {
		http.Error(w, "invalid booking timezone", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		payload, err := listTeacherSlotItems(h.db, loc.String())
		if err != nil {
			http.Error(w, "failed to load slots", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode(payload)

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		if err := r.ParseForm(); err != nil || !h.study.auth.VerifyCSRF(r, session) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var slots []teacherSlotInput
		if raw := strings.TrimSpace(r.FormValue("slots")); raw != "" {
			if err := json.Unmarshal([]byte(raw), &slots); err != nil {
				http.Error(w, "invalid slots", http.StatusBadRequest)
				return
			}
		}
		if err := replaceTeacherSlots(h.db, loc.String(), slots); err != nil {
			http.Error(w, "failed to save slots", http.StatusBadRequest)
			return
		}
		payload, err := listTeacherSlotItems(h.db, loc.String())
		if err != nil {
			http.Error(w, "failed to reload slots", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(payload)

	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
