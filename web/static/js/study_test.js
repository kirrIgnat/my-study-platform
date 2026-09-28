(() => {
  const form = document.querySelector('[data-test-form]');
  if (!form) return;

  const timerNode = document.querySelector('[data-test-timer]');
  const timerWrap = timerNode ? timerNode.closest('.test-timer') : null;
  const saveState = document.querySelector('[data-save-state]');
  const expiresAt = Number(form.dataset.expiresAt || 0) * 1000;
  let saveTimer = 0;
  let submitting = false;
  let lastSnapshot = '';

  const snapshot = () => new URLSearchParams(new FormData(form)).toString();

  const setState = (text, cls = '') => {
    if (!saveState) return;
    saveState.textContent = text;
    saveState.classList.remove('is-saving', 'is-error');
    if (cls) saveState.classList.add(cls);
  };

  const save = async () => {
    if (submitting) return;
    const body = new URLSearchParams(new FormData(form));
    const current = body.toString();
    if (current === lastSnapshot) return;
    setState('Сохраняю…', 'is-saving');
    try {
      const response = await fetch('/study/test/save', {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8'},
        credentials: 'same-origin',
        body: current,
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        if (payload.status === 'expired' || payload.status === 'submitted') {
          window.location.reload();
          return;
        }
        throw new Error(payload.error || 'save failed');
      }
      lastSnapshot = current;
      setState('Ответы сохранены');
    } catch (_) {
      setState('Не удалось сохранить. Проверьте соединение.', 'is-error');
    }
  };

  const queueSave = () => {
    window.clearTimeout(saveTimer);
    saveTimer = window.setTimeout(save, 450);
  };

  form.addEventListener('change', queueSave);
  window.setInterval(save, 10000);

  const renderTimer = () => {
    if (!timerNode || !expiresAt) return;
    const left = Math.max(0, expiresAt - Date.now());
    const total = Math.ceil(left / 1000);
    const minutes = Math.floor(total / 60);
    const seconds = total % 60;
    timerNode.textContent = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
    if (timerWrap) timerWrap.classList.toggle('is-low', total <= 120);
    if (left <= 0 && !submitting) {
      submitting = true;
      setState('Время закончилось. Отправляю ответы…', 'is-saving');
      HTMLFormElement.prototype.submit.call(form);
    }
  };

  renderTimer();
  window.setInterval(renderTimer, 250);

  form.addEventListener('submit', () => {
    submitting = true;
    setState('Отправляю ответы…', 'is-saving');
  });
})();
