(() => {
  'use strict';

  const dialog = document.getElementById('studentInspectorDialog');
  if (!dialog) return;
  const title = document.getElementById('studentInspectorTitle');
  const meta = document.getElementById('studentInspectorMeta');
  const body = document.getElementById('studentInspectorBody');
  const csrf = document.querySelector('meta[name="csrf-token"]')?.content || '';
  const monthNames = ['Январь','Февраль','Март','Апрель','Май','Июнь','Июль','Август','Сентябрь','Октябрь','Ноябрь','Декабрь'];
  const escapeHTML = (value = '') => String(value).replace(/[&<>'"]/g, (ch) => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[ch]));
  const parseDate = (value) => { const [y,m,d] = value.split('-').map(Number); return new Date(y,m-1,d); };
  const dateKey = (date) => `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
  let activeStudentID = '';
  let snapshot = null;
  let calendarMonth = new Date(new Date().getFullYear(), new Date().getMonth(), 1);
  let selectedDate = dateKey(new Date());

  const fetchStudent = async () => {
    const response = await fetch(`/api/study/admin/student?user_id=${encodeURIComponent(activeStudentID)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || 'Не удалось загрузить ученика');
    snapshot = data;
    return data;
  };

  const renderCalendar = () => {
    const grid = body.querySelector('[data-student-calendar-grid]');
    const monthTitle = body.querySelector('[data-student-calendar-title]');
    const dayPanel = body.querySelector('[data-student-calendar-day]');
    if (!grid || !monthTitle || !dayPanel) return;
    const events = Array.isArray(snapshot?.events) ? snapshot.events : [];
    const grouped = new Map();
    events.forEach((event) => { if (!grouped.has(event.date)) grouped.set(event.date, []); grouped.get(event.date).push(event); });
    grouped.forEach((items) => items.sort((a,b) => a.time.localeCompare(b.time)));
    monthTitle.textContent = `${monthNames[calendarMonth.getMonth()]} ${calendarMonth.getFullYear()}`;
    grid.innerHTML = '';
    const first = new Date(calendarMonth.getFullYear(), calendarMonth.getMonth(), 1);
    const last = new Date(calendarMonth.getFullYear(), calendarMonth.getMonth()+1, 0);
    const leading = (first.getDay()+6)%7;
    for (let i=0;i<leading;i+=1) { const spacer=document.createElement('span'); spacer.className='lesson-calendar-cell spacer'; grid.appendChild(spacer); }
    for (let d=1;d<=last.getDate();d+=1) {
      const date = new Date(calendarMonth.getFullYear(), calendarMonth.getMonth(), d);
      const key = dateKey(date);
      const list = grouped.get(key) || [];
      const button = document.createElement('button');
      button.type='button'; button.className='lesson-calendar-cell';
      if (selectedDate === key) button.classList.add('selected');
      if (key === dateKey(new Date())) button.classList.add('today');
      button.innerHTML = `<span>${d}</span>${list.length ? `<b>${list.length}</b><i>${list.slice(0,3).map((e)=>`<u class="${escapeHTML(e.status)}"></u>`).join('')}</i>` : ''}`;
      button.addEventListener('click', () => { selectedDate=key; renderCalendar(); });
      grid.appendChild(button);
    }
    const list = grouped.get(selectedDate) || [];
    const human = new Intl.DateTimeFormat('ru-RU',{weekday:'long',day:'numeric',month:'long',year:'numeric'}).format(parseDate(selectedDate));
    dayPanel.innerHTML = `<div class="calendar-day-panel-head"><b>${escapeHTML(human)}</b><span>${list.length ? `${list.length} занят.` : 'занятий нет'}</span></div>${list.length ? `<div class="calendar-event-list">${list.map((event)=>`<article class="calendar-event ${escapeHTML(event.status)}"><time>${escapeHTML(event.time)}</time><div><b>${escapeHTML(event.topic || 'Персональное занятие')}</b><em>${escapeHTML(event.status)}${event.rescheduled ? ' · перенесено' : ''}</em></div><button type="button" data-edit-booking="${escapeHTML(event.id)}">ДЗ / материалы →</button></article>`).join('')}</div>` : ''}`;
    dayPanel.querySelectorAll('[data-edit-booking]').forEach((btn)=>btn.addEventListener('click',()=>selectLesson(btn.dataset.editBooking)));
  };

  const selectLesson = (bookingID) => {
    const select = body.querySelector('#studentLessonSelect');
    if (select) select.value = bookingID;
    const lesson = (snapshot?.lessons || []).find((item) => item.BookingID === bookingID);
    if (!lesson) return;
    body.querySelector('#studentLessonTitle').value = lesson.Title || lesson.Topic || '';
    body.querySelector('#studentLessonTheory').value = lesson.Theory || '';
    body.querySelector('#studentLessonHomework').value = lesson.Homework || '';
    body.querySelector('#studentLessonVideo').value = lesson.VideoURL || '';
    const state = body.querySelector('#studentLessonState');
    state.textContent = lesson.Published ? 'Материалы уже опубликованы ученику.' : 'Черновик: ученик пока не видит материалы.';
    body.querySelector('.student-lesson-editor')?.scrollIntoView({behavior:'smooth',block:'nearest'});
  };

  const saveLesson = async (publish) => {
    const bookingID = body.querySelector('#studentLessonSelect')?.value || '';
    const status = body.querySelector('#studentLessonSaveStatus');
    if (!bookingID) { status.textContent='Выберите занятие.'; return; }
    status.textContent = publish ? 'Публикую материалы и ДЗ…' : 'Сохраняю черновик…';
    const response = await fetch('/api/study/admin/lesson/save', {
      method:'POST',
      headers:{'Content-Type':'application/json',Accept:'application/json','X-CSRF-Token':csrf},
      body:JSON.stringify({
        booking_id:bookingID,
        title:body.querySelector('#studentLessonTitle').value.trim(),
        theory:body.querySelector('#studentLessonTheory').value.trim(),
        homework:body.querySelector('#studentLessonHomework').value.trim(),
        video_url:body.querySelector('#studentLessonVideo').value.trim(),
        publish
      })
    });
    const data = await response.json().catch(()=>({}));
    if (!response.ok) { status.textContent=data.error || 'Не удалось сохранить'; return; }
    status.textContent = publish ? 'Готово: ученик получил материалы и уведомление.' : 'Черновик сохранён.';
    await fetchStudent();
    const savedID = bookingID;
    renderSnapshot();
    selectLesson(savedID);
    const newStatus = body.querySelector('#studentLessonSaveStatus');
    if (newStatus) newStatus.textContent = publish ? 'Готово: ученик получил материалы и уведомление.' : 'Черновик сохранён.';
  };

  const renderSnapshot = () => {
    const student = snapshot.student;
    title.textContent = student.name;
    meta.textContent = `${student.email}${student.telegram ? ` · ${student.telegram}` : ''}`;
    const schedules = Array.isArray(snapshot.schedules) ? snapshot.schedules : [];
    const lessons = Array.isArray(snapshot.lessons) ? snapshot.lessons : [];
    const events = Array.isArray(snapshot.events) ? snapshot.events : [];
    const today = new Date(new Date().getFullYear(),new Date().getMonth(),new Date().getDate());
    const upcoming = events.filter((event)=>parseDate(event.date) >= today).sort((a,b)=>`${a.date} ${a.time}`.localeCompare(`${b.date} ${b.time}`))[0];
    if (upcoming) { const d=parseDate(upcoming.date); calendarMonth=new Date(d.getFullYear(),d.getMonth(),1); selectedDate=upcoming.date; }

    body.innerHTML = `
      <section class="student-inspector-summary">
        <div><span>Регулярных занятий</span><b>${schedules.length}</b></div>
        <div><span>Всего занятий</span><b>${events.length}</b></div>
        <div><span>Опубликовано материалов</span><b>${lessons.filter((l)=>l.Published).length}</b></div>
      </section>
      <section class="student-inspector-section"><div class="panel-head"><div><span class="panel-label">ПОСТОЯННОЕ РАСПИСАНИЕ</span><h3>Слоты ученика</h3></div></div><div class="recurring-schedule-chips">${schedules.length ? schedules.map((s)=>`<div class="recurring-schedule-chip"><span>${escapeHTML(s.WeekdayName)}</span><b>${escapeHTML(s.Time)}</b><small>${escapeHTML(s.Topic || 'Персональное занятие')}</small></div>`).join('') : '<div class="empty-state compact"><b>Постоянных слотов нет</b></div>'}</div></section>
      <section class="student-inspector-section"><div class="panel-head"><div><span class="panel-label">КАЛЕНДАРЬ</span><h3>Все занятия ученика</h3></div></div><div class="lesson-calendar embedded-student-calendar"><div class="lesson-calendar-toolbar"><button type="button" data-student-calendar-prev>←</button><strong data-student-calendar-title>Календарь</strong><button type="button" data-student-calendar-next>→</button></div><div class="lesson-calendar-weekdays"><span>ПН</span><span>ВТ</span><span>СР</span><span>ЧТ</span><span>ПТ</span><span>СБ</span><span>ВС</span></div><div class="lesson-calendar-grid" data-student-calendar-grid></div><div class="lesson-calendar-day" data-student-calendar-day></div></div></section>
      <section class="student-inspector-section student-lesson-editor"><div class="panel-head"><div><span class="panel-label">МАТЕРИАЛЫ И ДЗ</span><h3>Материалы конкретного занятия</h3></div></div>
        ${lessons.length ? `<div class="student-material-index">${lessons.map((l)=>`<button type="button" data-material-booking="${escapeHTML(l.BookingID)}"><span><b>${escapeHTML(l.Date)} · ${escapeHTML(l.Time)}</b><small>${escapeHTML(l.Title || l.Topic || 'Материалы занятия')}</small></span><em class="${l.Published ? 'published' : 'draft'}">${l.Published ? 'опубликовано' : (l.Homework || l.Theory || l.VideoURL ? 'черновик' : 'без материалов')}</em></button>`).join('')}</div><label class="inspector-field"><span>Занятие для редактирования</span><select id="studentLessonSelect">${lessons.map((l)=>`<option value="${escapeHTML(l.BookingID)}">${escapeHTML(l.Date)} · ${escapeHTML(l.Time)} · ${escapeHTML(l.Topic || l.Title || 'Занятие')}</option>`).join('')}</select></label>
        <div class="inspector-form-grid"><label class="inspector-field"><span>Название урока</span><input id="studentLessonTitle" maxlength="180"></label><label class="inspector-field"><span>Ссылка на видео</span><input id="studentLessonVideo" type="url" placeholder="https://..."></label></div>
        <label class="inspector-field"><span>Теоретический материал</span><textarea id="studentLessonTheory" rows="7"></textarea></label>
        <label class="inspector-field homework"><span>Домашнее задание</span><textarea id="studentLessonHomework" rows="6"></textarea></label>
        <div class="inspector-material-state" id="studentLessonState"></div><div class="student-inspector-actions"><button class="study-button secondary small" type="button" data-save-draft>Сохранить черновик</button><button class="study-button primary small" type="button" data-save-publish>Сохранить и отправить ученику</button></div><div class="dialog-status" id="studentLessonSaveStatus"></div>` : '<div class="empty-state"><b>У ученика пока нет занятий</b><span>После появления занятия здесь можно будет выдать ДЗ и материалы.</span></div>'}
      </section>`;

    body.querySelector('[data-student-calendar-prev]')?.addEventListener('click',()=>{calendarMonth=new Date(calendarMonth.getFullYear(),calendarMonth.getMonth()-1,1);renderCalendar();});
    body.querySelector('[data-student-calendar-next]')?.addEventListener('click',()=>{calendarMonth=new Date(calendarMonth.getFullYear(),calendarMonth.getMonth()+1,1);renderCalendar();});
    body.querySelector('#studentLessonSelect')?.addEventListener('change',(event)=>selectLesson(event.target.value));
    body.querySelectorAll('[data-material-booking]').forEach((button)=>button.addEventListener('click',()=>selectLesson(button.dataset.materialBooking)));
    body.querySelector('[data-save-draft]')?.addEventListener('click',()=>saveLesson(false));
    body.querySelector('[data-save-publish]')?.addEventListener('click',()=>saveLesson(true));
    renderCalendar();
    if (lessons.length) selectLesson(lessons[0].BookingID);
  };

  const openStudent = async (id) => {
    activeStudentID=id;
    title.textContent='Загрузка…'; meta.textContent='';
    body.innerHTML='<div class="empty-state"><b>Загружаю календарь и материалы…</b></div>';
    dialog.showModal();
    try { await fetchStudent(); renderSnapshot(); }
    catch (error) { body.innerHTML=`<div class="empty-state"><b>Не удалось открыть ученика</b><span>${escapeHTML(error.message)}</span></div>`; }
  };

  document.querySelectorAll('[data-open-student]').forEach((button)=>button.addEventListener('click',()=>openStudent(button.dataset.studentId)));
  document.querySelector('[data-close-student]')?.addEventListener('click',()=>dialog.close());
  dialog.addEventListener('click',(event)=>{ if(event.target===dialog) dialog.close(); });
})();
