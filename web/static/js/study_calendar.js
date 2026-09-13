(() => {
  'use strict';

  const roots = document.querySelectorAll('[data-study-calendar]');
  if (!roots.length) return;

  const monthNames = ['Январь','Февраль','Март','Апрель','Май','Июнь','Июль','Август','Сентябрь','Октябрь','Ноябрь','Декабрь'];
  const parseDate = (value) => {
    const [y, m, d] = value.split('-').map(Number);
    return new Date(y, m - 1, d);
  };
  const dateKey = (date) => `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
  const escapeHTML = (value = '') => String(value).replace(/[&<>'"]/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[ch]));

  roots.forEach(async (root) => {
    const endpoint = root.dataset.calendarEndpoint;
    const role = root.dataset.calendarRole || 'student';
    const grid = root.querySelector('[data-calendar-grid]');
    const title = root.querySelector('[data-calendar-title]');
    const dayPanel = root.querySelector('[data-calendar-day]');
    const prev = root.querySelector('[data-calendar-prev]');
    const next = root.querySelector('[data-calendar-next]');
    let events = [];
    let currentMonth = new Date();
    currentMonth = new Date(currentMonth.getFullYear(), currentMonth.getMonth(), 1);
    let selected = dateKey(new Date());

    const byDate = () => {
      const map = new Map();
      events.forEach((event) => {
        if (!map.has(event.date)) map.set(event.date, []);
        map.get(event.date).push(event);
      });
      map.forEach((list) => list.sort((a,b) => a.time.localeCompare(b.time)));
      return map;
    };

    const renderDay = () => {
      const map = byDate();
      const list = map.get(selected) || [];
      const date = parseDate(selected);
      const human = new Intl.DateTimeFormat('ru-RU', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }).format(date);
      if (!list.length) {
        dayPanel.innerHTML = `<div class="calendar-day-panel-head"><b>${escapeHTML(human)}</b><span>Занятий нет</span></div>`;
        return;
      }
      dayPanel.innerHTML = `<div class="calendar-day-panel-head"><b>${escapeHTML(human)}</b><span>${list.length} занят${list.length === 1 ? 'ие' : 'ия'}</span></div><div class="calendar-event-list">${list.map((event) => {
        const titleText = event.topic || 'Персональное занятие';
        const student = role === 'admin' && event.user_name ? `<small>${escapeHTML(event.user_name)}</small>` : '';
        const action = role === 'admin'
          ? `<a href="/study/admin/lesson?id=${encodeURIComponent(event.id)}">Материалы / ДЗ →</a>`
          : (event.meeting_url ? `<a href="${escapeHTML(event.meeting_url)}" target="_blank" rel="noopener noreferrer">Войти в конференцию ↗</a>` : '<span>Ссылка появится позже</span>');
        return `<article class="calendar-event ${escapeHTML(event.status)}"><time>${escapeHTML(event.time)}</time><div><b>${escapeHTML(titleText)}</b>${student}<em>${escapeHTML(event.status)}${event.rescheduled ? ' · перенесено' : ''}</em></div>${action}</article>`;
      }).join('')}</div>`;
    };

    const render = () => {
      const map = byDate();
      title.textContent = `${monthNames[currentMonth.getMonth()]} ${currentMonth.getFullYear()}`;
      grid.innerHTML = '';
      const first = new Date(currentMonth.getFullYear(), currentMonth.getMonth(), 1);
      const last = new Date(currentMonth.getFullYear(), currentMonth.getMonth() + 1, 0);
      const leading = (first.getDay() + 6) % 7;
      for (let i=0; i<leading; i+=1) {
        const spacer = document.createElement('span');
        spacer.className = 'lesson-calendar-cell spacer';
        grid.appendChild(spacer);
      }
      for (let day=1; day<=last.getDate(); day+=1) {
        const date = new Date(currentMonth.getFullYear(), currentMonth.getMonth(), day);
        const key = dateKey(date);
        const list = map.get(key) || [];
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'lesson-calendar-cell';
        if (key === selected) button.classList.add('selected');
        if (key === dateKey(new Date())) button.classList.add('today');
        button.innerHTML = `<span>${day}</span>${list.length ? `<b>${list.length}</b><i>${list.slice(0,3).map((e) => `<u class="${escapeHTML(e.status)}"></u>`).join('')}</i>` : ''}`;
        button.addEventListener('click', () => { selected = key; render(); renderDay(); });
        grid.appendChild(button);
      }
    };

    prev?.addEventListener('click', () => { currentMonth = new Date(currentMonth.getFullYear(), currentMonth.getMonth()-1, 1); render(); });
    next?.addEventListener('click', () => { currentMonth = new Date(currentMonth.getFullYear(), currentMonth.getMonth()+1, 1); render(); });

    try {
      const response = await fetch(endpoint, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Не удалось загрузить календарь');
      events = Array.isArray(data.events) ? data.events : [];
      const today = new Date(new Date().getFullYear(), new Date().getMonth(), new Date().getDate());
      const upcoming = events.filter((event) => parseDate(event.date) >= today).sort((a, b) => `${a.date} ${a.time}`.localeCompare(`${b.date} ${b.time}`))[0];
      if (upcoming) {
        const d = parseDate(upcoming.date);
        currentMonth = new Date(d.getFullYear(), d.getMonth(), 1);
        selected = upcoming.date;
      }
      render();
      renderDay();
    } catch (error) {
      grid.innerHTML = `<div class="calendar-load-error">${escapeHTML(error.message)}</div>`;
      dayPanel.innerHTML = '<span>Календарь временно недоступен.</span>';
    }
  });
})();
