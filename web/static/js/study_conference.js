(() => {
  'use strict';

  const mount = document.querySelector('#jitsiMeet');
  if (!mount) return;

  const loading = mount.querySelector('.conference-loading');
  const status = document.querySelector('[data-recording-state]');
  const live = document.querySelector('[data-live-recording]');
  const startButton = document.querySelector('[data-record-start]');
  const stopButton = document.querySelector('[data-record-stop]');
  const moderator = mount.dataset.moderator === '1';

  let joined = false;
  let recording = false;
  let recordingPending = false;

  const revealConference = () => {
    mount.classList.add('is-connected');
    if (loading && loading.isConnected) {
      loading.remove();
    }
  };

  const setRecordingUI = (message) => {
    if (live) live.hidden = !recording;
    if (status) {
      status.textContent = message || (recording ? 'Идёт серверная запись' : 'Запись выключена');
      status.classList.toggle('is-recording', recording);
      status.classList.toggle('is-pending', recordingPending);
    }

    if (startButton) {
      startButton.disabled = !moderator || !joined || recording || recordingPending;
    }
    if (stopButton) {
      stopButton.disabled = !moderator || !joined || !recording || recordingPending;
    }
  };

  if (typeof window.JitsiMeetExternalAPI !== 'function') {
    mount.innerHTML = '<div class="conference-loading error"><b>Не удалось загрузить Jitsi Meet.</b><span>Проверьте JITSI_PUBLIC_URL и доступность сервера конференций.</span></div>';
    return;
  }

  const api = new window.JitsiMeetExternalAPI(mount.dataset.domain, {
    roomName: mount.dataset.room,
    parentNode: mount,
    width: '100%',
    height: '100%',
    jwt: mount.dataset.jwt,
    lang: 'ru',
    userInfo: {
      displayName: mount.dataset.userName || '',
      email: mount.dataset.userEmail || ''
    },
    configOverwrite: {
      prejoinConfig: { enabled: false },
      disableDeepLinking: true,
      requireDisplayName: false,
      enableWelcomePage: false
    },
    interfaceConfigOverwrite: {
      MOBILE_APP_PROMO: false,
      SHOW_JITSI_WATERMARK: false,
      SHOW_WATERMARK_FOR_GUESTS: false
    }
  });

  // ExternalAPI inserts a cross-origin iframe. The old fullscreen template kept
  // the MY.STUDY loading layer above that iframe forever. Reveal Jitsi as soon
  // as its iframe has loaded; videoConferenceJoined below is a second guarantee.
  const attachIframeLoad = () => {
    const frame = mount.querySelector('iframe');
    if (!frame) return false;
    frame.addEventListener('load', revealConference, { once: true });
    return true;
  };

  if (!attachIframeLoad()) {
    const observer = new MutationObserver(() => {
      if (attachIframeLoad()) observer.disconnect();
    });
    observer.observe(mount, { childList: true });
    window.setTimeout(() => observer.disconnect(), 15000);
  }

  // Safety fallback: never leave a working conference hidden behind our loader.
  window.setTimeout(() => {
    if (mount.querySelector('iframe')) revealConference();
  }, 2500);

  api.addListener('videoConferenceJoined', () => {
    joined = true;
    revealConference();
    setRecordingUI();
  });

  api.addListener('videoConferenceLeft', () => {
    joined = false;
    recording = false;
    recordingPending = false;
    setRecordingUI('Конференция завершена');
  });

  api.addListener('recordingStatusChanged', (event) => {
    if (!event || event.mode !== 'file') return;

    recordingPending = false;

    if (event.error) {
      recording = false;
      setRecordingUI(`Ошибка записи: ${event.error}`);
      return;
    }

    recording = Boolean(event.on);
    setRecordingUI(recording ? 'Идёт серверная запись' : 'Запись остановлена');
  });

  if (moderator && startButton) {
    startButton.addEventListener('click', () => {
      if (!joined || recording || recordingPending) return;

      recordingPending = true;
      setRecordingUI('Запускаю Jibri…');

      api.executeCommand('startRecording', {
        mode: 'file',
        shouldShare: false,
        extraMetadata: {
          booking_id: mount.dataset.bookingId
        }
      });
    });
  }

  if (moderator && stopButton) {
    stopButton.addEventListener('click', () => {
      if (!joined || !recording || recordingPending) return;

      recordingPending = true;
      setRecordingUI('Останавливаю запись…');
      api.executeCommand('stopRecording', 'file', false);
    });
  }

  setRecordingUI();

  window.addEventListener('beforeunload', () => {
    try { api.dispose(); } catch (_) { /* nothing */ }
  });
})();
