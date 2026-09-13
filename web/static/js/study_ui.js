(() => {
  'use strict';

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  const revealSelectors = [
    '.study-nav',
    '.study-hero-copy',
    '.study-terminal-card',
    '.section-heading',
    '.home-coming-card',
    '.process-grid article',
    '.page-title',
    '.booking-workspace > *',
    '.auth-copy > *',
    '.auth-card',
    '.dashboard-hello',
    '.dashboard-grid > *',
    '.stats-grid > *',
    '.admin-layout > *',
    '.lesson-admin-grid > *',
    '.temporary-password-card',
    '.upcoming-lesson-card',
    '.lesson-material-card',
    '.coming-code',
    '.coming-copy',
    '.coming-status'
  ];

  function setupRevealMotion() {
    if (reducedMotion) return;

    const items = [...document.querySelectorAll(revealSelectors.join(','))];
    if (!items.length) return;

    document.body.classList.add('motion-enabled');
    items.forEach((item, index) => {
      item.classList.add('motion-reveal');
      item.style.setProperty('--reveal-delay', `${Math.min(index % 6, 5) * 70}ms`);
    });

    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        entry.target.classList.add('is-visible');
        observer.unobserve(entry.target);
      }
    }, { threshold: 0.12, rootMargin: '0px 0px -4% 0px' });

    items.forEach((item) => observer.observe(item));
  }

  const snippets = [
    {
      language: 'C',
      filename: 'learning_path.c',
      lines: [
        '<b class="code-keyword">int</b> learn(<b class="code-keyword">const char</b> *topic) {',
        '    Theory theory = understand(topic);',
        '    Practice practice = build(&theory);',
        '    Feedback feedback = review(&practice);',
        ' ',
        '    <b class="code-keyword">return</b> iterate(&feedback);',
        '}'
      ]
    },
    {
      language: 'C++',
      filename: 'learning_path.cpp',
      lines: [
        'Result learn(std::string_view topic) {',
        '    <b class="code-keyword">auto</b> theory = understand(topic);',
        '    <b class="code-keyword">auto</b> practice = build(theory);',
        '    <b class="code-keyword">auto</b> feedback = review(practice);',
        ' ',
        '    <b class="code-keyword">return</b> iterate(feedback);',
        '}'
      ]
    },
    {
      language: 'GO',
      filename: 'learning_path.go',
      lines: [
        '<b class="code-keyword">func</b> Learn(topic string) Result {',
        '    theory := Understand(topic)',
        '    practice := Build(theory)',
        '    feedback := Review(practice)',
        ' ',
        '    <b class="code-keyword">return</b> Iterate(feedback)',
        '}'
      ]
    },
    {
      language: 'SQL',
      filename: 'learning_path.sql',
      lines: [
        '<b class="code-keyword">WITH</b> practice <b class="code-keyword">AS</b> (',
        '    <b class="code-keyword">SELECT</b> understand(topic) <b class="code-keyword">AS</b> theory,',
        '           build(topic) <b class="code-keyword">AS</b> result',
        '    <b class="code-keyword">FROM</b> learning_path',
        ')',
        '<b class="code-keyword">SELECT</b> iterate(review(result))',
        '<b class="code-keyword">FROM</b> practice;'
      ]
    },
    {
      language: 'PY',
      filename: 'learning_path.py',
      lines: [
        '<b class="code-keyword">def</b> learn(topic: str) -> Result:',
        '    theory = understand(topic)',
        '    practice = build(theory)',
        '    feedback = review(practice)',
        ' ',
        '    <b class="code-keyword">return</b> iterate(feedback)',
        ' '
      ]
    }
  ];

  function setupCodeRotator() {
    const card = document.querySelector('[data-code-rotator]');
    if (!card) return;

    const output = card.querySelector('[data-code-output]');
    const filename = card.querySelector('[data-code-filename]');
    const language = card.querySelector('[data-code-language]');
    if (!output || !filename || !language) return;

    let index = 2;
    let timer = null;
    let paused = false;

    const render = (snippet) => {
      output.innerHTML = snippet.lines
        .map((line, lineIndex) => `<span class="code-line" style="--line-index:${lineIndex}">${line || ' '}</span>`)
        .join('');
      filename.textContent = snippet.filename;
      language.textContent = snippet.language;
    };

    const switchSnippet = () => {
      if (paused || document.hidden) return;
      card.classList.add('is-code-leaving');
      window.setTimeout(() => {
        index = (index + 1) % snippets.length;
        render(snippets[index]);
        card.classList.remove('is-code-leaving');
        card.classList.add('is-code-entering');
        window.setTimeout(() => card.classList.remove('is-code-entering'), 850);
      }, 360);
    };

    render(snippets[index]);
    if (reducedMotion) return;

    const start = () => {
      window.clearInterval(timer);
      timer = window.setInterval(switchSnippet, 5400);
    };

    card.addEventListener('mouseenter', () => { paused = true; });
    card.addEventListener('mouseleave', () => { paused = false; });
    card.addEventListener('focusin', () => { paused = true; });
    card.addEventListener('focusout', () => { paused = false; });
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) start();
    });
    start();
  }


  function setupPasswordToggles() {
    document.querySelectorAll('[data-password-toggle]').forEach((button) => {
      const wrap = button.closest('.password-input-wrap');
      const input = wrap?.querySelector('input');
      if (!input) return;
      button.addEventListener('click', () => {
        const showing = input.type === 'text';
        input.type = showing ? 'password' : 'text';
        button.textContent = showing ? 'Показать' : 'Скрыть';
        button.setAttribute('aria-label', showing ? 'Показать пароль' : 'Скрыть пароль');
      });
    });
  }

  document.querySelectorAll('form[data-confirm]').forEach((form) => {
    form.addEventListener('submit', (event) => {
      const message = form.dataset.confirm || 'Продолжить?';
      if (!window.confirm(message)) event.preventDefault();
    });
  });

  setupRevealMotion();
  setupCodeRotator();
  setupPasswordToggles();
})();
