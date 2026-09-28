package app

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func getAdminLesson(db *AppDB, bookingID string, loc *time.Location) (StudyLesson, error) {
	rows, err := db.Query(`SELECT b.id,COALESCE(b.schedule_id,''),b.user_id,b.date,b.time,b.timezone,b.topic,b.meeting_url,b.status,
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
	lesson := StudyLesson{BookingID: row[0], ScheduleID: row[1], UserID: row[2], Date: row[3], Time: row[4], EndTime: lessonEndTime(row[4]), DurationMinutes: lessonDurationMinutes, Timezone: row[5], Topic: row[6], MeetingURL: row[7], Status: row[8], UserName: row[9], UserEmail: row[10], Title: row[11], Theory: row[12], Homework: row[13], VideoURL: row[14], Published: row[15] == "1"}
	if ts := parseInt64(row[16]); ts > 0 {
		lesson.PublishedAt = time.Unix(ts, 0).In(loc)
	}
	if ts := parseInt64(row[17]); ts > 0 {
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
		lesson := StudyLesson{BookingID: row[0], UserID: row[1], Date: row[2], Time: row[3], EndTime: lessonEndTime(row[3]), DurationMinutes: lessonDurationMinutes, Timezone: row[4], Topic: row[5], MeetingURL: row[6], Status: row[7], UserName: row[8], UserEmail: row[9], Title: row[10], Theory: row[11], Homework: row[12], VideoURL: row[13], Published: row[14] == "1"}
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
		lesson := StudyLesson{BookingID: row[0], UserID: row[1], Date: row[2], Time: row[3], EndTime: lessonEndTime(row[3]), DurationMinutes: lessonDurationMinutes, Timezone: row[4], Topic: row[5], MeetingURL: row[6], Status: row[7], UserName: row[8], UserEmail: row[9], Title: row[10], Theory: row[11], Homework: row[12], VideoURL: row[13], Published: row[14] == "1"}
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
