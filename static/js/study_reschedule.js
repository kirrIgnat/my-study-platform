(() => {
  'use strict';

  const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content || '';
  const permanentDialog = document.getElementById('permanentMoveDialog');
  const temporaryDialog = document.getElementById('temporaryMoveDialog');

  const postJSON = async (url, payload) => {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-CSRF-Token': csrfToken },
      body: JSON.stringify(payload)
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || 'Не удалось выполнить действие');
    return data;
  };

  if (permanentDialog) {
    const picker = document.getElementById('recurringSlotPicker');
    const status = document.getElementById('permanentMoveStatus');
    const submit = document.getElementById('permanentMoveSubmit');
    const reason = document.getElementById('permanentMoveReason');
    let selected = null;
    let scheduleID = '';

    const loadSlots = async () => {
      picker.innerHTML = '<span>Загружаю свободные слоты…</span>';
      status.textContent = '';
      submit.disabled = true;
      selected = null;
      try {
        const response = await fetch(`/api/study/recurring-slots?schedule_id=${encodeURIComponent(scheduleID)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || 'Не удалось загрузить слоты');
        picker.innerHTML = '';
        const groups = new Map();
        data.slots.forEach((slot) => {
          if (!groups.has(slot.weekday)) groups.set(slot.weekday, { name: slot.weekday_name, slots: [] });
          groups.get(slot.weekday).slots.push(slot);
        });
        groups.forEach((group) => {
          const section = document.createElement('section');
          section.className = 'recurring-slot-group';
          const title = document.createElement('b');
          title.textContent = group.name;
          section.appendChild(title);
          const row = document.createElement('div');
          row.className = 'recurring-slot-row';
          group.slots.forEach((slot) => {
            const button = document.createElement('button');
            button.type = 'button';
            button.className = 'slot-button';
            button.textContent = slot.current ? `${slot.time} · текущее` : slot.time;
            button.disabled = !slot.available || slot.current;
            if (slot.current) button.classList.add('current-slot');
            button.addEventListener('click', () => {
              picker.querySelectorAll('.slot-button.selected').forEach((node) => node.classList.remove('selected'));
              button.classList.add('selected');
              selected = slot;
              submit.disabled = false;
              status.textContent = `${slot.weekday_name}, ${slot.time} · после согласования преподавателем`;
            });
            row.appendChild(button);
          });
          section.appendChild(row);
          picker.appendChild(section);
        });
      } catch (error) {
        picker.innerHTML = `<span>${error.message}</span>`;
      }
    };

    document.querySelectorAll('[data-open-permanent-move]').forEach((button) => {
      button.addEventListener('click', () => {
        scheduleID = button.dataset.scheduleId || '';
        const title = document.getElementById('permanentMoveTitle');
        if (title) title.textContent = `Перенести ${button.dataset.scheduleLabel || 'занятие'}`;
        permanentDialog.showModal();
        loadSlots();
      });
    });

    submit.addEventListener('click', async () => {
      if (!selected) return;
      submit.disabled = true;
      submit.textContent = 'Отправляю…';
      status.textContent = 'Создаю запрос преподавателю…';
      try {
        await postJSON('/api/study/reschedule/permanent', { schedule_id: scheduleID, weekday: selected.weekday, time: selected.time, reason: reason.value.trim() });
        window.location.assign('/study/account?tab=transfers&status=move-requested');
      } catch (error) {
        status.textContent = error.message;
        submit.disabled = false;
        submit.textContent = 'Отправить на согласование';
      }
    });
  }

  if (temporaryDialog) {
    const title = document.getElementById('temporaryMoveTitle');
    const limit = document.getElementById('temporaryMoveLimit');
    const calendar = document.getElementById('moveCalendarGrid');
    const slotList = document.getElementById('moveSlotList');
    const reason = document.getElementById('temporaryMoveReason');
    const status = document.getElementById('temporaryMoveStatus');
    const submit = document.getElementById('temporaryMoveSubmit');
    let bookingID = '';
    let selectedDate = '';
    let selectedTime = '';
    let days = new Map();
    let bookingDate = '';

    const dateKey = (date) => {
      const y = date.getFullYear();
      const m = String(date.getMonth() + 1).padStart(2, '0');
      const d = String(date.getDate()).padStart(2, '0');
      return `${y}-${m}-${d}`;
    };
    const parseDate = (value) => {
      const [y, m, d] = value.split('-').map(Number);
      return new Date(y, m - 1, d);
    };
    const human = (value) => new Intl.DateTimeFormat('ru-RU', { weekday: 'short', day: 'numeric', month: 'short' }).format(parseDate(value));

    const renderSlots = () => {
      slotList.innerHTML = '';
      selectedTime = '';
      submit.disabled = true;
      if (!selectedDate) {
        slotList.innerHTML = '<span>Выберите свободную дату.</span>';
        return;
      }
      const available = (days.get(selectedDate)?.slots || []).filter((slot) => slot.available);
      if (!available.length) {
        slotList.innerHTML = '<span>На этот день свободного времени нет.</span>';
        return;
      }
      available.forEach((slot) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'slot-button';
        button.textContent = slot.time;
        button.addEventListener('click', () => {
          slotList.querySelectorAll('.selected').forEach((node) => node.classList.remove('selected'));
          button.classList.add('selected');
          selectedTime = slot.time;
          submit.disabled = false;
          status.textContent = `Запрос: ${human(selectedDate)} в ${selectedTime}`;
        });
        slotList.appendChild(button);
      });
    };

    const renderCalendar = () => {
      calendar.innerHTML = '';
      const current = parseDate(bookingDate);
      const weekday = (current.getDay() + 6) % 7;
      const monday = new Date(current);
      monday.setDate(current.getDate() - weekday);
      for (let i = 0; i < 7; i += 1) {
        const date = new Date(monday);
        date.setDate(monday.getDate() + i);
        const key = dateKey(date);
        const day = days.get(key);
        const canChoose = day?.slots?.some((slot) => slot.available) || false;
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'calendar-day move-calendar-day';
        button.innerHTML = `<small>${new Intl.DateTimeFormat('ru-RU', { weekday: 'short' }).format(date)}</small><b>${date.getDate()}</b>`;
        button.disabled = !canChoose;
        if (key === selectedDate) button.classList.add('selected');
        if (canChoose) button.addEventListener('click', () => {
          selectedDate = key;
          renderCalendar();
          renderSlots();
        });
        calendar.appendChild(button);
      }
    };

    const openTemporary = async (button) => {
      bookingID = button.dataset.bookingId || '';
      bookingDate = button.dataset.bookingDate || '';
      selectedDate = '';
      selectedTime = '';
      days = new Map();
      reason.value = '';
      title.textContent = `Перенести ${bookingDate} · ${button.dataset.bookingTime || ''}`;
      status.textContent = 'Загружаю свободные слоты этой недели…';
      calendar.innerHTML = '';
      slotList.innerHTML = '';
      submit.disabled = true;
      temporaryDialog.showModal();
      try {
        const response = await fetch(`/api/study/reschedule/availability?booking_id=${encodeURIComponent(bookingID)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || 'Не удалось загрузить свободное время');
        days = new Map(data.days.map((day) => [day.date, day]));
        limit.textContent = `Разовых переносов осталось: ${data.remaining} из 2 для месяца этого занятия. Перенос действует только после подтверждения преподавателем.`;
        status.textContent = data.remaining > 0 ? 'Выберите новую дату и время в пределах этой недели.' : 'Лимит разовых переносов на этот месяц исчерпан.';
        renderCalendar();
        renderSlots();
      } catch (error) {
        status.textContent = error.message;
      }
    };

    document.querySelectorAll('[data-open-temporary-move]').forEach((button) => button.addEventListener('click', () => openTemporary(button)));

    submit.addEventListener('click', async () => {
      if (!bookingID || !selectedDate || !selectedTime) return;
      submit.disabled = true;
      submit.textContent = 'Отправляю…';
      status.textContent = 'Отправляю запрос преподавателю…';
      try {
        await postJSON('/api/study/reschedule/temporary', { booking_id: bookingID, date: selectedDate, time: selectedTime, reason: reason.value.trim() });
        window.location.assign('/study/account?tab=transfers&status=move-requested');
      } catch (error) {
        status.textContent = error.message;
        submit.disabled = false;
        submit.textContent = 'Отправить запрос преподавателю';
      }
    });
  }
})();
