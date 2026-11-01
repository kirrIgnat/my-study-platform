package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

func (s *StudyApp) syncJibriRecordingsForBooking(bookingID string) error {
	if !s.jitsi.RecordingEnabled() {
		return nil
	}
	return s.syncJibriRecordings(strings.TrimSpace(bookingID))
}

func (s *StudyApp) syncJibriRecordingsForStudent(userID string) error {
	if !s.jitsi.RecordingEnabled() {
		return nil
	}
	bookings, err := s.db.Query(`SELECT id FROM bookings WHERE user_id=?`, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(bookings))
	for _, row := range bookings {
		if len(row) > 0 {
			allowed[row[0]] = true
		}
	}
	return s.syncJibriRecordingsFiltered(allowed)
}

func (s *StudyApp) syncJibriRecordings(targetBookingID string) error {
	if targetBookingID == "" {
		return s.syncJibriRecordingsFiltered(nil)
	}
	return s.syncJibriRecordingsFiltered(map[string]bool{targetBookingID: true})
}

func (s *StudyApp) syncJibriRecordingsFiltered(allowed map[string]bool) error {
	root := strings.TrimSpace(s.jitsi.RecordingsDir)
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("прочитать каталог Jibri: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionName := entry.Name()
		if sessionName == "" || sessionName == "." || sessionName == ".." {
			continue
		}
		sessionDir, err := safeRecordingPath(root, sessionName)
		if err != nil {
			continue
		}
		metadataPath := filepath.Join(sessionDir, "metadata.json")
		metadataBytes, err := os.ReadFile(metadataPath)
		if err != nil {
			continue
		}
		bookingID, roomName := recordingMetadataIdentity(metadataBytes)
		if bookingID == "" && roomName != "" {
			rows, queryErr := s.db.Query(`SELECT booking_id FROM lesson_conferences WHERE room_name=? LIMIT 1`, roomName)
			if queryErr != nil {
				return queryErr
			}
			if len(rows) > 0 {
				bookingID = rows[0][0]
			}
		}
		if bookingID == "" || (allowed != nil && !allowed[bookingID]) {
			continue
		}
		valid, queryErr := s.db.Query(`SELECT id FROM bookings WHERE id=? LIMIT 1`, bookingID)
		if queryErr != nil {
			return queryErr
		}
		if len(valid) == 0 {
			continue
		}
		mediaEntry, mediaInfo, err := findJibriMediaFile(sessionDir)
		if err != nil || mediaEntry == "" {
			continue
		}
		relative := filepath.ToSlash(filepath.Join(sessionName, mediaEntry))
		id, err := randomToken(12)
		if err != nil {
			return err
		}
		mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(mediaEntry)))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		recording := LessonRecording{
			ID:           id,
			BookingID:    bookingID,
			RelativePath: relative,
			FileName:     mediaEntry,
			MimeType:     mimeType,
			SizeBytes:    mediaInfo.Size(),
			CreatedAt:    mediaInfo.ModTime(),
		}
		if err := upsertLessonRecording(s.db, recording); err != nil {
			return err
		}
	}
	return nil
}

func findJibriMediaFile(dir string) (string, fs.FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	allowed := map[string]bool{".mp4": true, ".webm": true, ".mkv": true, ".mov": true}
	var bestName string
	var bestInfo fs.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !allowed[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() <= 0 {
			continue
		}
		if bestInfo == nil || info.Size() > bestInfo.Size() {
			bestName, bestInfo = entry.Name(), info
		}
	}
	return bestName, bestInfo, nil
}

func recordingMetadataIdentity(data []byte) (bookingID, roomName string) {
	var payload any
	if json.Unmarshal(data, &payload) != nil {
		return "", ""
	}
	bookingID = findJSONScalar(payload, "booking_id")
	meetingURL := findJSONScalar(payload, "meeting_url")
	if meetingURL == "" {
		meetingURL = findJSONScalar(payload, "meetingUrl")
	}
	if meetingURL != "" {
		meetingURL = strings.TrimRight(strings.TrimSpace(meetingURL), "/")
		if idx := strings.LastIndex(meetingURL, "/"); idx >= 0 && idx+1 < len(meetingURL) {
			roomName = meetingURL[idx+1:]
		}
		if q := strings.IndexByte(roomName, '?'); q >= 0 {
			roomName = roomName[:q]
		}
	}
	return strings.TrimSpace(bookingID), strings.TrimSpace(roomName)
}

func findJSONScalar(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed[key]; ok {
			switch v := raw.(type) {
			case string:
				return v
			case json.Number:
				return v.String()
			}
		}
		for _, child := range typed {
			if found := findJSONScalar(child, key); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findJSONScalar(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func safeRecordingPath(root, relative string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("каталог записей не настроен")
	}
	relative = filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if relative == "." || relative == "" || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("некорректный путь записи")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(filepath.Join(rootAbs, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("путь записи выходит за пределы каталога")
	}
	return fullAbs, nil
}

func (s *StudyApp) recordingAbsolutePath(relative string) (string, error) {
	path, err := safeRecordingPath(s.jitsi.RecordingsDir, relative)
	if err != nil {
		return "", err
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(s.jitsi.RecordingsDir)
	resolvedPath, pathErr := filepath.EvalSymlinks(path)
	if rootErr == nil && pathErr == nil {
		rel, relErr := filepath.Rel(resolvedRoot, resolvedPath)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", errors.New("некорректная ссылка на файл записи")
		}
		return resolvedPath, nil
	}
	return path, nil
}

func (s *StudyApp) removeRecordingFiles(relative string) error {
	fullPath, err := s.recordingAbsolutePath(relative)
	if err != nil {
		return err
	}
	sessionDir := filepath.Dir(fullPath)
	rootAbs, err := filepath.Abs(s.jitsi.RecordingsDir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, sessionDir)
	if err != nil || rel == "." || rel == ".." || strings.Contains(rel, string(filepath.Separator)) || strings.HasPrefix(rel, "..") {
		return errors.New("некорректный каталог с записью")
	}
	return os.RemoveAll(sessionDir)
}
