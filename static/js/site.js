const reduceMotion = window.matchMedia(
  '(prefers-reduced-motion: reduce)'
).matches;

/* =========================
   REVEAL АНИМАЦИИ
========================= */

const observer = new IntersectionObserver(
  (entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add('visible');
        observer.unobserve(entry.target);
      }
    });
  },
  {
    threshold: 0.1,
    rootMargin: '0px 0px -24px 0px'
  }
);

document.querySelectorAll('.reveal').forEach((el, index) => {
  el.style.setProperty(
    '--reveal-delay',
    `${Math.min(index * 45, 180)}ms`
  );

  observer.observe(el);
});


/* =========================
   СОЗВЕЗДИЕ
========================= */

const canvas =
  document.getElementById('constellationCanvas');

const card =
  document.getElementById('constellationCard');


if (canvas && card) {

  const ctx = canvas.getContext('2d');

  let dpr = 1;

  let width = 0;
  let height = 0;

  let points = [];
  let stars = [];
  let links = [];

  let pointerX = 0;
  let pointerY = 0;

  let currentX = 0;
  let currentY = 0;

  let frame = 0;

  let seed =
    Math.floor(Math.random() * 0xffffffff) >>> 0;


  const rnd = () =>
    (
      (
        seed =
          (
            seed * 1664525 +
            1013904223
          ) >>> 0
      ) /
      4294967296
    );


  /* -------------------------
     Создание сцены
  ------------------------- */

  function makeScene() {

    seed =
      (
        Date.now() ^
        Math.floor(
          Math.random() * 0xffffffff
        )
      ) >>> 0;


    /*
       Маленькие фоновые звёзды
    */

    stars = Array.from(
      {
        length: Math.max(
          80,
          Math.floor(
            (width * height) / 5500
          )
        )
      },

      () => ({
        x: rnd() * width,
        y: rnd() * height,

        r:
          0.3 +
          rnd() * 0.9,

        a:
          0.10 +
          rnd() * 0.34,

        drift:
          (rnd() - 0.5) * 0.8
      })
    );


    /*
       Главные точки созвездия
    */

    const count =
      11 +
      Math.floor(rnd() * 4);


    points = Array.from(
      { length: count },

      (_, i) => ({
        /*
           Оставляем небольшие поля,
           но используем почти всю карточку
        */

        x:
          width *
          (
            0.04 +
            rnd() * 0.92
          ),

        y:
          height *
          (
            0.05 +
            rnd() * 0.90
          ),

        r:
          1.2 +
          rnd() * 1.9,

        glow:
          0.22 +
          rnd() * 0.38,

        phase:
          rnd() *
          Math.PI *
          2,

        idx: i
      })
    );


    /*
       Сортируем, чтобы линии
       выглядели более естественно
    */

    points.sort(
      (a, b) =>
        a.x - b.x
    );


    links = [];


    /*
       Основная цепь
    */

    for (
      let i = 0;
      i < points.length - 1;
      i++
    ) {

      links.push([
        i,
        i + 1
      ]);


      /*
         Иногда дополнительная связь
      */

      if (
        i < points.length - 2 &&
        rnd() > 0.72
      ) {

        links.push([
          i,
          i + 2
        ]);
      }
    }


    /*
       Несколько дополнительных
       аккуратных соединений
    */

    if (points.length > 9) {

      links.push([1, 5]);
      links.push([4, 8]);

      if (rnd() > 0.5) {
        links.push([
          6,
          points.length - 1
        ]);
      }
    }
  }


  /* -------------------------
     Resize
  ------------------------- */

  function resize() {

    const rect =
      card.getBoundingClientRect();

    dpr =
      Math.min(
        window.devicePixelRatio || 1,
        2
      );

    width = rect.width;
    height = rect.height;


    canvas.width =
      Math.round(
        width * dpr
      );

    canvas.height =
      Math.round(
        height * dpr
      );


    ctx.setTransform(
      dpr,
      0,
      0,
      dpr,
      0,
      0
    );


    makeScene();

    draw(0);
  }


  /* -------------------------
     Drawing
  ------------------------- */

  function draw(time) {

    ctx.clearRect(
      0,
      0,
      width,
      height
    );


    currentX +=
      (pointerX - currentX) *
      0.04;

    currentY +=
      (pointerY - currentY) *
      0.04;


    /*
       Параллакс
    */

    const ox =
      currentX * 12;

    const oy =
      currentY * 10;


    ctx.save();


    /*
       Фоновые звёзды
    */

    stars.forEach((s) => {

      ctx.beginPath();

      ctx.arc(
        s.x +
          ox * s.drift,

        s.y +
          oy * s.drift,

        s.r,

        0,

        Math.PI * 2
      );


      ctx.fillStyle =
        `rgba(225,238,248,${s.a})`;

      ctx.fill();
    });


    /*
       Линии созвездия
    */

    ctx.lineWidth = 0.8;

    ctx.strokeStyle =
      'rgba(145,190,211,0.30)';


    links.forEach(
      ([a, b]) => {

        const p1 =
          points[a];

        const p2 =
          points[b];


        ctx.beginPath();

        ctx.moveTo(
          p1.x + ox,
          p1.y + oy
        );

        ctx.lineTo(
          p2.x + ox,
          p2.y + oy
        );

        ctx.stroke();
      }
    );


    /*
       Главные звёзды
    */

    points.forEach((p) => {

      const pulse =
        reduceMotion
          ? 1
          : 1 +
            Math.sin(
              time * 0.0014 +
              p.phase
            ) *
            0.11;


      /*
         Небольшой halo
      */

      ctx.beginPath();

      ctx.arc(
        p.x + ox,
        p.y + oy,

        p.r * 3.2,

        0,

        Math.PI * 2
      );


      ctx.fillStyle =
        `rgba(110,185,215,${
          p.glow * 0.09
        })`;

      ctx.fill();


      /*
         Звезда
      */

      ctx.beginPath();

      ctx.arc(
        p.x + ox,
        p.y + oy,

        p.r * pulse,

        0,

        Math.PI * 2
      );


      ctx.fillStyle =
        'rgba(222,239,247,0.94)';


      ctx.shadowBlur = 10;

      ctx.shadowColor =
        `rgba(145,199,220,${p.glow})`;


      ctx.fill();


      ctx.shadowBlur = 0;
    });


    ctx.restore();


    if (!reduceMotion) {

      frame =
        requestAnimationFrame(
          draw
        );
    }
  }


  /* -------------------------
     Mouse parallax
  ------------------------- */

  card.addEventListener(
    'pointermove',

    (event) => {

      if (reduceMotion) return;


      const rect =
        card.getBoundingClientRect();


      pointerX =
        (
          event.clientX -
          rect.left
        ) /
          rect.width -
        0.5;


      pointerY =
        (
          event.clientY -
          rect.top
        ) /
          rect.height -
        0.5;
    },

    { passive: true }
  );


  card.addEventListener(
    'pointerleave',

    () => {

      pointerX = 0;
      pointerY = 0;
    },

    { passive: true }
  );


  resize();


  if (!reduceMotion) {

    frame =
      requestAnimationFrame(
        draw
      );
  }


  window.addEventListener(
    'resize',
    resize,
    { passive: true }
  );


  window.addEventListener(
    'pagehide',

    () =>
      cancelAnimationFrame(
        frame
      ),

    { once: true }
  );
}