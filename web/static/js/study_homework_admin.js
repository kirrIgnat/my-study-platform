(() => {
  const search = document.querySelector('[data-homework-search]');
  const difficulty = document.querySelector('[data-homework-difficulty]');
  const items = Array.from(document.querySelectorAll('[data-homework-item]'));
  if (!items.length || (!search && !difficulty)) return;

  const apply = () => {
    const query = (search?.value || '').trim().toLocaleLowerCase('ru');
    const level = difficulty?.value || '';
    items.forEach((item) => {
      const haystack = (item.dataset.search || '').toLocaleLowerCase('ru');
      const matchesText = !query || haystack.includes(query);
      const matchesLevel = !level || item.dataset.difficulty === level;
      item.hidden = !(matchesText && matchesLevel);
    });
  };

  search?.addEventListener('input', apply);
  difficulty?.addEventListener('change', apply);
})();
