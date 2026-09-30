package app

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (s *StudyApp) studentPaymentsData(w http.ResponseWriter, r *http.Request, session *UserSession) StudyPageData {
	_ = ensureRecurringBookings(s.db, s.loc, 16)
	data := s.baseData(w, r, "Оплата занятий — MY.STUDY", "account")
	data.CabinetTab = "payments"
	bookings, err := listUserBookings(s.db, session.User.ID, s.loc)
	if err != nil {
		log.Printf("payment bookings: %v", err)
	}
	data.PaymentBookings = sortStudentPaymentBookings(bookings, time.Now().In(s.loc))
	return data
}

func (s *StudyApp) studentPaymentsPage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role == "admin" {
		http.Redirect(w, r, "/study/admin/payments", http.StatusSeeOther)
		return
	}
	data := s.studentPaymentsData(w, r, session)
	switch r.URL.Query().Get("status") {
	case "pending":
		data.Success = "Оплата отправлена преподавателю на подтверждение. После одобрения занятие станет зелёным."
	}
	renderStudy(w, "study_payment.html", data)
}

func (s *StudyApp) studentPaymentSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.session(r)
	if session == nil {
		http.Redirect(w, r, "/study/login", http.StatusSeeOther)
		return
	}
	if session.User.Role != "student" || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := submitLessonPayment(s.db, session.User, r.FormValue("booking_id")); err != nil {
		data := s.studentPaymentsData(w, r, session)
		data.Error = err.Error()
		renderStudy(w, "study_payment.html", data)
		return
	}
	http.Redirect(w, r, "/study/payment?status=pending", http.StatusSeeOther)
}

func (s *StudyApp) adminPaymentsData(w http.ResponseWriter, r *http.Request) StudyPageData {
	_ = ensureRecurringBookings(s.db, s.loc, 16)
	data := s.baseData(w, r, "Оплата занятий — MY.ADMIN", "admin")
	data.CabinetTab = "payments"
	bookings, err := listAdminBookings(s.db, s.loc)
	if err != nil {
		log.Printf("admin payment bookings: %v", err)
	} else {
		for _, booking := range bookings {
			if booking.Status == "done" && !booking.PaymentExcluded {
				data.PaymentBookings = append(data.PaymentBookings, booking)
			}
		}
	}
	payments, err := listLessonPaymentsForAdmin(s.db, s.loc)
	if err != nil {
		log.Printf("admin payments: %v", err)
	}
	data.LessonPayments = payments
	for _, payment := range payments {
		if payment.Status == "pending" {
			data.PendingPaymentCount++
		}
	}
	return data
}

func (s *StudyApp) adminPaymentsPage(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	if s.requireAdmin(w, r) == nil {
		return
	}
	data := s.adminPaymentsData(w, r)
	switch r.URL.Query().Get("status") {
	case "qr-saved":
		data.Success = "QR-код сохранён для выбранного занятия и уже доступен этому ученику."
	case "qr-cleared":
		data.Success = "QR-код выбранного занятия удалён."
	case "resolved":
		data.Success = "Оплата обработана. Ученик получил уведомление."
	case "payment-deleted":
		data.Success = "Оплата удалена из занятия. Занятие снова отмечено как неоплаченное; QR-код сохранён."
	case "payment-excluded":
		data.Success = "Занятие убрано из раздела оплаты. Само занятие и его материалы сохранены."
	}
	renderStudy(w, "study_admin_payments.html", data)
}

func (s *StudyApp) adminPaymentQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPaymentQRBytes+(256<<10))
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	bookingID := strings.TrimSpace(r.FormValue("booking_id"))
	if bookingID == "" {
		data := s.adminPaymentsData(w, r)
		data.Error = "Занятие для QR-кода не выбрано."
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	if strings.TrimSpace(r.FormValue("action")) == "clear" {
		if err := clearLessonPaymentQRCode(s.db, bookingID); err != nil {
			data := s.adminPaymentsData(w, r)
			data.Error = err.Error()
			renderStudy(w, "study_admin_payments.html", data)
			return
		}
		http.Redirect(w, r, "/study/admin/payments?status=qr-cleared", http.StatusSeeOther)
		return
	}
	file, _, err := r.FormFile("qr_code")
	if err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = "Выберите PNG, JPEG или WebP с QR-кодом."
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxPaymentQRBytes+1))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dataURL, err := qrDataURLFromBytes(payload)
	if err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = err.Error()
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	if err := saveLessonPaymentQRCode(s.db, session.User.ID, bookingID, dataURL); err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = err.Error()
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/payments?status=qr-saved", http.StatusSeeOther)
}

func (s *StudyApp) adminPaymentResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := resolveLessonPayment(s.db, session.User.ID, r.FormValue("payment_id"), r.FormValue("decision"), r.FormValue("teacher_note")); err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = err.Error()
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/payments?status=resolved", http.StatusSeeOther)
}

func (s *StudyApp) adminPaymentDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := deleteLessonPayment(s.db, r.FormValue("booking_id")); err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = err.Error()
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/payments?status=payment-deleted", http.StatusSeeOther)
}

func (s *StudyApp) adminPaymentExclude(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	session := s.requireAdmin(w, r)
	if session == nil || !s.auth.VerifyCSRF(r, session) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := excludeLessonFromPayments(s.db, session.User.ID, r.FormValue("booking_id")); err != nil {
		data := s.adminPaymentsData(w, r)
		data.Error = err.Error()
		renderStudy(w, "study_admin_payments.html", data)
		return
	}
	http.Redirect(w, r, "/study/admin/payments?status=payment-excluded", http.StatusSeeOther)
}
