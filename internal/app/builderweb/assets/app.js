'use strict';
(() => {
  const TEMPLATES = [
    {id:'minimal',name:'Study Minimal',description:'Чистый и лаконичный',accent:'#7768d8',theme:'light'},
    {id:'cozy',name:'Study Cozy',description:'Теплый и спокойный',accent:'#ba8669',theme:'light'},
    {id:'modern',name:'Study Modern',description:'Контрастный интерфейс',accent:'#737bdc',theme:'dark'},
    {id:'personal',name:'Tutor Portfolio',description:'Личный сайт педагога',accent:'#598f81',theme:'light'},
    {id:'custom',name:'У меня своя идея',description:'Опишите проект без готового шаблона',accent:'#7768d8',theme:'light'}
  ];
  const FEATURES = [
 ['auth','Авторизация и регистрация','Вход и учетные записи учеников'],
 ['teacher-panel','Кабинет преподавателя','Рабочее пространство преподавателя'],
 ['student-profile','Кабинет ученика','Личная учебная страница'],
 ['schedule','Индивидуальное расписание','Календарь персональных занятий'],
 ['recurring','Повторяющиеся занятия','Регулярные уроки'],
 ['reschedule','Перенос и отмена занятий','Управление изменениями'],
 ['homework','Домашние задания','Задания и ответы'],
 ['materials','Прикрепление файлов','Материалы к урокам'],
 ['comments','Комментарии к заданиям','Обсуждение работы'],
 ['completion','Отметки выполнения','Готовность домашних заданий'],
 ['reminders','Уведомления','События внутри сайта'],
 ['telegram','Интеграция с Telegram/Max',''],
 ['payments','Учет оплат','Баланс и история платежей (можно сделать автооплату, если есть статус самозанятый или ИП))'],
 ['progress','Прогресс ученика','Итоги занятий'],
 ['admin','Административная панель','Управление платформой'],
 ['whiteboard','Доска для занятий','Рисование и сохранение доски в карточке урока']
 ];
  function generatorPanel(){
    const colorField=(key,label)=>`<label class="custom-color-field"><span>${label}</span><span class="custom-color-control"><input type="color" data-custom-color="${key}" value="${state[key]}" aria-label="${label}"><code data-color-label="${key}">${state[key].toUpperCase()}</code></span></label>`;
    return `<section class="preset-generator" aria-label="Свой вариант"><div class="generator-head"><h2>Свой вариант</h2><button type="button" class="btn btn-ghost" data-presets="1">← К пресетам</button></div><div class="generator-controls"><div><span class="generator-label">Тема предпросмотра</span><div class="generator-themes">${[['light','Светлая'],['dark','Темная']].map(([id,n])=>`<button type="button" class="choice ${state.theme===id?'selected':''}" data-theme="${id}" aria-pressed="${state.theme===id}">${n}</button>`).join('')}</div></div>${colorField('primary_light','Основной цвет · светлая тема')}${colorField('primary_dark','Основной цвет · темная тема')}${colorField('accent','Акцентный цвет')}</div></section>`;
  }
  const state = {customEditor:false,primary_light:'#ffffff',primary_dark:'#1e2636',id:crypto.randomUUID(),font:'sans',home:'overview',previewPage:'overview',step:0,preset:'minimal',theme:'light',accent:'#7768d8',layout:'sidebar',style:'soft',features:['schedule','homework','student-profile','teacher-panel'],site_name:'',description:'',extra:'',name:'',contact:'',mobile:false,loading:false,submitted:false};
  const $ = (s) => document.querySelector(s);
  const esc = (s='') => String(s).replace(/[&<>"']/g, c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const field = (name,label,placeholder,rows=0,required=false,hint='') => `<div class="field-wrap"><label for="input-${name}">${label}${required?' <span class="required">*</span>':''}</label>${rows?`<textarea id="input-${name}" data-field="${name}" rows="${rows}" maxlength="${name==='description'?4000:2000}" placeholder="${esc(placeholder)}">${esc(state[name])}</textarea>`:`<input id="input-${name}" data-field="${name}" maxlength="${name==='contact'?160:100}" placeholder="${esc(placeholder)}" value="${esc(state[name])}">`}${hint?`<span class="field-hint">${hint}</span>`:''}</div>`;
  const headers = [
    ['01 / ОСНОВА','Начнем с идеи.','Выберите внешний вид будущей платформы.'],
    ['02 / ОФОРМЛЕНИЕ','Настройте стиль.','Подберите цвет, тему и структуру навигации. Результат сразу появится справа.'],
    ['03 / ФУНКЦИИ','Что должен уметь сайт?','Выберите только необходимые возможности. Необычные идеи можно описать на следующем шаге.'],
    ['04 / ВАША ИДЕЯ','Расскажите о проекте.','Опишите, кому нужен сайт, как проходят занятия и что важно реализовать.'],
    ['05 / ПОДТВЕРЖДЕНИЕ','Все готово.','Проверьте выбранные настройки и отправьте техническое задание на рассмотрение.']
  ];
  const preset = () => TEMPLATES.find(x=>x.id===state.preset);
  const featureName = id => FEATURES.find(f=>f[0]===id)?.[1]||id;
  const capital = s => s[0].toUpperCase()+s.slice(1);
  function render(){
    const [label,title,subtitle]=headers[state.step];
    $('#section-label').textContent=label;$('#section-title').textContent=title;$('#section-subtitle').textContent=subtitle;
    $('#step-current').textContent=String(state.step+1).padStart(2,'0');$('#progress-fill').style.width=((state.step+1)*20)+'%';
    document.querySelectorAll('.step').forEach((b,i)=>{b.classList.toggle('active',i===state.step);b.classList.toggle('done',i<state.step);b.setAttribute('aria-current',i===state.step?'step':'false');});
    const root=$('#step-content');
    const optionGroup=(key,label,options)=>`<div class="form-field"><div class="field-heading">${label}</div><div class="toggle-grid">${options.map(([v,n])=>`<button type="button" class="choice ${state[key]===v?'selected':''}" data-${key}="${v}">${n}</button>`).join('')}</div></div>`;
    if(state.step===0) root.innerHTML=state.preset==='custom'&&state.customEditor?generatorPanel():`<div class="template-grid">${TEMPLATES.map(t=>`<button type="button" data-preset="${t.id}" class="template-option ${t.id} ${t.id===state.preset?'selected':''}" aria-pressed="${t.id===state.preset}"><div class="thumb" style="--thumb-accent:${t.id===state.preset?state.accent:t.accent}"><div class="thumb-sidebar"><i></i><i></i><i></i><i></i></div><div class="thumb-content"><div class="thumb-top"></div><div class="thumb-hero"></div><div class="thumb-cards"><i></i><i></i></div></div></div><div class="template-meta"><span class="template-name">${t.name}</span><span class="template-tick">✓</span></div><div class="template-desc">${t.description}</div></button>`).join('')}</div>`;
    else if(state.step===1) root.innerHTML=`<div class="design-fields">${optionGroup('font','Шрифт',[['sans','Без засечек'],['serif','С засечками'],['mono','Моноширинный']])}${optionGroup('home','Главная страница',[['overview','Обзор'],['schedule','Расписание'],['profile','О преподавателе']])}<div class="form-field"><div class="field-heading">Акцентный цвет <span class="field-helper">· Можно выбрать свой</span></div><div class="swatches">${['#7768d8','#ba8669','#598f81','#e18eaa','#4e93c4','#353f5c'].map(c=>`<button type="button" class="swatch ${state.accent.toLowerCase()===c?'selected':''}" data-accent="${c}" style="background:${c}" aria-label="Цвет ${c}" aria-pressed="${state.accent.toLowerCase()===c}"></button>`).join('')}<input class="swatch-custom" id="custom-color" type="color" aria-label="Свой цвет" value="${state.accent}"></div></div><div class="form-field"><div class="field-heading">Тема сайта</div><div class="toggle-grid">${[['light','Светлая','☼'],['dark','Темная','☾']].map(t=>`<button type="button" class="choice ${state.theme===t[0]?'selected':''}" data-theme="${t[0]}"><span class="symbol">${t[2]}</span>${t[1]}</button>`).join('')}</div></div><div class="form-field"><div class="field-heading">Расположение навигации</div><div class="toggle-grid">${[['sidebar','Боковое меню','▤'],['topbar','Верхнее меню','▥']].map(t=>`<button type="button" class="choice ${state.layout===t[0]?'selected':''}" data-layout="${t[0]}"><span class="symbol">${t[2]}</span>${t[1]}</button>`).join('')}</div></div><div class="form-field"><div class="field-heading">Характер интерфейса</div><div class="toggle-grid">${[['soft','Мягкие формы','◯'],['sharp','Строгие формы','□']].map(t=>`<button type="button" class="choice ${state.style===t[0]?'selected':''}" data-style="${t[0]}"><span class="symbol">${t[2]}</span>${t[1]}</button>`).join('')}</div></div></div>`;
    else if(state.step===2) root.innerHTML=`<div class="feature-grid">${FEATURES.map(f=>`<label class="feature"><input type="checkbox" data-feature="${f[0]}" ${state.features.includes(f[0])?'checked':''}><span><span class="feature-name">${f[1]}</span><span class="feature-desc">${f[2]}</span></span></label>`).join('')}</div>`;
    else if(state.step===3) root.innerHTML=`<div class="text-form">${field('site_name','Название проекта','Например, My English Study',0,false)}${field('description','Описание будущего проекта','Я провожу индивидуальные занятия по математике. Хочу, чтобы мои ученики могли...',6,true,'Опишите вашу идею свободным текстом, минимум 15 символов.')}${field('extra','Дополнительные пожелания','Нестандартные функции, примеры понравившихся сайтов, особые пожелания...',3)}${field('name','Как к вам обращаться','Имя',0,true)}${field('contact','Контакт для ответа','Email или Telegram @username',0,true)}</div>`;
    else if(state.step===4) root.innerHTML=state.submitted?`<div class="completion"><div class="completion-icon">✓</div><h2>Заявка отправлена</h2><p>Мы сохранили описание вашего проекта и выбранные настройки. Ответ поступит по указанному вами контакту.</p></div>`:`<div class="summary-card"><h3>${esc(state.site_name||'Персональный учебный сайт')}</h3>${[
      ['Шаблон',preset().name],['Тема',state.theme==='dark'?'Темная':'Светлая'],['Акцент',state.accent.toUpperCase()],...(state.preset==='custom'?[['Основной · светлая',state.primary_light.toUpperCase()],['Основной · темная',state.primary_dark.toUpperCase()]]:[]),['Навигация',state.layout==='sidebar'?'Боковая':'Верхняя'],['Стиль',state.style==='soft'?'Мягкий':'Строгий'],['Шрифт',state.font],['Главная',state.home],['Функции',state.features.length?state.features.map(featureName).join(', '):'Без дополнительных функций'],['Ваше имя',state.name],['Контакт',state.contact]
    ].map(([l,v])=>`<div class="summary-row"><span>${l}</span><strong>${esc(v)}</strong></div>`).join('')}<div class="summary-description"><b>Ваше описание</b><br>${esc(state.description)}${state.extra?`<br><br><b>Пожелания</b><br>${esc(state.extra)}`:''}</div></div><p class="privacy-note">Нажимая «Отправить заявку», вы передаете указанные данные для рассмотрения обращения.</p>`;
    $('#back-btn').hidden=state.step===0||state.submitted;
    $('#next-btn').hidden=state.submitted;
    $('#next-btn').innerHTML=state.step===4?'Отправить заявку <span aria-hidden="true">→</span>':'Продолжить <span aria-hidden="true">→</span>';
    $('#next-btn').disabled=state.loading;
    updatePreview();
  }
  function updatePreview(){
    const stage=$('#preview-stage'),pv=$('#website-preview'),dark=state.theme==='dark';
    stage.classList.toggle('mobile',state.mobile);
    $('#view-desktop').classList.toggle('selected',!state.mobile);$('#view-mobile').classList.toggle('selected',state.mobile);
    const colors={bg:dark?'#1e2636':'#ffffff',ink:dark?'#f1f2f7':'#293044',surface:dark?'#293349':'#f5f6fa',muted:dark?'#a4b0c7':'#8c96a6'};
    if(state.preset==='custom'){
      colors.bg=dark?state.primary_dark:state.primary_light;
      const rgb=colors.bg.slice(1).match(/../g).map(x=>parseInt(x,16));
      const bright=(rgb[0]*299+rgb[1]*587+rgb[2]*114)/1000>150;
      colors.ink=bright?'#182230':'#f3f7fa';colors.muted=bright?'#4b5968':'#c0d1dd';
      colors.surface='#'+rgb.map(v=>Math.round(bright?v*.94:v+(255-v)*.09).toString(16).padStart(2,'0')).join('');
    }
    for(const [k,v] of Object.entries(colors))pv.style.setProperty('--pv-'+k,v);
    pv.style.setProperty('--pv-accent',state.accent);pv.style.setProperty('--pv-radius',state.style==='soft'?'10px':'3px');
    pv.className='website-preview '+(state.layout==='topbar'?'pv-topbar':'');
    pv.classList.add('preset-'+state.preset);
    pv.style.fontFamily=({sans:'system-ui, sans-serif',serif:'Georgia, serif',mono:'monospace'})[state.font];
    const title=esc(state.site_name.trim()||preset().name);
    const has=id=>state.features.includes(id);
    const page=state.previewPage==='overview'?state.home:state.previewPage;
    const sections=[['overview','Главная'],['schedule','Расписание'],['account','Кабинет']];
    const cards=state.features.map(id=>`<div class="pv-card"><b>${esc(featureName(id))}</b><small>${id==='whiteboard'?'Доска сохранена · Урок 12':id==='payments'?'Оплачено 4 занятия':id==='progress'?'Завершено 8 из 12 шагов':''}</small></div>`).join('');
    const schedule=has('schedule')?`<h2>Мое расписание</h2><div class="pv-week">${['Пн','Вт','Ср','Чт','Пт'].map((d,i)=>`<div><b>${d}</b><div class="pv-card">${i===0||i===2?'16:00<br>Индивидуальный урок':'Свободно'}</div></div>`).join('')}</div>${has('recurring')?'<p>Каждую неделю · Пн / Ср</p>':''}${has('reschedule')?'<span class="pv-action">Перенести · Отменить</span>':''}`:'<p>Расписание не выбрано. Добавьте его на шаге «Возможности».</p>';
    const portfolio=`<div class="pv-portrait">А</div><h2>Username ваш преподаватель</h2><p>Оприсание.</p><div class="pv-banner"><b>Знакомство и первый урок</b><span class="pv-action">Записаться →</span></div>${has('student-profile')?'<p>Вход в кабинет ученика →</p>':''}`;
    let content=page==='schedule'?schedule:page==='profile'?portfolio:page==='account'?`<h2>Кабинет ученика</h2><p>username · индивидуальный план</p><div class="pv-card-row">${cards||'<p>Функции пока не выбраны</p>'}</div>`:state.preset==='personal'?portfolio:`<h2 class="pv-welcome">${state.preset==='cozy'?'Рады новой встрече ☀':'Привет, username!'}</h2>${has('schedule')?'<div class="pv-banner"><b>Ближайший урок · Среда, 16:00</b></div>':''}<div class="pv-card-row">${cards||'<p>Добавьте функции на третьем шаге</p>'}</div>`;

    pv.innerHTML=`<aside class="pv-sidebar"><div class="pv-logo">${title}<i>.</i></div><nav class="pv-nav">${sections.map(([id,n])=>`<button type="button" data-preview="${id}" class="pv-nav-item ${state.previewPage===id?'current':''}">${n}</button>`).join('')}</nav></aside><div class="pv-main"><div class="pv-header"><small>Личный кабинет</small><div class="pv-avatar">u</div></div>${content}</div>`;
    $('#preview-preset').textContent=preset().name.toUpperCase();$('#preview-theme').textContent=dark?'ТЕМНАЯ':'СВЕТЛАЯ';
  }
  function updateStatus(msg,type=''){const e=$('#submit-status');e.textContent=msg;e.className='submit-status '+type}
  function validateForm(){
    if(state.description.trim().length<15){updateStatus('Добавьте описание проекта (не менее 15 символов).','error');$('#input-description')?.focus();return false}
    if(state.name.trim().length<2){updateStatus('Укажите ваше имя.','error');$('#input-name')?.focus();return false}
    if(state.contact.trim().length<3){updateStatus('Укажите контакт для ответа.','error');$('#input-contact')?.focus();return false}
    return true;
  }
  async function submit(){
    if(state.loading)return;
    state.loading=true;$('#next-btn').disabled=true;updateStatus('Отправляем заявку...');
    const {id,primary_light,primary_dark,font,home,preset,theme,accent,layout,style,features,site_name,description,extra,name,contact}=state;
    try{
      const apiBase=location.pathname.replace(/\/$/,'');
      const res=await fetch(`${apiBase}/api/requests`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id,primary_light,primary_dark,font,home,preset,theme,accent,layout,style,features,site_name,description,extra,name,contact,company:''})});
      const data=await res.json();
      if(!res.ok)throw new Error(data.error||`HTTP ${res.status}`);
      state.submitted=true;updateStatus('');render();
    }catch(err){updateStatus(`Не удалось отправить заявку: ${err.message}`,'error')}
    finally{state.loading=false;$('#next-btn').disabled=false}
  }
  $('#step-content').addEventListener('click',event=>{
    const el=event.target.closest('[data-preset],[data-accent],[data-theme],[data-layout],[data-style],[data-font],[data-home],[data-presets]');
    if(!el)return;
    if(el.dataset.presets){state.customEditor=false;render();return;}
    if(el.dataset.preset==='custom'){state.preset='custom';state.customEditor=true;state.previewPage='overview';render();return;}
    if(el.dataset.preset){state.customEditor=false;const t=TEMPLATES.find(t=>t.id===el.dataset.preset);Object.assign(state,{preset:t.id,accent:t.accent,theme:t.theme,layout:t.id==='modern'?'sidebar':'topbar',style:t.id==='minimal'?'sharp':'soft',home:t.id==='personal'?'profile':'overview',previewPage:'overview'});}
    for(const k of ['accent','theme','layout','style','font','home'])if(el.dataset[k])state[k]=el.dataset[k];
    render();
  });
  $('#step-content').addEventListener('input',event=>{
    if(event.target.dataset.field)state[event.target.dataset.field]=event.target.value;
    if(event.target.dataset.customColor){const key=event.target.dataset.customColor;if(['primary_light','primary_dark','accent'].includes(key)){state[key]=event.target.value;document.querySelectorAll('[data-color-label]').forEach(e=>{if(e.dataset.colorLabel===key)e.textContent=state[key].toUpperCase()});updatePreview();}}
    if(event.target.id==='custom-color'){state.accent=event.target.value;updatePreview();document.querySelectorAll('[data-accent]').forEach(b=>b.classList.toggle('selected',b.dataset.accent===state.accent));}
    if(event.target.dataset.field==='site_name')updatePreview();
    updateStatus('');
  });
  $('#step-content').addEventListener('change',event=>{
    const f=event.target.dataset.feature;if(!f)return;
    if(event.target.checked){if(!state.features.includes(f))state.features.push(f)}else state.features=state.features.filter(x=>x!==f);
    updatePreview();
  });
  $('#back-btn').addEventListener('click',()=>{if(state.step>0){state.step--;updateStatus('');render();window.scrollTo({top:0,behavior:'smooth'})}});
  $('#next-btn').addEventListener('click',()=>{
    if(state.step===4){submit();return}
    if(state.step===3&&!validateForm())return;
    state.step++;updateStatus('');render();window.scrollTo({top:0,behavior:'smooth'});
  });
  document.querySelectorAll('[data-jump]').forEach(b=>b.addEventListener('click',()=>{
    const step=Number(b.dataset.jump);
    if(state.submitted||step>state.step)return;
    state.step=step;updateStatus('');render();
  }));
  $('#view-desktop').addEventListener('click',()=>{state.mobile=false;updatePreview()});
  $('#view-mobile').addEventListener('click',()=>{state.mobile=true;updatePreview()});
  $('#website-preview').addEventListener('click',e=>{const b=e.target.closest('[data-preview]');if(b){state.previewPage=b.dataset.preview;updatePreview()}});
  render();
})();
