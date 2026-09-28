(() => {
  'use strict';

  const endpoint = '/api/study/admin/slots';
  const csrf = document.querySelector('meta[name="csrf-token"]')?.content || '';
  const daysRoot = document.getElementById('teacherSlotsDays');
  const saveButton = document.getElementById('teacherSlotsSave');
  const message = document.getElementById('teacherSlotsMessage');
  const timezoneNode = document.getElementById('teacherSlotsTimezone');

  const dayNames = {
    1: 'Понедельник',
    2: 'Вторник',
    3: 'Среда',
    4: 'Четверг',
    5: 'Пятница',
    6: 'Суббота',
    7: 'Воскресенье'
  };

  let payload = null;
  let selected = new Set();
  let occupied = new Map();

  const keyOf = (weekday, time) => `${weekday}/${time}`;

  function setMessage(text, kind = '') {
    message.textContent = text;
    message.className = `teacher-slots-message ${kind}`.trim();
  }

  function render() {
    if (!payload) return;

    timezoneNode.textContent = payload.timezone || '—';
    daysRoot.replaceChildren();

    for (let weekday = 1; weekday <= 7; weekday += 1) {
      const card = document.createElement('section');
      card.className = 'teacher-slot-day';

      const head = document.createElement('div');
      head.className = 'teacher-slot-day-head';

      const title = document.createElement('h2');
      title.textContent = dayNames[weekday];
      head.appendChild(title);

      const count = document.createElement('span');
      const daySelected = [...selected].filter((key) => key.startsWith(`${weekday}/`)).length;
      count.textContent = `${daySelected} сл.`;
      head.appendChild(count);
      card.appendChild(head);

      const grid = document.createElement('div');
      grid.className = 'teacher-slot-times';

      for (const time of payload.times || []) {
        const key = keyOf(weekday, time);
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'teacher-slot-button';
        const [hour, minute] = time.split(':').map(Number);
        const endTotal = hour * 60 + minute + 90;
        const end = `${String(Math.floor(endTotal / 60) % 24).padStart(2, '0')}:${String(endTotal % 60).padStart(2, '0')}`;
        button.textContent = `${time}–${end}`;
        button.dataset.slotKey = key;

        if (selected.has(key)) button.classList.add('selected');

        if (occupied.has(key)) {
          button.classList.add('occupied');
          button.disabled = true;
          const owner = occupied.get(key);
          button.title = owner ? `Занято: ${owner}` : 'Слот уже закреплён за учеником';
        } else {
          button.addEventListener('click', () => {
            if (selected.has(key)) {
              selected.delete(key);
            } else {
              selected.add(key);
            }
            render();
          });
        }

        grid.appendChild(button);
      }

      card.appendChild(grid);
      daysRoot.appendChild(card);
    }
  }

  function applyPayload(nextPayload) {
    payload = nextPayload;
    selected = new Set();
    occupied = new Map();

    for (const slot of payload.slots || []) {
      const key = keyOf(slot.weekday, slot.time);
      if (slot.active || slot.occupied) selected.add(key);
      if (slot.occupied) occupied.set(key, slot.occupied_by || 'ученик');
    }

    render();
  }

  async function loadSlots() {
    setMessage('');
    try {
      const response = await fetch(endpoint, {
        method: 'GET',
        credentials: 'same-origin',
        headers: { Accept: 'application/json' }
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      applyPayload(data);
      if (!data.configured) {
        setMessage('Слоты ещё не сохранены в базе. До первого сохранения ученики продолжают видеть старое расписание из BOOKING_SCHEDULE.');
      }
    } catch (error) {
      console.error(error);
      daysRoot.innerHTML = '<div class="teacher-slots-loading error">Не удалось загрузить слоты.</div>';
      setMessage('Не удалось загрузить расписание. Обновите страницу.', 'error');
    }
  }

  async function saveSlots() {
    if (!payload) return;

    saveButton.disabled = true;
    setMessage('Сохраняю…');

    const slots = [...selected].map((key) => {
      const slash = key.indexOf('/');
      return {
        weekday: Number(key.slice(0, slash)),
        time: key.slice(slash + 1)
      };
    });

    const form = new URLSearchParams();
    form.set('csrf_token', csrf);
    form.set('slots', JSON.stringify(slots));

    try {
      const response = await fetch(endpoint, {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8',
          Accept: 'application/json'
        },
        body: form.toString()
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `HTTP ${response.status}`);
      }

      applyPayload(await response.json());
      setMessage('Слоты сохранены.', 'success');
    } catch (error) {
      console.error(error);
      setMessage('Не удалось сохранить слоты.', 'error');
    } finally {
      saveButton.disabled = false;
    }
  }

  saveButton?.addEventListener('click', saveSlots);
  loadSlots();
})();
