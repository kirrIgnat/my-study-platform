(() => {
  const language = document.querySelector('[data-homework-language]');
  if (!language) return;

  const extensionByLanguage = {
    C: '.c',
    'C++': '.cpp',
    Go: '.go',
    Python: '.py'
  };

  const taskID = document.querySelector('.homework-id-chip')?.textContent?.trim() || 'id_t1';
  const fileNodes = Array.from(document.querySelectorAll('[data-homework-file]'));

  const syncFilename = () => {
    const ext = extensionByLanguage[language.value] || '.c';
    const filename = `${taskID}${ext}`;
    fileNodes.forEach((node) => { node.textContent = filename; });
  };

  language.addEventListener('change', syncFilename);
  syncFilename();

  const editor = document.querySelector('.homework-code-editor:not([readonly])');
  if (!editor) return;
  editor.addEventListener('keydown', (event) => {
    if (event.key !== 'Tab') return;
    event.preventDefault();
    const start = editor.selectionStart;
    const end = editor.selectionEnd;
    editor.setRangeText('    ', start, end, 'end');
  });
})();
