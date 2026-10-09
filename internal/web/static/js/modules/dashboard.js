// Dashboard Module: Server Lifecycle, In-game Chat, Players, Modal Control & World Environment

// Process Control
async function controlProcess(action) {
  try {
    const isAdmin = currentUser && currentUser.role === 'admin';
    const endpoint = (isAdmin || action !== 'start') ? `/api/admin/process/${action}` : '/api/process/start';
    const res = await fetch(endpoint, { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || `서버 ${action} 실패`, 'error');
      return;
    }
    showToast(data.message || (isAdmin ? `서버 ${action} 완료` : '서버 시작 요청이 완료되었습니다.'));
    fetchProcessStatus();
  } catch (err) {
    showToast('서버 제어 요청 실패', 'error');
  }
}

// Process Status Polling
async function fetchProcessStatus() {
  try {
    let res = await fetch('/api/process/status');
    if (!res.ok) {
      res = await fetch('/api/admin/process/status');
    }
    if (!res.ok) return;

    const data = await res.json();
    const st = data.status;

    const procBadge = document.getElementById('procBadge');
    const soulOrb = document.getElementById('soulOrb');
    const procMeta = document.getElementById('procMeta');
    const btnStart = document.getElementById('btnStart');
    const btnStop = document.getElementById('btnStop');
    const btnRestart = document.getElementById('btnRestart');
    const jarStatusBadge = document.getElementById('jarStatusBadge');

    const dashboardOfflineCard = document.getElementById('dashboardOfflineCard');
    const mapOfflineOverlay = document.getElementById('mapOfflineOverlay');
    const btnDashboardStart = document.getElementById('btnDashboardStart');
    const btnMapStart = document.getElementById('btnMapStart');
    const dashboardOfflineTitle = document.getElementById('dashboardOfflineTitle');
    const mapOfflineTitle = document.getElementById('mapOfflineTitle');

    const isAdmin = currentUser && currentUser.role === 'admin';
    const startBtnText = isAdmin ? '서버 시작하기.' : '서버 시작 요청하기.';

    const isSleeping = (st === 'stopped' && data.auto_sleep_enabled);

    if (procBadge) {
      if (isSleeping) {
        procBadge.textContent = 'SLEEPING';
        procBadge.className = 'status-text sleeping';
      } else {
        procBadge.textContent = st.toUpperCase();
        procBadge.className = `status-text ${st}`;
      }
    }
    if (soulOrb) {
      soulOrb.className = `status-dot ${isSleeping ? 'sleeping' : st}`;
    }

    if (st === 'running') {
      const up = formatUptime(data.uptime_seconds);
      if (procMeta) procMeta.textContent = `Uptime: ${up}`;
      if (btnStart) btnStart.disabled = true;
      if (btnStop) btnStop.disabled = false;
      if (btnRestart) btnRestart.disabled = false;

      // Hide offline overlays
      if (dashboardOfflineCard) dashboardOfflineCard.style.display = 'none';
      if (mapOfflineOverlay) mapOfflineOverlay.style.display = 'none';

      // Load map only when server is running
      const bluemapFrame = document.getElementById('bluemapFrame');
      if (bluemapFrame && (!bluemapFrame.src || bluemapFrame.src === 'about:blank' || bluemapFrame.src.endsWith('/about:blank'))) {
        bluemapFrame.src = window.serverMapUrl || '/squaremap/';
      }
    } else if (st === 'starting') {
      if (procMeta) procMeta.textContent = '기동 중...';
      if (btnStart) btnStart.disabled = true;
      if (btnStop) btnStop.disabled = true;
      if (btnRestart) btnRestart.disabled = true;

      // Show starting status on overlays
      if (dashboardOfflineCard) dashboardOfflineCard.style.display = 'block';
      if (mapOfflineOverlay) mapOfflineOverlay.style.display = 'flex';
      if (dashboardOfflineTitle) dashboardOfflineTitle.textContent = '서버가 기동 중입니다...';
      if (mapOfflineTitle) mapOfflineTitle.textContent = '서버가 기동 중입니다...';
      if (btnDashboardStart) {
        btnDashboardStart.textContent = '기동 중...';
        btnDashboardStart.disabled = true;
      }
      if (btnMapStart) {
        btnMapStart.textContent = '기동 중...';
        btnMapStart.disabled = true;
      }
    } else if (st === 'stopping') {
      if (procMeta) procMeta.textContent = '종료 중...';
      if (btnStart) btnStart.disabled = true;
      if (btnStop) btnStop.disabled = true;
      if (btnRestart) btnRestart.disabled = true;

      if (dashboardOfflineCard) dashboardOfflineCard.style.display = 'block';
      if (mapOfflineOverlay) mapOfflineOverlay.style.display = 'flex';
      if (dashboardOfflineTitle) dashboardOfflineTitle.textContent = '서버가 종료 중입니다...';
      if (mapOfflineTitle) mapOfflineTitle.textContent = '지도에 서버가 종료되어 있습니다.';
      if (btnDashboardStart) {
        btnDashboardStart.textContent = startBtnText;
        btnDashboardStart.disabled = true;
      }
      if (btnMapStart) {
        btnMapStart.textContent = startBtnText;
        btnMapStart.disabled = true;
      }
    } else {
      // Stopped / Offline / Sleeping
      if (isSleeping) {
        if (procMeta) procMeta.textContent = '절전 모드 (접속 시 자동 기동)';
        if (dashboardOfflineTitle) dashboardOfflineTitle.textContent = '서버가 절전 모드(0MB)로 대기 중입니다.';
        if (mapOfflineTitle) mapOfflineTitle.textContent = '서버가 절전 모드입니다. 플레이어가 접속하면 자동으로 켜집니다.';
        const sleepBtnText = isAdmin ? '⚡ 서버 깨우기 (시작)' : '⚡ 서버 깨우기 요청';
        if (btnDashboardStart) btnDashboardStart.textContent = sleepBtnText;
        if (btnMapStart) btnMapStart.textContent = sleepBtnText;
      } else {
        if (procMeta) procMeta.textContent = '오프라인';
        if (dashboardOfflineTitle) dashboardOfflineTitle.textContent = '서버가 종료되어 있습니다.';
        if (mapOfflineTitle) mapOfflineTitle.textContent = '지도에 서버가 종료되어 있습니다.';
        if (btnDashboardStart) btnDashboardStart.textContent = startBtnText;
        if (btnMapStart) btnMapStart.textContent = startBtnText;
      }
      if (btnStart) btnStart.disabled = false;
      if (btnStop) btnStop.disabled = true;
      if (btnRestart) btnRestart.disabled = true;
      if (btnDashboardStart) btnDashboardStart.disabled = false;
      if (btnMapStart) btnMapStart.disabled = false;

      // Show offline overlays
      if (dashboardOfflineCard) dashboardOfflineCard.style.display = 'block';
      if (mapOfflineOverlay) mapOfflineOverlay.style.display = 'flex';

      // Reset map iframe to about:blank when offline to prevent unnecessary background polling
      const bluemapFrame = document.getElementById('bluemapFrame');
      if (bluemapFrame && bluemapFrame.src && !bluemapFrame.src.endsWith('about:blank')) {
        bluemapFrame.src = 'about:blank';
      }
    }

    if (jarStatusBadge) {
      jarStatusBadge.textContent = data.jar_exist ? '설치됨' : '미설치';
      jarStatusBadge.className = `badge ${data.jar_exist ? 'admin' : 'user'}`;
    }
  } catch (err) {}
}

// Fetch Players
async function fetchPlayers() {
  const onlineCountBadge = document.getElementById('onlineCountBadge');
  const playerList = document.getElementById('playerList');

  try {
    const res = await fetch('/api/players');
    if (!res.ok) {
      if (onlineCountBadge) onlineCountBadge.textContent = '오프라인';
      latestPlayers = [];
      return;
    }

    const data = await res.json();
    if (onlineCountBadge) onlineCountBadge.textContent = `${data.online} / ${data.max}`;
    latestPlayers = data.players || [];

    if (!playerList) return;
    playerList.innerHTML = '';
    if (!data.players || data.players.length === 0) {
      playerList.innerHTML = '<li style="text-align: center; padding: 32px 10px; color: var(--text-muted); font-size: 0.825rem;">접속 중인 플레이어가 없습니다.</li>';
      return;
    }

    const isAdmin = currentUser && currentUser.role === 'admin';
    data.players.forEach(name => {
      const li = document.createElement('li');
      li.className = `player-item ${isAdmin ? 'interactive' : ''}`;
      if (isAdmin) {
        li.title = `클릭하여 ${name} 제어 및 관리`;
        li.addEventListener('click', () => openPlayerModal(name));
      }
      li.innerHTML = `
        <img class="player-avatar" src="https://minotar.net/avatar/${encodeURIComponent(name)}/24" alt="${escapeHtml(name)}">
        <span class="player-name">${escapeHtml(name)}</span>
        ${isAdmin ? '<span style="font-size: 0.725rem; color: var(--accent); margin-left: auto;">관리 ⚙</span>' : ''}
      `;
      playerList.appendChild(li);
    });
  } catch (err) {
    if (onlineCountBadge) onlineCountBadge.textContent = '조회 실패';
    latestPlayers = [];
  }
}

// Player Management Modal
function openPlayerModal(name) {
  selectedPlayerForModal = name;
  const modal = document.getElementById('playerActionModal');
  const modalName = document.getElementById('modalPlayerName');
  if (!modal || !modalName) return;

  modalName.textContent = name;
  const reasonInput = document.getElementById('playerPunishReason');
  if (reasonInput) reasonInput.value = '';
  const msgInput = document.getElementById('playerMsgInput');
  if (msgInput) msgInput.value = '';
  const itemInput = document.getElementById('playerGiveItem');
  if (itemInput) itemInput.value = '';

  modal.style.display = 'flex';
}

function closePlayerModal() {
  const modal = document.getElementById('playerActionModal');
  if (modal) modal.style.display = 'none';
  selectedPlayerForModal = null;
}

async function executePlayerAction(action, args = []) {
  if (!selectedPlayerForModal) {
    showToast('선택된 플레이어가 없습니다.', 'error');
    return;
  }

  const param1 = args[0] || '';
  const param2 = args[1] || '';

  try {
    const res = await fetch('/api/admin/players/action', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        player: selectedPlayerForModal,
        action: action,
        param1: param1,
        param2: param2
      })
    });

    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '명령 실행 실패', 'error');
      return;
    }

    showToast(`[${selectedPlayerForModal}] ${data.message || '작업이 완료되었습니다.'}`);
    if (data.response) {
      showToast(data.response);
    }

    if (action === 'kick' || action === 'ban') {
      closePlayerModal();
      fetchPlayers();
    }
  } catch (err) {
    showToast('서버 통신 오류', 'error');
  }
}

// Real-time Chat Sync (In-game & Web)
async function fetchChatMessages() {
  try {
    const res = await fetch(`/api/chat?since=${lastChatID}`);
    if (!res.ok) return;

    const data = await res.json();
    const messages = data.messages || [];
    if (messages.length === 0) return;

    const chatHistory = document.getElementById('chatHistory');
    messages.forEach(msg => {
      if (msg.id > lastChatID) {
        lastChatID = msg.id;
      }
      appendChatMessage(msg);
    });

    if (chatHistory) chatHistory.scrollTop = chatHistory.scrollHeight;
  } catch (err) {}
}

function appendChatMessage(msg) {
  const chatHistory = document.getElementById('chatHistory');
  if (!chatHistory) return;

  const div = document.createElement('div');
  div.className = 'chat-line';

  if (msg.is_system) {
    div.innerHTML = `<span class="chat-system">[시스템] ${escapeHtml(msg.text)}</span>`;
  } else {
    const srcTag = msg.source === 'game' ? '<span class="badge" style="font-size: 0.65rem; margin-right: 4px;">인게임</span>' :
                   msg.source === 'web' ? '<span class="badge" style="font-size: 0.65rem; margin-right: 4px; background: rgba(59, 130, 246, 0.15); color: #93c5fd;">웹</span>' : '';
    div.innerHTML = `
      ${srcTag}
      <span class="chat-author">&lt;${escapeHtml(msg.sender)}&gt;</span>
      <span>${escapeHtml(msg.text)}</span>
    `;
  }

  chatHistory.appendChild(div);
}

// World Environment Functions (Weather & Time)
function updateWorldEnvironmentUI(env) {
  if (!env) return;

  const ticks = env.world_time_ticks ?? env.ticks ?? currentServerTicks;
  currentServerTicks = ticks;
  const timeStr = env.world_time_string ?? env.time ?? formatMinecraftTimeTicks(ticks).time;
  const phase = env.world_phase ?? env.phase ?? formatMinecraftTimeTicks(ticks).phase;
  const weather = env.world_weather ?? env.weather ?? currentServerWeather;
  currentServerWeather = weather;

  const timeDisplay = document.getElementById('timeText');
  const timePhase = document.getElementById('timePhaseText');
  const timeIcon = document.getElementById('timeIcon');
  const timeSlider = document.getElementById('timeSlider');
  const weatherIcon = document.getElementById('weatherIcon');
  const weatherText = document.getElementById('weatherText');

  if (timeDisplay) timeDisplay.textContent = timeStr;
  if (timePhase) timePhase.textContent = `(${phase})`;
  if (timeIcon) {
    if (ticks >= 23000 || ticks < 1000) timeIcon.textContent = '🌅';
    else if (ticks < 12000) timeIcon.textContent = '☀️';
    else if (ticks < 13500) timeIcon.textContent = '🌇';
    else timeIcon.textContent = '🌙';
  }

  if (timeSlider && !isDraggingTimeSlider) {
    timeSlider.value = String(ticks);
  }

  if (weatherIcon && weatherText) {
    if (weather === 'thunder') {
      weatherIcon.textContent = '⛈️';
      weatherText.textContent = '뇌우';
    } else if (weather === 'rain') {
      weatherIcon.textContent = '🌧️';
      weatherText.textContent = '비';
    } else {
      weatherIcon.textContent = '☀️';
      weatherText.textContent = '맑음';
    }
  }
}

async function fetchWorldEnvironment() {
  try {
    const res = await fetch('/api/world/environment');
    if (!res.ok) return;
    const data = await res.json();
    updateWorldEnvironmentUI(data);
  } catch (err) {}
}

async function sendSetTime(ticks) {
  ticks = parseInt(ticks, 10);
  if (isNaN(ticks)) return;
  currentServerTicks = ticks;
  updateWorldEnvironmentUI({ ticks, weather: currentServerWeather });

  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'set_time', ticks }));
  }
  try {
    await fetch('/api/admin/world/time', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ticks })
    });
  } catch (err) {}
}

async function sendSetWeather(weather) {
  currentServerWeather = weather;
  updateWorldEnvironmentUI({ weather, ticks: currentServerTicks });

  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'set_weather', weather }));
  }
  try {
    await fetch('/api/admin/world/weather', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ weather })
    });
  } catch (err) {}
}

function initDashboard() {
  // Process Controls
  const btnStart = document.getElementById('btnStart');
  const btnStop = document.getElementById('btnStop');
  const btnRestart = document.getElementById('btnRestart');
  if (btnStart) btnStart.addEventListener('click', () => controlProcess('start'));
  if (btnStop) btnStop.addEventListener('click', () => controlProcess('stop'));
  if (btnRestart) btnRestart.addEventListener('click', () => controlProcess('restart'));

  // Chat Submission
  const chatForm = document.getElementById('chatForm');
  const messageInput = document.getElementById('messageInput');
  if (chatForm && messageInput) {
    chatForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const text = messageInput.value.trim();
      if (!text) return;

      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'chat', text: text }));
        messageInput.value = '';
        return;
      }

      try {
        const res = await fetch('/api/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ message: text })
        });

        const data = await res.json();
        if (!res.ok) {
          showToast(data.error || '채팅 전송 실패', 'error');
          return;
        }

        messageInput.value = '';
        fetchChatMessages();
      } catch (err) {
        showToast('채팅 전송 중 통신 오류가 발생했습니다.', 'error');
      }
    });
  }

  // Player Action Modal Controls
  const btnClosePlayerModal = document.getElementById('btnClosePlayerModal');
  if (btnClosePlayerModal) {
    btnClosePlayerModal.addEventListener('click', closePlayerModal);
  }
  const playerActionModal = document.getElementById('playerActionModal');
  if (playerActionModal) {
    playerActionModal.addEventListener('click', (e) => {
      if (e.target === playerActionModal) closePlayerModal();
    });
  }
  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closePlayerModal();
  });

  const playerMsgForm = document.getElementById('playerMsgForm');
  if (playerMsgForm) {
    playerMsgForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const input = document.getElementById('playerMsgInput');
      const msg = input ? input.value.trim() : '';
      if (!msg) return;
      executePlayerAction('msg', [msg]);
      if (input) input.value = '';
    });
  }

  document.querySelectorAll('.btn-gm').forEach(b => {
    b.addEventListener('click', () => {
      if (b.dataset.gm) executePlayerAction('gamemode', [b.dataset.gm]);
    });
  });

  document.querySelectorAll('.btn-item-preset').forEach(b => {
    b.addEventListener('click', () => {
      if (b.dataset.item) executePlayerAction('give', [b.dataset.item, b.dataset.count || '1']);
    });
  });

  const playerGiveForm = document.getElementById('playerGiveForm');
  if (playerGiveForm) {
    playerGiveForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const item = document.getElementById('playerGiveItem')?.value.trim();
      const count = document.getElementById('playerGiveCount')?.value.trim() || '1';
      if (!item) return;
      executePlayerAction('give', [item, count]);
    });
  }

  document.getElementById('btnPlayerSpawn')?.addEventListener('click', () => executePlayerAction('spawn'));
  document.getElementById('btnPlayerClear')?.addEventListener('click', () => {
    if (confirm(`${selectedPlayerForModal}의 인벤토리를 비우시겠습니까?`)) {
      executePlayerAction('clear');
    }
  });
  document.getElementById('btnPlayerOp')?.addEventListener('click', () => executePlayerAction('op'));
  document.getElementById('btnPlayerDeop')?.addEventListener('click', () => executePlayerAction('deop'));

  document.getElementById('btnPlayerKick')?.addEventListener('click', () => {
    const reason = document.getElementById('playerPunishReason')?.value.trim() || '관리자에 의해 퇴장됨';
    executePlayerAction('kick', [reason]);
  });
  document.getElementById('btnPlayerBan')?.addEventListener('click', () => {
    if (confirm(`정말로 ${selectedPlayerForModal}을(를) 영구 차단(Ban)하시겠습니까?`)) {
      const reason = document.getElementById('playerPunishReason')?.value.trim() || '관리자에 의해 차단됨';
      executePlayerAction('ban', [reason]);
    }
  });

  // World Time Slider & Presets & Weather Controls
  const timeSlider = document.getElementById('timeSlider');
  if (timeSlider) {
    timeSlider.addEventListener('input', (e) => {
      isDraggingTimeSlider = true;
      const ticks = parseInt(e.target.value, 10);
      const res = formatMinecraftTimeTicks(ticks);
      const timeDisplay = document.getElementById('timeText');
      const timePhase = document.getElementById('timePhaseText');
      const timeIcon = document.getElementById('timeIcon');
      if (timeDisplay) timeDisplay.textContent = res.time;
      if (timePhase) timePhase.textContent = `(${res.phase})`;
      if (timeIcon) {
        if (ticks >= 23000 || ticks < 1000) timeIcon.textContent = '🌅';
        else if (ticks < 12000) timeIcon.textContent = '☀️';
        else if (ticks < 13500) timeIcon.textContent = '🌇';
        else timeIcon.textContent = '🌙';
      }
    });

    const commitSliderTime = () => {
      if (isDraggingTimeSlider) {
        isDraggingTimeSlider = false;
        const ticks = parseInt(timeSlider.value, 10);
        sendSetTime(ticks);
      }
    };

    timeSlider.addEventListener('change', (e) => {
      isDraggingTimeSlider = false;
      const ticks = parseInt(e.target.value, 10);
      sendSetTime(ticks);
    });

    timeSlider.addEventListener('mouseup', commitSliderTime);
    timeSlider.addEventListener('touchend', commitSliderTime);
  }

  document.querySelectorAll('.btn-time-preset').forEach(btn => {
    btn.addEventListener('click', () => {
      const ticks = parseInt(btn.dataset.ticks, 10);
      if (!isNaN(ticks)) {
        if (timeSlider) timeSlider.value = String(ticks);
        sendSetTime(ticks);
      }
    });
  });

  const btnWeatherToggle = document.getElementById('btnWeatherToggle');
  if (btnWeatherToggle) {
    btnWeatherToggle.addEventListener('click', () => {
      if (currentUser && currentUser.role === 'admin') {
        const nextWeather = (currentServerWeather === 'clear') ? 'rain' : (currentServerWeather === 'rain' ? 'thunder' : 'clear');
        sendSetWeather(nextWeather);
      }
    });
  }

  // BlueMap Toolbar Controls
  const btnReloadMap = document.getElementById('btnReloadMap');
  const bluemapFrame = document.getElementById('bluemapFrame');
  if (btnReloadMap && bluemapFrame) {
    btnReloadMap.addEventListener('click', () => {
      const currentSrc = bluemapFrame.src;
      bluemapFrame.src = 'about:blank';
      setTimeout(() => { bluemapFrame.src = currentSrc; }, 100);
    });
  }

  const btnFullscreenMap = document.getElementById('btnFullscreenMap');
  const mapWrapper = document.getElementById('mapWrapper');
  if (btnFullscreenMap && mapWrapper) {
    btnFullscreenMap.addEventListener('click', () => {
      if (!document.fullscreenElement) {
        if (mapWrapper.requestFullscreen) {
          mapWrapper.requestFullscreen().catch(() => {});
        } else if (mapWrapper.webkitRequestFullscreen) {
          mapWrapper.webkitRequestFullscreen();
        }
      } else {
        if (document.exitFullscreen) {
          document.exitFullscreen().catch(() => {});
        } else if (document.webkitExitFullscreen) {
          document.webkitExitFullscreen();
        }
      }
    });

    const updateFsBtnText = () => {
      if (document.fullscreenElement) {
        btnFullscreenMap.textContent = '전체 화면 종료 ✕';
        btnFullscreenMap.className = 'btn btn-secondary btn-sm';
      } else {
        btnFullscreenMap.textContent = '전체 화면으로 열기 ⛶';
        btnFullscreenMap.className = 'btn btn-primary btn-sm';
      }
    };

    document.addEventListener('fullscreenchange', updateFsBtnText);
    document.addEventListener('webkitfullscreenchange', updateFsBtnText);
  }

  // Offline Overlay Start Buttons (Dashboard & Map)
  const handleStartRequest = async (btn) => {
    const btnDashboardStart = document.getElementById('btnDashboardStart');
    const btnMapStart = document.getElementById('btnMapStart');
    if (btnDashboardStart) {
      btnDashboardStart.disabled = true;
      btnDashboardStart.textContent = '시작 요청 중...';
    }
    if (btnMapStart) {
      btnMapStart.disabled = true;
      btnMapStart.textContent = '시작 요청 중...';
    }
    await controlProcess('start');
  };

  const btnDashboardStart = document.getElementById('btnDashboardStart');
  if (btnDashboardStart) {
    btnDashboardStart.addEventListener('click', () => handleStartRequest(btnDashboardStart));
  }

  const btnMapStart = document.getElementById('btnMapStart');
  if (btnMapStart) {
    btnMapStart.addEventListener('click', () => handleStartRequest(btnMapStart));
  }
}

