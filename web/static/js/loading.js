window.addEventListener('load', () => {
  window.setTimeout(() => {
    document.body.classList.add('launch');
    window.setTimeout(() => window.location.replace('/profile'), 420);
  }, 1050);
});
