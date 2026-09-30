package app

import (
	"strings"
	"testing"
	"time"
)

func TestPaymentStatusLabel(t *testing.T) {
	cases := map[string]string{
		"":         "Не оплачено",
		"unpaid":   "Не оплачено",
		"pending":  "На подтверждении",
		"approved": "Оплачено",
		"rejected": "Не подтверждено",
	}
	for status, want := range cases {
		if got := paymentStatusLabel(status); got != want {
			t.Fatalf("paymentStatusLabel(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestQRDataURLFromBytes(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 0, 0, 0, 0}
	got, err := qrDataURLFromBytes(png)
	if err != nil {
		t.Fatalf("png rejected: %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("unexpected data URL: %q", got)
	}
	if _, err := qrDataURLFromBytes([]byte("<script>alert(1)</script>")); err == nil {
		t.Fatal("text upload must be rejected")
	}
}

func TestSortStudentPaymentBookings(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	bookings := []StudyBooking{
		{ID: "done-new", SlotStart: now.Add(-2 * time.Hour), Status: "done"},
		{ID: "future2", SlotStart: now.Add(48 * time.Hour), Status: "confirmed"},
		{ID: "cancelled", SlotStart: now.Add(time.Hour), Status: "cancelled"},
		{ID: "done-old", SlotStart: now.Add(-48 * time.Hour), Status: "done"},
		{ID: "excluded", SlotStart: now.Add(-time.Hour), Status: "done", PaymentExcluded: true},
	}
	got := sortStudentPaymentBookings(bookings, now)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	want := []string{"done-new", "done-old"}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("position %d = %q, want %q", i, got[i].ID, want[i])
		}
	}
}
