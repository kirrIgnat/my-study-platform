const bookingRoot = document.querySelector('.booking-page');

if (bookingRoot) {
  const monthNames = ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'];
  const shortMonthNames = ['янв.', 'февр.', 'марта', 'апр.', 'мая', 'июня', 'июля', 'авг.', 'сент.', 'окт.', 'нояб.', 'дек.'];

  const calendarGrid = document.getElementById('calendarGrid');
  const calendarTitle = document.getElementById('calendarTitle');
  const calendarPrev = document.getElementById('calendarPrev');
  const calendarNext = document.getElementById('calendarNext');
  const calendarHint = document.getElementById('calendarHint');
  const slotsTitle = document.getElementById('slotsTitle');
  const slotList = document.getElementById('slotList');
  const timezoneLabel = document.getElementById('bookingTimezoneLabel');
  const bookingSelection = document.getElementById('bookingSelection');
  const bookingForm = document.getElementById('bookingForm');
  const bookingSubmit = document.getElementById('bookingSubmit');
  const bookingStatus = document.getElementById('bookingStatus');
  const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content || '';

  let availability = new Map();
  let timezone = '';
  let currentMonth = null;
  let minMonth = null;
  let maxMonth = null;
  let selectedDate = '';
  let selectedTime = '';
  let submitting = false;

  function parseISODate(value) {
    const [year, month, day] = value.split('-').map(Number);
    return new Date(year, month - 1, day);
  }

  function dateKey(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  }

  function monthKey(date) {
    return date.getFullYear() * 12 + date.getMonth();
  }

  function humanDate(value) {
    const date = parseISODate(value);
    return `${date.getDate()} ${shortMonthNames[date.getMonth()]} ${date.getFullYear()}`;
  }

  function timezoneText(value) {
    if (value === 'Europe/Moscow') return 'Московское время · МСК';
    return `Часовой пояс · ${value}`;
  }

  function endTimeFor(start) {
    const [hour, minute] = start.split(':').map(Number);
    const total = hour * 60 + minute + 90;
    return `${String(Math.floor(total / 60) % 24).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`;
  }

  function hasAvailableSlots(day) {
    return day && day.slots.some((slot) => slot.available);
  }

  function renderCalendar() {
    if (!currentMonth) return;
    calendarGrid.innerHTML = '';
    calendarTitle.textContent = `${monthNames[currentMonth.getMonth()]} ${currentMonth.getFullYear()}`;

    const first = new Date(currentMonth.getFullYear(), currentMonth.getMonth(), 1);
    const last = new Date(currentMonth.getFullYear(), currentMonth.getMonth() + 1, 0);
    const leading = (first.getDay() + 6) % 7;

    for (let i = 0; i < leading; i += 1) {
      const spacer = document.createElement('span');
      spacer.className = 'calendar-day calendar-day-spacer';
      calendarGrid.appendChild(spacer);
    }

    for (let dayNumber = 1; dayNumber <= last.getDate(); dayNumber += 1) {
      const date = new Date(currentMonth.getFullYear(), currentMonth.getMonth(), dayNumber);
      const key = dateKey(date);
      const day = availability.get(key);
      const available = hasAvailableSlots(day);
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'calendar-day';
      button.textContent = String(dayNumber);
      button.disabled = !available;
      if (key === selectedDate) button.classList.add('selected');
      if (day?.week_booked) {
        button.setAttribute('aria-label', `${humanDate(key)}, на этой неделе занятие уже забронировано`);
      }
      if (available) {
        const count = day.slots.filter((slot) => slot.available).length;
        button.setAttribute('aria-label', `${humanDate(key)}, свободных слотов: ${count}`);
        button.addEventListener('click', () => selectDate(key));
      }
      calendarGrid.appendChild(button);
    }

    calendarPrev.disabled = !minMonth || monthKey(currentMonth) <= monthKey(minMonth);
    calendarNext.disabled = !maxMonth || monthKey(currentMonth) >= monthKey(maxMonth);
  }

  function renderSlots() {
    if (!selectedDate) {
      slotsTitle.textContent = 'Выберите дату';
      slotList.innerHTML = '<div class="booking-empty">После выбора даты здесь появятся свободные слоты.</div>';
      updateSelection();
      return;
    }

    slotsTitle.textContent = humanDate(selectedDate);
    slotList.innerHTML = '';
    const day = availability.get(selectedDate);
    const slots = day ? day.slots.filter((slot) => slot.available) : [];
    if (!slots.length) {
      slotList.innerHTML = '<div class="booking-empty">На эту дату свободных слотов уже нет.</div>';
      selectedTime = '';
      updateSelection();
      return;
    }

    slots.forEach((slot) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'slot-button';
      button.textContent = `${slot.time}–${slot.end_time || endTimeFor(slot.time)}`;
      if (slot.time === selectedTime) button.classList.add('selected');
      button.addEventListener('click', () => {
        selectedTime = slot.time;
        renderSlots();
      });
      slotList.appendChild(button);
    });
    updateSelection();
  }

  function updateSelection() {
    if (selectedDate && selectedTime) {
      const selectedSlot = availability.get(selectedDate)?.slots.find((slot) => slot.time === selectedTime);
      const timeRange = `${selectedTime}–${selectedSlot?.end_time || endTimeFor(selectedTime)}`;
      bookingSelection.textContent = `${humanDate(selectedDate)} · ${timeRange}`;
      if (bookingSubmit) bookingSubmit.disabled = submitting;
      if (bookingStatus && !submitting) bookingStatus.textContent = timezone ? `${timezoneText(timezone)} · слот будет повторяться каждую неделю` : 'Можно закрепить постоянный слот.';
      return;
    }
    bookingSelection.textContent = selectedDate ? `${humanDate(selectedDate)} · выберите время` : 'Дата и время не выбраны';
    if (bookingSubmit) bookingSubmit.disabled = true;
    if (bookingStatus && !submitting) bookingStatus.textContent = 'Сначала выберите дату первого занятия и время.';
  }

  function selectDate(key) {
    selectedDate = key;
    selectedTime = '';
    const date = parseISODate(key);
    currentMonth = new Date(date.getFullYear(), date.getMonth(), 1);
    renderCalendar();
    renderSlots();
  }

  function chooseInitialDate() {
    for (const [key, day] of availability.entries()) {
      if (hasAvailableSlots(day)) {
        selectedDate = key;
        const date = parseISODate(key);
        currentMonth = new Date(date.getFullYear(), date.getMonth(), 1);
        return;
      }
    }
  }

  async function loadAvailability({ keepSelection = false } = {}) {
    calendarHint.textContent = 'Загружаю доступные даты…';
    try {
      const response = await fetch('/api/study/availability?days=45', { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error('Не удалось загрузить календарь');
      const payload = await response.json();
      availability = new Map(payload.days.map((day) => [day.date, day]));
      timezone = payload.timezone;
      timezoneLabel.textContent = timezoneText(timezone);

      const dates = payload.days.map((day) => parseISODate(day.date));
      if (dates.length) {
        minMonth = new Date(dates[0].getFullYear(), dates[0].getMonth(), 1);
        const last = dates[dates.length - 1];
        maxMonth = new Date(last.getFullYear(), last.getMonth(), 1);
      }

      if (!keepSelection || !selectedDate || !hasAvailableSlots(availability.get(selectedDate))) {
        selectedDate = '';
        selectedTime = '';
        chooseInitialDate();
      } else if (selectedTime) {
        const stillFree = availability.get(selectedDate)?.slots.some((slot) => slot.time === selectedTime && slot.available);
        if (!stillFree) selectedTime = '';
      }
      if (!currentMonth && minMonth) currentMonth = new Date(minMonth);
      if (payload.schedule_count > 0) {
        calendarHint.textContent = `У вас уже ${payload.schedule_count} регулярн${payload.schedule_count === 1 ? 'ое занятие' : 'ых занятия'} в неделю. Можно добавить ещё один свободный слот.`;
      } else {
        calendarHint.textContent = 'Подсвечены даты, где есть свободное время. Выбранный день недели и время закрепятся за вами постоянно.';
      }
      renderCalendar();
      renderSlots();
    } catch (error) {
      calendarGrid.innerHTML = '';
      calendarHint.textContent = error.message || 'Календарь временно недоступен.';
      slotList.innerHTML = '<div class="booking-empty">Не удалось загрузить расписание.</div>';
    }
  }

  calendarPrev.addEventListener('click', () => {
    if (!currentMonth || calendarPrev.disabled) return;
    currentMonth = new Date(currentMonth.getFullYear(), currentMonth.getMonth() - 1, 1);
    renderCalendar();
  });
  calendarNext.addEventListener('click', () => {
    if (!currentMonth || calendarNext.disabled) return;
    currentMonth = new Date(currentMonth.getFullYear(), currentMonth.getMonth() + 1, 1);
    renderCalendar();
  });

  if (bookingForm) {
    bookingForm.addEventListener('submit', async (event) => {
      event.preventDefault();
      if (submitting || !selectedDate || !selectedTime) return;
      submitting = true;
      bookingSubmit.disabled = true;
      bookingSubmit.textContent = 'Закрепляю время…';
      bookingStatus.classList.remove('success');
      bookingStatus.textContent = 'Проверяю, что постоянный слот свободен…';

      try {
        const response = await fetch('/api/study/bookings', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-CSRF-Token': csrfToken },
          body: JSON.stringify({ date: selectedDate, time: selectedTime, topic: document.getElementById('bookingTopic').value.trim() })
        });
        const payload = await response.json().catch(() => ({}));
        if (response.status === 401) {
          window.location.assign('/study/login');
          return;
        }
        if (!response.ok) throw new Error(payload.error || 'Не удалось закрепить время');

        const bookedDate = selectedDate;
        const bookedTime = selectedTime;
        document.getElementById('bookingTopic').value = '';
        selectedTime = '';
        await loadAvailability({ keepSelection: true });
        const bookedSlot = availability.get(bookedDate)?.slots.find((slot) => slot.time === bookedTime);
        const bookedRange = `${bookedTime}–${bookedSlot?.end_time || endTimeFor(bookedTime)}`;
        bookingStatus.textContent = `Готово: ${humanDate(bookedDate)} · ${bookedRange}. Новый еженедельный слот добавлен.`;
        bookingStatus.classList.add('success');
      } catch (error) {
        await loadAvailability({ keepSelection: true });
        bookingStatus.textContent = error.message || 'Не удалось закрепить время.';
      } finally {
        submitting = false;
        bookingSubmit.textContent = 'Добавить еженедельное занятие';
        bookingSubmit.disabled = !(selectedDate && selectedTime);
      }
    });
  }

  loadAvailability();
}
