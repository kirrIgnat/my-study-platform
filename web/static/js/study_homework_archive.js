(() => {
  const toggle = document.querySelector('[data-completed-homework-toggle]');
  const panel = document.querySelector('[data-completed-homework-panel]');
  if (!toggle || !panel) return;

  const search = panel.querySelector('[data-completed-homework-search]');
  const items = Array.from(panel.querySelectorAll('[data-completed-homework-item]'));
  const empty = panel.querySelector('[data-completed-homework-empty]');

  const setOpen = (open) => {
    panel.hidden = !open;
    toggle.setAttribute('aria-expanded', String(open));
    toggle.classList.toggle('is-open', open);
    if (open && search) {
      requestAnimationFrame(() => search.focus({preventScroll: true}));
    }
  };

  toggle.addEventListener('click', () => setOpen(panel.hidden));

  if (!search) return;
  search.addEventListener('input', () => {
    const query = search.value.trim().toLocaleLowerCase('ru');
    let visible = 0;
    for (const item of items) {
      const haystack = (item.dataset.searchText || '').toLocaleLowerCase('ru');
      const match = query === '' || haystack.includes(query);
      item.hidden = !match;
      if (match) visible++;
    }
    if (empty) empty.hidden = visible !== 0;
  });
})();
