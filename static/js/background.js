(() => {
  const canvas = document.getElementById('fractalCanvas');
  if (!canvas) return;

  const ctx = canvas.getContext('2d');
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  let width = 0;
  let height = 0;
  let dpr = 1;
  let mouseX = 0;
  let mouseY = 0;
  let targetX = 0;
  let targetY = 0;
  let raf = 0;

  const motifs = [
    { x: .14, y: .18, size: 118, angle: -.2, depth: 4 },
    { x: .82, y: .26, size: 145, angle: 2.15, depth: 4 },
    { x: .22, y: .78, size: 132, angle: .65, depth: 4 },
    { x: .78, y: .76, size: 105, angle: 3.35, depth: 3 }
  ];

  function resize() {
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    width = window.innerWidth;
    height = window.innerHeight;
    canvas.width = Math.round(width * dpr);
    canvas.height = Math.round(height * dpr);
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    draw();
  }

  function branch(x, y, len, angle, depth, phase) {
    if (depth <= 0 || len < 7) return;
    const x2 = x + Math.cos(angle) * len;
    const y2 = y + Math.sin(angle) * len;

    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(x2, y2);
    ctx.stroke();

    const spread = .47 + phase * .035;
    const next = len * .61;
    branch(x2, y2, next, angle - spread, depth - 1, phase + .2);
    branch(x2, y2, next, angle + spread, depth - 1, phase + .1);
  }

  function draw() {
    ctx.clearRect(0, 0, width, height);
    const px = mouseX * 10;
    const py = mouseY * 8;

    ctx.save();
    ctx.lineWidth = .65;
    ctx.strokeStyle = 'rgba(145, 199, 220, 0.055)';
    ctx.globalCompositeOperation = 'screen';

    motifs.forEach((m, i) => {
      const direction = i % 2 === 0 ? 1 : -1;
      const x = width * m.x + px * direction;
      const y = height * m.y + py * -direction;
      const angle = m.angle + mouseX * .055 * direction;
      branch(x, y, m.size, angle, m.depth, i * .17);
    });

    ctx.restore();
  }

  function animate() {
    mouseX += (targetX - mouseX) * .055;
    mouseY += (targetY - mouseY) * .055;
    draw();
    if (Math.abs(targetX - mouseX) > .001 || Math.abs(targetY - mouseY) > .001) {
      raf = requestAnimationFrame(animate);
    } else {
      raf = 0;
    }
  }

  if (!reduceMotion) {
    window.addEventListener('pointermove', (event) => {
      targetX = event.clientX / Math.max(width, 1) - .5;
      targetY = event.clientY / Math.max(height, 1) - .5;
      if (!raf) raf = requestAnimationFrame(animate);
    }, { passive: true });
  }

  window.addEventListener('resize', resize, { passive: true });
  resize();
})();
