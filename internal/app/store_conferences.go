package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func getLessonConference(db *AppDB, bookingID string, loc *time.Location) (LessonConference, error) {
	rows, err := db.Query(`SELECT booking_id,room_name,COALESCE(created_by,''),created_at,updated_at FROM lesson_conferences WHERE booking_id=? LIMIT 1`, strings.TrimSpace(bookingID))
	if err != nil {
		return LessonConference{}, err
	}
	if len(rows) == 0 {
		return LessonConference{}, nil
	}
	r := rows[0]
	result := LessonConference{BookingID: r[0], RoomName: r[1], CreatedBy: r[2]}
	if ts := parseInt64(r[3]); ts > 0 {
		result.CreatedAt = time.Unix(ts, 0).In(loc)
	}
	if ts := parseInt64(r[4]); ts > 0 {
		result.UpdatedAt = time.Unix(ts, 0).In(loc)
	}
	return result, nil
}

func createLessonConference(db *AppDB, adminID, bookingID string) (LessonConference, error) {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return LessonConference{}, errors.New("занятие не выбрано")
	}
	var created LessonConference
	err := db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT b.user_id,b.date,b.time,b.status,COALESCE(c.room_name,'')
			FROM bookings b LEFT JOIN lesson_conferences c ON c.booking_id=b.id WHERE b.id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("занятие не найдено")
		}
		if rows[0][3] == "cancelled" {
			return errors.New("для отменённого занятия конференция недоступна")
		}
		if rows[0][4] != "" {
			created = LessonConference{BookingID: bookingID, RoomName: rows[0][4]}
			return nil
		}
		room, err := newConferenceRoomName()
		if err != nil {
			return err
		}
		now := time.Now().UTC().Unix()
		if err := tx.exec(`INSERT INTO lesson_conferences(booking_id,room_name,created_by,created_at,updated_at) VALUES(?,?,?,?,?)`, bookingID, room, adminID, now, now); err != nil {
			return err
		}
		body := fmt.Sprintf("Для занятия %s в %s подготовлена приватная конференция MY.STUDY.", rows[0][1], rows[0][2])
		if err := createNotificationTx(tx, rows[0][0], "conference_ready", "Конференция к занятию готова", body, bookingID, "", now); err != nil {
			return err
		}
		created = LessonConference{BookingID: bookingID, RoomName: room, CreatedBy: adminID, CreatedAt: time.Unix(now, 0).UTC(), UpdatedAt: time.Unix(now, 0).UTC()}
		return nil
	})
	return created, err
}

func deleteLessonConference(db *AppDB, adminID, bookingID string) error {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return errors.New("занятие не выбрано")
	}
	return db.WithTx(func(tx *dbTx) error {
		rows, err := tx.query(`SELECT b.user_id,b.date,b.time,c.room_name FROM bookings b JOIN lesson_conferences c ON c.booking_id=b.id WHERE b.id=? LIMIT 1`, bookingID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("конференция не найдена")
		}
		if err := tx.exec(`DELETE FROM lesson_conferences WHERE booking_id=?`, bookingID); err != nil {
			return err
		}
		now := time.Now().UTC().Unix()
		body := fmt.Sprintf("Конференция для занятия %s в %s была удалена преподавателем.", rows[0][1], rows[0][2])
		return createNotificationTx(tx, rows[0][0], "conference_removed", "Конференция удалена", body, bookingID, "", now)
	})
}

func upsertLessonRecording(db *AppDB, recording LessonRecording) error {
	visible := 0
	if recording.StudentVisible {
		visible = 1
	}
	createdAt := recording.CreatedAt.UTC().Unix()
	if createdAt <= 0 {
		createdAt = time.Now().UTC().Unix()
	}
	return db.Exec(`INSERT INTO lesson_recordings(id,booking_id,relative_path,file_name,mime_type,size_bytes,student_visible,created_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(relative_path) DO UPDATE SET file_name=excluded.file_name,mime_type=excluded.mime_type,size_bytes=excluded.size_bytes`,
		recording.ID, recording.BookingID, recording.RelativePath, recording.FileName, recording.MimeType, recording.SizeBytes, visible, createdAt)
}

func listLessonRecordings(db *AppDB, bookingID string, includeHidden bool, loc *time.Location) ([]LessonRecording, error) {
	query := `SELECT id,booking_id,relative_path,file_name,mime_type,size_bytes,student_visible,created_at FROM lesson_recordings WHERE booking_id=?`
	if !includeHidden {
		query += ` AND student_visible=1`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := db.Query(query, bookingID)
	if err != nil {
		return nil, err
	}
	return recordingsFromRows(rows, loc), nil
}

func listVisibleRecordingsForStudent(db *AppDB, userID string, loc *time.Location) (map[string][]LessonRecording, error) {
	rows, err := db.Query(`SELECT r.id,r.booking_id,r.relative_path,r.file_name,r.mime_type,r.size_bytes,r.student_visible,r.created_at
		FROM lesson_recordings r JOIN bookings b ON b.id=r.booking_id
		WHERE b.user_id=? AND r.student_visible=1 ORDER BY r.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]LessonRecording)
	for _, recording := range recordingsFromRows(rows, loc) {
		result[recording.BookingID] = append(result[recording.BookingID], recording)
	}
	return result, nil
}

func recordingsFromRows(rows [][]string, loc *time.Location) []LessonRecording {
	result := make([]LessonRecording, 0, len(rows))
	for _, r := range rows {
		if len(r) < 8 {
			continue
		}
		size := parseInt64(r[5])
		recording := LessonRecording{ID: r[0], BookingID: r[1], RelativePath: r[2], FileName: r[3], MimeType: r[4], SizeBytes: size, SizeLabel: formatFileSize(size), StudentVisible: r[6] == "1"}
		if ts := parseInt64(r[7]); ts > 0 {
			recording.CreatedAt = time.Unix(ts, 0).In(loc)
		}
		result = append(result, recording)
	}
	return result
}

func getLessonRecording(db *AppDB, id string, loc *time.Location) (LessonRecording, string, error) {
	rows, err := db.Query(`SELECT r.id,r.booking_id,r.relative_path,r.file_name,r.mime_type,r.size_bytes,r.student_visible,r.created_at,b.user_id
		FROM lesson_recordings r JOIN bookings b ON b.id=r.booking_id WHERE r.id=? LIMIT 1`, strings.TrimSpace(id))
	if err != nil {
		return LessonRecording{}, "", err
	}
	if len(rows) == 0 {
		return LessonRecording{}, "", errors.New("запись не найдена")
	}
	r := rows[0]
	recordings := recordingsFromRows([][]string{r[:8]}, loc)
	if len(recordings) == 0 {
		return LessonRecording{}, "", errors.New("запись повреждена")
	}
	return recordings[0], r[8], nil
}

func setLessonRecordingVisibility(db *AppDB, recordingID string, visible bool) error {
	value := 0
	if visible {
		value = 1
	}
	return db.Exec(`UPDATE lesson_recordings SET student_visible=? WHERE id=?`, value, strings.TrimSpace(recordingID))
}

func deleteLessonRecordingDB(db *AppDB, recordingID string) error {
	return db.Exec(`DELETE FROM lesson_recordings WHERE id=?`, strings.TrimSpace(recordingID))
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d Б", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f КБ", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f МБ", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.2f ГБ", float64(size)/(1024*1024*1024))
}
