// Authentication & WebSocket Module

async function checkAuth() {
  try {
    const res = await fetch('/api/me');
    if (res.ok) {
      const data = await res.json();
      currentUser = data;
      showApp(data);
    } else {
      showLogin();
    }
  } catch (err) {
    showLogin();
  }
}

function showLogin() {
  clearTimers();
  const authView = document.getElementById('authView');
  const appView = document.getElementById('appView');
  const loginNickname = document.getElementById('loginNickname');

  if (authView) authView.style.display = 'flex';
  if (appView) appView.style.display = 'none';

  // Restore saved nickname if any
  const savedNick = localStorage.getItem('mc_nickname');
  if (savedNick && loginNickname) {
    loginNickname.value = savedNick;
  }
}

function showApp(userData) {
  const authView = document.getElementById('authView');
  const appView = document.getElementById('appView');
  const userNicknameBadge = document.getElementById('userNicknameBadge');
  const senderInput = document.getElementById('senderInput');
  const roleBadge = document.getElementById('roleBadge');
  const bluemapFrame = document.getElementById('bluemapFrame');

  if (authView) authView.style.display = 'none';
  if (appView) appView.style.display = 'flex';

  const nick = userData.nickname || localStorage.getItem('mc_nickname') || 'Player';
  if (userNicknameBadge) userNicknameBadge.textContent = nick;
  if (senderInput) senderInput.value = nick;

  const isAdmin = (userData.role === 'admin');
  if (roleBadge) {
    roleBadge.textContent = isAdmin ? 'ADMIN' : 'USER';
    roleBadge.className = `badge ${isAdmin ? 'admin' : 'user'}`;
  }

  // Toggle admin-only elements across sidebar and top header
  const adminElements = document.querySelectorAll('.admin-only');
  adminElements.forEach(el => {
    if (el.tagName === 'DIV' && (el.classList.contains('sidebar-section-divider') || el.classList.contains('sidebar-section-label'))) {
      el.style.display = isAdmin ? 'block' : 'none';
    } else if (el.classList.contains('status-actions')) {
      el.style.display = isAdmin ? 'flex' : 'none';
    } else {
      el.style.display = isAdmin ? 'flex' : 'none';
    }
  });

  // Whitelist view switching by user role
  const userWlView = document.getElementById('userWhitelistView');
  const adminWlView = document.getElementById('adminWhitelistView');
  if (userWlView) userWlView.style.display = isAdmin ? 'none' : 'block';
  if (adminWlView) adminWlView.style.display = isAdmin ? 'block' : 'none';

  const userWlNameInput = document.getElementById('userWlNameInput');
  if (userWlNameInput && (!userWlNameInput.value || userWlNameInput.value === '')) {
    if (nick && nick !== 'Player') {
      userWlNameInput.value = nick;
    }
  }

  const mapUrl = userData.map_url || userData.bluemap_url || '/squaremap/';
  if (bluemapFrame) {
    bluemapFrame.src = mapUrl;
  }

  const timeSlider = document.getElementById('timeSlider');
  if (timeSlider) {
    timeSlider.disabled = !isAdmin;
    timeSlider.style.cursor = isAdmin ? 'pointer' : 'default';
  }
  const btnWeatherToggle = document.getElementById('btnWeatherToggle');
  if (btnWeatherToggle) {
    btnWeatherToggle.style.cursor = isAdmin ? 'pointer' : 'default';
    btnWeatherToggle.title = isAdmin ? '날씨 (클릭하여 변경)' : '현재 날씨';
  }

  clearTimers();

  // Establish instant real-time WebSocket connection
  initWebSocket();

  // Fetch World Environment (Weather & Time) with continuous polling fallback
  if (typeof fetchWorldEnvironment === 'function') {
    fetchWorldEnvironment();
    worldEnvPollTimer = setInterval(fetchWorldEnvironment, 2500);
  }

  // 1. Players polling (fallback / keepalive 5s)
  if (typeof fetchPlayers === 'function') {
    fetchPlayers();
    playerPollTimer = setInterval(fetchPlayers, 5000);
  }

  // 2. Process status polling (3s)
  if (typeof fetchProcessStatus === 'function') {
    fetchProcessStatus();
    processPollTimer = setInterval(fetchProcessStatus, 3000);
  }

  // 3. Real-time In-Game Chat polling (fallback 3s)
  if (typeof fetchChatMessages === 'function') {
    fetchChatMessages();
    chatPollTimer = setInterval(fetchChatMessages, 3000);
  }

  if (isAdmin) {
    if (typeof fetchLogs === 'function') {
      consolePollTimer = setInterval(fetchLogs, 4000);
    }
    if (typeof fetchMetrics === 'function') {
      metricsPollTimer = setInterval(fetchMetrics, 3000);
    }
  }

  // Navigate according to current browser URL
  if (typeof navigateByPath === 'function') {
    navigateByPath(window.location.pathname, false);
  }
}

function initWebSocket() {
  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    return;
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  try {
    socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      console.log('[WS] Connected to Sanctum live stream');
      if (wsReconnectTimer) {
        clearTimeout(wsReconnectTimer);
        wsReconnectTimer = null;
      }
    };

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleWebSocketMessage(msg);
      } catch (e) {}
    };

    socket.onclose = () => {
      socket = null;
      if (!wsReconnectTimer && currentUser) {
        wsReconnectTimer = setTimeout(initWebSocket, 3000);
      }
    };

    socket.onerror = () => {
      try { socket.close(); } catch (e) {}
    };
  } catch (err) {
    if (currentUser) {
      wsReconnectTimer = setTimeout(initWebSocket, 3000);
    }
  }
}

function handleWebSocketMessage(msg) {
  if (!msg) return;

  switch (msg.type) {
    case 'init':
      if (msg.payload) {
        if (msg.payload.logs && typeof appendConsole === 'function') {
          appendConsole(msg.payload.logs);
        }
        if (msg.payload.chats && typeof appendChatMessage === 'function') {
          const chatHistory = document.getElementById('chatHistory');
          if (chatHistory) chatHistory.innerHTML = '';
          msg.payload.chats.forEach(c => {
            if (c.id > lastChatID) lastChatID = c.id;
            appendChatMessage(c);
          });
          if (chatHistory) chatHistory.scrollTop = chatHistory.scrollHeight;
        }
        if (msg.payload.metrics && typeof renderMetricsData === 'function') {
          renderMetricsData(msg.payload.metrics);
        }
      }
      break;

    case 'log':
      if (msg.payload && typeof appendConsoleSingleLine === 'function') {
        appendConsoleSingleLine(msg.payload);
      }
      break;

    case 'chat':
      if (msg.payload && typeof appendChatMessage === 'function') {
        const c = msg.payload;
        if (c.id > lastChatID) lastChatID = c.id;
        appendChatMessage(c);
        const chatHistory = document.getElementById('chatHistory');
        if (chatHistory) chatHistory.scrollTop = chatHistory.scrollHeight;
      }
      break;

    case 'metrics':
      if (msg.payload && typeof renderMetricsData === 'function') {
        renderMetricsData(msg.payload);
      }
      break;

    case 'command_result':
      if (msg.payload && msg.payload.response && typeof appendConsoleSingleLine === 'function') {
        appendConsoleSingleLine(msg.payload.response);
      }
      break;
  }
}

function initAuth() {
  const loginForm = document.getElementById('loginForm');
  const loginNickname = document.getElementById('loginNickname');
  const loginPassword = document.getElementById('loginPassword');
  const loginError = document.getElementById('loginError');
  const logoutBtn = document.getElementById('logoutBtn');

  if (loginForm) {
    loginForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      if (loginError) loginError.textContent = '';
      const nick = loginNickname ? loginNickname.value.trim() : '';
      const pwd = loginPassword ? loginPassword.value.trim() : '';
      if (!nick || !pwd) return;

      localStorage.setItem('mc_nickname', nick);

      try {
        const res = await fetch('/api/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ nickname: nick, password: pwd })
        });

        const data = await res.json();
        if (!res.ok) {
          if (loginError) loginError.textContent = data.error || '접속 실패';
          return;
        }

        if (loginPassword) loginPassword.value = '';
        checkAuth();
        showToast(`${nick} 님 환영합니다.`);
      } catch (err) {
        if (loginError) loginError.textContent = '통신 오류가 발생했습니다.';
      }
    });
  }

  if (logoutBtn) {
    logoutBtn.addEventListener('click', async () => {
      try {
        await fetch('/api/logout', { method: 'POST' });
        showLogin();
        showToast('로그아웃되었습니다.');
      } catch (err) {
        showLogin();
      }
    });
  }
}
