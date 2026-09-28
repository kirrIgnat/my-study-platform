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
  const statusLabel = (status) => status === 'correct' ? 'Правильно' : status === 'incorrect' ? 'Неправильно' : 'Не проверено';
  const assignmentDate = (value) => {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('ru-RU',{day:'2-digit',month:'2-digit',year:'numeric'}).format(date);
  };
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
    dayPanel.innerHTML = `<div class="calendar-day-panel-head"><b>${escapeHTML(human)}</b><span>${list.length ? `${list.length} занят.` : 'занятий нет'}</span></div>${list.length ? `<div class="calendar-event-list">${list.map((event)=>`<article class="calendar-event ${escapeHTML(event.status)}"><time>${escapeHTML(event.time)}${event.end_time ? `–${escapeHTML(event.end_time)}` : ''}</time><div><b>${escapeHTML(event.topic || 'Персональное занятие')}</b><em>${escapeHTML(event.status)}${event.rescheduled ? ' · перенесено' : ''}</em></div><button type="button" data-edit-booking="${escapeHTML(event.id)}">ДЗ / материалы →</button></article>`).join('')}</div>` : ''}`;
    dayPanel.querySelectorAll('[data-edit-booking]').forEach((btn)=>btn.addEventListener('click',()=>selectLesson(btn.dataset.editBooking)));
  };

  const reviewCardHTML = (assignment) => {
    const task = assignment.Task || {};
    const status = assignment.Status || 'assigned';
    return `<article class="student-homework-review-card ${escapeHTML(status)}" data-review-assignment="${escapeHTML(assignment.ID)}">
      <div class="student-homework-review-main">
        <div class="task-badges"><span>${escapeHTML(task.Topic || 'Без темы')}</span>${task.Language ? `<span>${escapeHTML(task.Language)}</span>` : ''}<span class="difficulty ${escapeHTML(task.Difficulty || '')}">${escapeHTML(task.DifficultyLabel || task.Difficulty || '')}</span></div>
        <b>${escapeHTML(task.Title || 'Задача')}</b>
        <small>Выдана ${escapeHTML(assignmentDate(assignment.AssignedAt))} · ${escapeHTML(statusLabel(status))}</small>
      </div>
      <input class="student-homework-note" type="text" maxlength="2000" value="${escapeHTML(assignment.TeacherNote || '')}" placeholder="Комментарий ученику (необязательно)">
      <div class="student-homework-verdicts">
        <button class="homework-verdict correct ${status === 'correct' ? 'active' : ''}" type="button" data-homework-verdict="correct">✓ Правильно</button>
        <button class="homework-verdict incorrect ${status === 'incorrect' ? 'active' : ''}" type="button" data-homework-verdict="incorrect">× Неправильно</button>
      </div>
    </article>`;
  };

  const homeworkReviewHTML = () => {
    const assignments = Array.isArray(snapshot?.homework_assignments) ? snapshot.homework_assignments : [];
    const pending = assignments.filter((item) => item.Status === 'assigned');
    const incorrect = assignments.filter((item) => item.Status === 'incorrect');
    const correct = assignments.filter((item) => item.Status === 'correct');
    return `<section class="student-inspector-section student-homework-review-section">
      <div class="panel-head"><div><span class="panel-label">ПРОВЕРКА ДЗ</span><h3>Результаты задач ученика</h3></div><span class="counter">${pending.length}</span></div>
      <p class="panel-note">После «Правильно» задача больше не предлагается этому ученику в новых ДЗ. После «Неправильно» она остаётся доступной и помечается как задача на повторение.</p>
      <div class="student-homework-review-groups">
        <div><div class="subsection-title"><span>ЖДУТ ПРОВЕРКИ</span><b>${pending.length}</b></div>${pending.length ? `<div class="student-homework-review-list">${pending.map(reviewCardHTML).join('')}</div>` : '<div class="empty-state compact"><b>Непроверенных задач нет</b></div>'}</div>
        <div><div class="subsection-title"><span>НУЖНО ПОВТОРИТЬ</span><b>${incorrect.length}</b></div>${incorrect.length ? `<div class="student-homework-review-list">${incorrect.map(reviewCardHTML).join('')}</div>` : '<div class="empty-state compact"><b>Ошибочных задач нет</b></div>'}</div>
      </div>
      ${correct.length ? `<details class="student-homework-correct-history"><summary>Правильно решённые задачи · ${correct.length}</summary><div class="student-homework-review-list">${correct.map(reviewCardHTML).join('')}</div></details>` : ''}
      <div class="dialog-status" id="studentHomeworkReviewStatus"></div>
    </section>`;
  };

  const bindHomeworkReview = () => {
    body.querySelectorAll('[data-homework-verdict]').forEach((button) => button.addEventListener('click', async () => {
      const card = button.closest('[data-review-assignment]');
      const statusBox = body.querySelector('#studentHomeworkReviewStatus');
      if (!card || !statusBox) return;
      const assignmentID = card.dataset.reviewAssignment || '';
      const verdict = button.dataset.homeworkVerdict || '';
      const note = card.querySelector('.student-homework-note')?.value || '';
      const selectedBooking = body.querySelector('#studentLessonSelect')?.value || '';
      statusBox.textContent = verdict === 'correct' ? 'Отмечаю задачу как правильную…' : 'Отмечаю задачу для повторения…';
      button.disabled = true;
      try {
        const response = await fetch('/api/study/admin/homework/status', {
          method:'POST',
          headers:{'Content-Type':'application/json',Accept:'application/json','X-CSRF-Token':csrf},
          body:JSON.stringify({assignment_id:assignmentID,status:verdict,teacher_note:note})
        });
        const data = await response.json().catch(()=>({}));
        if (!response.ok) throw new Error(data.error || 'Не удалось сохранить проверку');
        await fetchStudent();
        renderSnapshot(selectedBooking);
        const newStatus = body.querySelector('#studentHomeworkReviewStatus');
        if (newStatus) newStatus.textContent = verdict === 'correct' ? 'Сохранено: задача зачтена и убрана из будущего выбора.' : 'Сохранено: задача доступна для повторного назначения.';
      } catch (error) {
        button.disabled = false;
        statusBox.textContent = error.message;
      }
    }));
  };

  const renderHomeworkPicker = (bookingID) => {
    const host = body.querySelector('#studentHomeworkPicker');
    if (!host) return;
    const choices = Array.isArray(snapshot?.homework_choices) ? snapshot.homework_choices : [];
    const assignments = Array.isArray(snapshot?.homework_assignments) ? snapshot.homework_assignments : [];
    const current = assignments.filter((item) => item.BookingID === bookingID);
    host.innerHTML = `<div class="student-homework-picker-head"><div><span class="panel-label">НОВОЕ ДЗ</span><h4>Задачи из базы</h4></div><span>${choices.length} доступно</span></div>
      <p class="panel-note">Правильно решённые и уже ожидающие проверки задачи здесь не показываются. Задачи с ошибкой можно назначать повторно.</p>
      ${current.length ? `<div class="student-current-homework"><div class="subsection-title"><span>УЖЕ В ЭТОМ ДЗ</span><b>${current.length}</b></div>${current.map((item)=>`<div><b>${escapeHTML(item.Task?.Title || 'Задача')}</b><span class="task-history-state ${escapeHTML(item.Status)}">${escapeHTML(statusLabel(item.Status))}</span></div>`).join('')}</div>` : ''}
      ${choices.length ? `<div class="student-homework-choice-tools"><input type="search" data-student-homework-search placeholder="Поиск по названию или теме"></div><div class="student-homework-choice-list">${choices.map((choice)=>`<label class="student-homework-choice ${choice.LastStatus === 'incorrect' ? 'recommended-repeat' : ''}" data-student-homework-choice data-search="${escapeHTML(`${choice.Task?.Title || ''} ${choice.Task?.Topic || ''} ${choice.Task?.Language || ''}`.toLowerCase())}"><input type="checkbox" value="${escapeHTML(choice.Task?.ID || '')}"><span><b>${escapeHTML(choice.Task?.Title || 'Задача')}</b><small>${escapeHTML(choice.Task?.Topic || '')}${choice.Task?.Language ? ` · ${escapeHTML(choice.Task.Language)}` : ''} · ${escapeHTML(choice.Task?.DifficultyLabel || '')}${choice.LastStatus === 'incorrect' ? ' · ↻ повторить' : ' · ещё не решена'}</small></span></label>`).join('')}</div><button class="study-button primary wide" type="button" data-assign-student-homework>Добавить выбранные задачи в ДЗ</button>` : '<div class="empty-state compact"><b>Нет задач для добавления</b><span>Ученик либо уже решил доступные задачи, либо оставшиеся сейчас ждут проверки.</span></div>'}
      <div class="dialog-status" id="studentHomeworkAssignStatus"></div>`;

    const search = host.querySelector('[data-student-homework-search]');
    search?.addEventListener('input', () => {
      const query = search.value.trim().toLowerCase();
      host.querySelectorAll('[data-student-homework-choice]').forEach((item) => { item.hidden = query !== '' && !item.dataset.search.includes(query); });
    });
    host.querySelector('[data-assign-student-homework]')?.addEventListener('click', async () => {
      const statusBox = host.querySelector('#studentHomeworkAssignStatus');
      const taskIDs = [...host.querySelectorAll('[data-student-homework-choice] input:checked')].map((input)=>input.value).filter(Boolean);
      if (!taskIDs.length) { if (statusBox) statusBox.textContent='Выберите хотя бы одну задачу.'; return; }
      if (statusBox) statusBox.textContent='Добавляю задачи…';
      try {
        const response = await fetch('/api/study/admin/homework/assign', {
          method:'POST',
          headers:{'Content-Type':'application/json',Accept:'application/json','X-CSRF-Token':csrf},
          body:JSON.stringify({user_id:activeStudentID,booking_id:bookingID,task_ids:taskIDs})
        });
        const data = await response.json().catch(()=>({}));
        if (!response.ok) throw new Error(data.error || 'Не удалось назначить задачи');
        await fetchStudent();
        renderSnapshot(bookingID);
        const newStatus = body.querySelector('#studentHomeworkAssignStatus');
        if (newStatus) newStatus.textContent=`Добавлено задач: ${data.assigned || taskIDs.length}.`;
      } catch (error) {
        if (statusBox) statusBox.textContent=error.message;
      }
    });
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
    renderHomeworkPicker(bookingID);
  };

  const saveLesson = async (publish) => {
    const bookingID = body.querySelector('#studentLessonSelect')?.value || '';
    const status = body.querySelector('#studentLessonSaveStatus');
    if (!bookingID) { if (status) status.textContent='Выберите занятие.'; return; }
    if (status) status.textContent = publish ? 'Публикую материалы…' : 'Сохраняю черновик…';
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
    if (!response.ok) { if (status) status.textContent=data.error || 'Не удалось сохранить'; return; }
    await fetchStudent();
    renderSnapshot(bookingID);
    const newStatus = body.querySelector('#studentLessonSaveStatus');
    if (newStatus) newStatus.textContent = publish ? 'Готово: ученик получил материалы.' : 'Черновик сохранён.';
  };

  const renderSnapshot = (preferredBookingID = '') => {
    const student = snapshot.student;
    title.textContent = student.name;
    meta.textContent = `${student.email}${student.telegram ? ` · ${student.telegram}` : ''}`;
    const schedules = Array.isArray(snapshot.schedules) ? snapshot.schedules : [];
    const lessons = Array.isArray(snapshot.lessons) ? snapshot.lessons : [];
    const events = Array.isArray(snapshot.events) ? snapshot.events : [];
    const assignments = Array.isArray(snapshot.homework_assignments) ? snapshot.homework_assignments : [];
    const pendingHomework = assignments.filter((item)=>item.Status === 'assigned').length;
    const incorrectHomework = assignments.filter((item)=>item.Status === 'incorrect').length;
    const today = new Date(new Date().getFullYear(),new Date().getMonth(),new Date().getDate());
    const upcoming = events.filter((event)=>parseDate(event.date) >= today).sort((a,b)=>`${a.date} ${a.time}`.localeCompare(`${b.date} ${b.time}`))[0];
    if (upcoming && !preferredBookingID) { const d=parseDate(upcoming.date); calendarMonth=new Date(d.getFullYear(),d.getMonth(),1); selectedDate=upcoming.date; }

    body.innerHTML = `
      <section class="student-inspector-summary">
        <div><span>Регулярных занятий</span><b>${schedules.length}</b></div>
        <div><span>Ждут проверки</span><b>${pendingHomework}</b></div>
        <div><span>Нужно повторить</span><b>${incorrectHomework}</b></div>
      </section>
      ${homeworkReviewHTML()}
      <section class="student-inspector-section"><div class="panel-head"><div><span class="panel-label">ПОСТОЯННОЕ РАСПИСАНИЕ</span><h3>Слоты ученика</h3></div></div><div class="recurring-schedule-chips">${schedules.length ? schedules.map((s)=>`<div class="recurring-schedule-chip"><span>${escapeHTML(s.WeekdayName)}</span><b>${escapeHTML(s.Time)}${s.EndTime ? `–${escapeHTML(s.EndTime)}` : ''}</b><small>${escapeHTML(s.Topic || 'Персональное занятие')}</small></div>`).join('') : '<div class="empty-state compact"><b>Постоянных слотов нет</b></div>'}</div></section>
      <section class="student-inspector-section"><div class="panel-head"><div><span class="panel-label">КАЛЕНДАРЬ</span><h3>Все занятия ученика</h3></div></div><div class="lesson-calendar embedded-student-calendar"><div class="lesson-calendar-toolbar"><button type="button" data-student-calendar-prev>←</button><strong data-student-calendar-title>Календарь</strong><button type="button" data-student-calendar-next>→</button></div><div class="lesson-calendar-weekdays"><span>ПН</span><span>ВТ</span><span>СР</span><span>ЧТ</span><span>ПТ</span><span>СБ</span><span>ВС</span></div><div class="lesson-calendar-grid" data-student-calendar-grid></div><div class="lesson-calendar-day" data-student-calendar-day></div></div></section>
      <section class="student-inspector-section student-lesson-editor"><div class="panel-head"><div><span class="panel-label">МАТЕРИАЛЫ И НОВОЕ ДЗ</span><h3>Конкретное занятие</h3></div></div>
        ${lessons.length ? `<div class="student-material-index">${lessons.map((l)=>`<button type="button" data-material-booking="${escapeHTML(l.BookingID)}"><span><b>${escapeHTML(l.Date)} · ${escapeHTML(l.Time)}${l.EndTime ? `–${escapeHTML(l.EndTime)}` : ''}</b><small>${escapeHTML(l.Title || l.Topic || 'Материалы занятия')}</small></span><em class="${l.Published ? 'published' : 'draft'}">${l.Published ? 'опубликовано' : (l.Homework || l.Theory || l.VideoURL ? 'черновик' : 'без материалов')}</em></button>`).join('')}</div><label class="inspector-field"><span>Занятие</span><select id="studentLessonSelect">${lessons.map((l)=>`<option value="${escapeHTML(l.BookingID)}">${escapeHTML(l.Date)} · ${escapeHTML(l.Time)}${l.EndTime ? `–${escapeHTML(l.EndTime)}` : ''} · ${escapeHTML(l.Topic || l.Title || 'Занятие')}</option>`).join('')}</select></label>
        <div class="inspector-form-grid"><label class="inspector-field"><span>Название урока</span><input id="studentLessonTitle" maxlength="180"></label><label class="inspector-field"><span>Ссылка на видео</span><input id="studentLessonVideo" type="url" placeholder="https://..."></label></div>
        <label class="inspector-field"><span>Теоретический материал · Markdown</span><textarea id="studentLessonTheory" rows="7"></textarea></label>
        <label class="inspector-field homework"><span>Старое текстовое ДЗ <small>необязательно</small></span><textarea id="studentLessonHomework" rows="4"></textarea></label>
        <div class="inspector-material-state" id="studentLessonState"></div><div class="student-inspector-actions"><button class="study-button secondary small" type="button" data-save-draft>Сохранить черновик</button><button class="study-button primary small" type="button" data-save-publish>Сохранить и отправить ученику</button></div><div class="dialog-status" id="studentLessonSaveStatus"></div>
        <div class="student-homework-picker" id="studentHomeworkPicker"></div>` : '<div class="empty-state"><b>У ученика пока нет занятий</b><span>После появления занятия здесь можно будет выдать ДЗ и материалы.</span></div>'}
      </section>`;

    body.querySelector('[data-student-calendar-prev]')?.addEventListener('click',()=>{calendarMonth=new Date(calendarMonth.getFullYear(),calendarMonth.getMonth()-1,1);renderCalendar();});
    body.querySelector('[data-student-calendar-next]')?.addEventListener('click',()=>{calendarMonth=new Date(calendarMonth.getFullYear(),calendarMonth.getMonth()+1,1);renderCalendar();});
    body.querySelector('#studentLessonSelect')?.addEventListener('change',(event)=>selectLesson(event.target.value));
    body.querySelectorAll('[data-material-booking]').forEach((button)=>button.addEventListener('click',()=>selectLesson(button.dataset.materialBooking)));
    body.querySelector('[data-save-draft]')?.addEventListener('click',()=>saveLesson(false));
    body.querySelector('[data-save-publish]')?.addEventListener('click',()=>saveLesson(true));
    bindHomeworkReview();
    renderCalendar();
    if (lessons.length) {
      const preferred = lessons.some((lesson)=>lesson.BookingID === preferredBookingID) ? preferredBookingID : lessons[0].BookingID;
      selectLesson(preferred);
    }
  };

  const openStudent = async (id) => {
    activeStudentID=id;
    title.textContent='Загрузка…'; meta.textContent='';
    body.innerHTML='<div class="empty-state"><b>Загружаю календарь, материалы и домашние задания…</b></div>';
    dialog.showModal();
    try { await fetchStudent(); renderSnapshot(); }
    catch (error) { body.innerHTML=`<div class="empty-state"><b>Не удалось открыть ученика</b><span>${escapeHTML(error.message)}</span></div>`; }
  };

  document.querySelectorAll('[data-open-student]').forEach((button)=>button.addEventListener('click',()=>openStudent(button.dataset.studentId)));
  document.querySelector('[data-close-student]')?.addEventListener('click',()=>dialog.close());
  dialog.addEventListener('click',(event)=>{ if(event.target===dialog) dialog.close(); });
})();
