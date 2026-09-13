package app

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type TelegramNotifier struct {
	token  string
	chatID string
	client *http.Client
}

func NewTelegramNotifier() *TelegramNotifier {
	return &TelegramNotifier{
		token:  strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		chatID: strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (n *TelegramNotifier) NotifyBooking(booking StudyBooking) error {
	if n == nil || n.token == "" || n.chatID == "" {
		return nil
	}
	contact := booking.UserEmail
	if booking.Telegram != "" {
		contact += " / " + booking.Telegram
	}
	text := fmt.Sprintf("Новая запись в KIRILL.STUDY\n\nСтудент: %s\nДата: %s\nВремя: %s (%s)\nКонтакт: %s", booking.UserName, booking.Date, booking.Time, booking.Timezone, contact)
	if booking.Topic != "" {
		text += "\nТема: " + booking.Topic
	}
	text += "\n\nID: " + booking.ID

	form := url.Values{"chat_id": {n.chatID}, "text": {text}}
	response, err := n.client.PostForm("https://api.telegram.org/bot"+n.token+"/sendMessage", form)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("telegram returned HTTP %s", response.Status)
	}
	return nil
}
