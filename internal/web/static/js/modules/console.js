// Console & Monitoring Module: Server Logs, Interactive Terminal, Tab Autocompletion, Resource Telemetry & Chart

// Fetch Console Logs
async function fetchLogs() {
  try {
    const res = await fetch('/api/admin/process/logs');
    if (!res.ok) return;

    const data = await res.json();
    if (data.logs) {
      appendConsole(data.logs);
    }
  } catch (err) {}
}

function appendConsole(logs) {
  const consoleOutput = document.getElementById('consoleOutput');
  if (!logs || !consoleOutput) return;

  const list = Array.isArray(logs) ? logs : String(logs).split('\n');
  const filtered = list.filter(line => {
    const trimmed = (line || '').trim();
    if (trimmed.match(/^(?:>\s*){2,}$/)) return false;
    return true;
  });

  const fullText = filtered.join('\n');
  if (fullText === lastRenderedLogsText && consoleOutput.textContent !== '서버 로그 로딩 중...\n' && consoleOutput.textContent !== '서버 로그 로딩 중...') {
    return;
  }
  lastRenderedLogsText = fullText;

  const isAtBottom = (consoleOutput.scrollHeight - consoleOutput.scrollTop <= consoleOutput.clientHeight + 80);
  consoleOutput.textContent = fullText ? (fullText.endsWith('\n') ? fullText : fullText + '\n') : '기록된 서버 로그가 없습니다.\n';
  if (isAtBottom) {
    consoleOutput.scrollTop = consoleOutput.scrollHeight;
  }
}

function appendConsoleSingleLine(line) {
  const consoleOutput = document.getElementById('consoleOutput');
  if (!line || !consoleOutput) return;
  const trimmed = line.trim();
  if (trimmed.match(/^(?:>\s*){2,}$/)) return;

  const isAtBottom = (consoleOutput.scrollHeight - consoleOutput.scrollTop <= consoleOutput.clientHeight + 80);
  if (consoleOutput.textContent === '서버 로그 로딩 중...\n' || consoleOutput.textContent === '기록된 서버 로그가 없습니다.\n') {
    consoleOutput.textContent = '';
  }
  consoleOutput.textContent += line.endsWith('\n') ? line : line + '\n';
  if (isAtBottom) {
    consoleOutput.scrollTop = consoleOutput.scrollHeight;
  }
}

async function sendConsoleCommand(cmd) {
  if (!cmd) return;
  appendConsoleSingleLine(`> ${cmd}`);

  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'command', command: cmd }));
    return;
  }

  // Fallback to HTTP
  try {
    const res = await fetch('/api/command', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command: cmd })
    });

    const data = await res.json();
    if (!res.ok) {
      appendConsoleSingleLine(`[Error] ${data.error}`);
      return;
    }

    if (data.output) {
      appendConsoleSingleLine(data.output);
    }
  } catch (err) {
    appendConsoleSingleLine(`[Error] ${err.message}`);
  }
}

// Command Autocompletion Logic
function updateCommandSuggestions() {
  const consoleCommand = document.getElementById('consoleCommand');
  const commandSuggestions = document.getElementById('commandSuggestions');
  if (!consoleCommand || !commandSuggestions) return;
  const text = consoleCommand.value;
  const trimmed = text.trimStart();
  if (!trimmed) {
    hideSuggestions();
    return;
  }

  const parts = trimmed.split(/\s+/);
  const firstWord = parts[0].toLowerCase();

  let matches = [];

  if (parts.length === 1 && !text.endsWith(' ')) {
    // Matching root command
    matches = COMMAND_DEFINITIONS.filter(c => c.cmd.toLowerCase().startsWith(firstWord)).map(c => ({
      insertText: c.cmd,
      displayCmd: c.cmd,
      desc: c.desc
    }));
  } else {
    // Subcommand or player argument matching
    const def = COMMAND_DEFINITIONS.find(c => c.cmd.toLowerCase() === firstWord);
    if (def) {
      const remaining = parts.slice(1).join(' ').toLowerCase();

      // If command accepts player names
      if (def.args && def.args.includes('<player>')) {
        const players = Array.isArray(latestPlayers) ? latestPlayers : [];
        players.forEach(p => {
          if (!remaining || p.toLowerCase().startsWith(remaining)) {
            matches.push({
              insertText: `${def.cmd} ${p}`,
              displayCmd: `${def.cmd} ${p}`,
              desc: `온라인 플레이어: ${p}`
            });
          }
        });
      }

      // If command has subcommands
      if (def.sub) {
        def.sub.forEach(sub => {
          if (!remaining || sub.toLowerCase().startsWith(remaining)) {
            matches.push({
              insertText: `${def.cmd} ${sub}`,
              displayCmd: `${def.cmd} ${sub}`,
              desc: def.desc
            });
          }
        });
      }
    }
  }

  currentSuggestions = matches.slice(0, 8);
  renderSuggestions();
}

function renderSuggestions() {
  const commandSuggestions = document.getElementById('commandSuggestions');
  if (!commandSuggestions) return;
  if (currentSuggestions.length === 0) {
    hideSuggestions();
    return;
  }

  commandSuggestions.innerHTML = '';
  selectedSuggestionIndex = 0;

  currentSuggestions.forEach((item, idx) => {
    const div = document.createElement('div');
    div.className = `suggestion-item ${idx === selectedSuggestionIndex ? 'selected' : ''}`;
    div.innerHTML = `
      <span class="suggestion-cmd">${escapeHtml(item.displayCmd)}</span>
      <span class="suggestion-desc">${escapeHtml(item.desc)}</span>
    `;
    div.addEventListener('mousedown', (e) => {
      e.preventDefault();
      applySuggestion(item.insertText);
    });
    commandSuggestions.appendChild(div);
  });

  commandSuggestions.style.display = 'flex';
}

function hideSuggestions() {
  const commandSuggestions = document.getElementById('commandSuggestions');
  if (!commandSuggestions) return;
  currentSuggestions = [];
  selectedSuggestionIndex = -1;
  commandSuggestions.style.display = 'none';
}

function applySuggestion(text) {
  const consoleCommand = document.getElementById('consoleCommand');
  if (!consoleCommand) return;
  consoleCommand.value = text + ' ';
  hideSuggestions();
  consoleCommand.focus();
}

function updateSuggestionSelection() {
  const commandSuggestions = document.getElementById('commandSuggestions');
  if (!commandSuggestions) return;
  const items = commandSuggestions.querySelectorAll('.suggestion-item');
  items.forEach((item, idx) => {
    item.classList.toggle('selected', idx === selectedSuggestionIndex);
  });
}

function handleConsoleKeyDown(e) {
  const commandSuggestions = document.getElementById('commandSuggestions');
  const consoleCommand = document.getElementById('consoleCommand');
  if (!consoleCommand) return;

  if (commandSuggestions && commandSuggestions.style.display !== 'none' && currentSuggestions.length > 0) {
    if (e.key === 'Tab') {
      e.preventDefault();
      const sel = currentSuggestions[selectedSuggestionIndex] || currentSuggestions[0];
      if (sel) applySuggestion(sel.insertText);
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedSuggestionIndex = (selectedSuggestionIndex + 1) % currentSuggestions.length;
      updateSuggestionSelection();
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedSuggestionIndex = (selectedSuggestionIndex - 1 + currentSuggestions.length) % currentSuggestions.length;
      updateSuggestionSelection();
      return;
    }
    if (e.key === 'Escape') {
      hideSuggestions();
      return;
    }
  } else {
    // Command History Navigation
    if (e.key === 'ArrowUp') {
      if (commandHistory.length > 0) {
        e.preventDefault();
        if (historyIndex === -1) historyIndex = commandHistory.length - 1;
        else if (historyIndex > 0) historyIndex--;
        consoleCommand.value = commandHistory[historyIndex] || '';
      }
    } else if (e.key === 'ArrowDown') {
      if (commandHistory.length > 0 && historyIndex !== -1) {
        e.preventDefault();
        if (historyIndex < commandHistory.length - 1) {
          historyIndex++;
          consoleCommand.value = commandHistory[historyIndex] || '';
        } else {
          historyIndex = -1;
          consoleCommand.value = '';
        }
      }
    }
  }
}

// System Metrics Rendering & Live Update
function renderMetricsData(cur, history) {
  if (!cur) return;

  const isRunning = !!cur.server_running;

  // Sync World Environment telemetry (Time & Weather) to top header widget
  if (typeof updateWorldEnvironmentUI === 'function') {
    updateWorldEnvironmentUI(cur);
  }

  // 1. CPU
  const cpuVal = isRunning ? (cur.process_cpu_percent ?? 0) : 0;
  const cpuEl = document.getElementById('metricCpuVal');
  const cpuBar = document.getElementById('metricCpuBar');
  if (cpuEl) cpuEl.textContent = `${cpuVal.toFixed(1)}%`;
  if (cpuBar) cpuBar.style.width = `${Math.min(cpuVal, 100)}%`;

  // 2. RAM
  const ramVal = isRunning ? (cur.process_memory_mb ?? 0) : 0;
  const ramEl = document.getElementById('metricRamVal');
  const ramBar = document.getElementById('metricRamBar');
  if (ramEl) ramEl.textContent = `${ramVal.toFixed(0)} MB`;
  const ramPct = Math.min((ramVal / 4096) * 100, 100);
  if (ramBar) ramBar.style.width = `${ramPct.toFixed(1)}%`;

  // 3. TPS
  const tpsEl = document.getElementById('metricTpsVal');
  const tps1m = document.getElementById('metricTps1m');
  const tps5m = document.getElementById('metricTps5m');
  const tps15m = document.getElementById('metricTps15m');
  if (tpsEl) {
    if (isRunning) {
      const tpsVal = cur.tps_1m ?? 20.0;
      tpsEl.textContent = tpsVal.toFixed(1);
      tpsEl.style.color = tpsVal >= 18 ? 'var(--status-running)' : (tpsVal >= 14 ? '#f59e0b' : 'var(--status-stopped)');
    } else {
      tpsEl.textContent = '-';
      tpsEl.style.color = 'var(--text-muted)';
    }
  }
  if (tps1m) tps1m.textContent = isRunning ? (cur.tps_1m ?? 20).toFixed(1) : '-';
  if (tps5m) tps5m.textContent = isRunning ? (cur.tps_5m ?? 20).toFixed(1) : '-';
  if (tps15m) tps15m.textContent = isRunning ? (cur.tps_15m ?? 20).toFixed(1) : '-';

  // 4. Process Runtime Info
  const statusText = document.getElementById('metricStatusText');
  const pidText = document.getElementById('metricPidText');
  const uptimeText = document.getElementById('metricUptimeText');
  const playersText = document.getElementById('metricPlayersText');
  if (statusText) {
    statusText.textContent = isRunning ? 'RUNNING' : 'STOPPED';
    statusText.className = `badge ${isRunning ? 'admin' : 'user'}`;
  }
  if (pidText) pidText.textContent = (isRunning && cur.process_pid) ? String(cur.process_pid) : '-';
  if (uptimeText) uptimeText.textContent = isRunning ? formatUptime(cur.process_uptime) : '-';
  if (playersText) playersText.textContent = isRunning ? `${cur.online_players ?? 0} / ${cur.max_players ?? 20}` : '- / -';

  // 5. History Buffer Management (Keep last 40 samples)
  if (history && Array.isArray(history) && history.length > 0) {
    metricsHistoryList = history.slice(-40);
  } else if (cur) {
    metricsHistoryList.push(cur);
    if (metricsHistoryList.length > 40) {
      metricsHistoryList.shift();
    }
  }

  // 6. Render Real-time Graph (Canvas)
  renderMetricsChart();
}

// Canvas-based Real-time Multi-line Telemetry Graph
function renderMetricsChart() {
  const canvas = document.getElementById('metricsChartCanvas');
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  const width = rect.width || canvas.clientWidth || 400;
  const height = rect.height || canvas.clientHeight || 200;

  if (canvas.width !== Math.floor(width * dpr) || canvas.height !== Math.floor(height * dpr)) {
    canvas.width = Math.floor(width * dpr);
    canvas.height = Math.floor(height * dpr);
  }

  ctx.save();
  ctx.scale(dpr, dpr);
  ctx.clearRect(0, 0, width, height);

  const padding = { top: 20, right: 20, bottom: 25, left: 35 };
  const chartW = width - padding.left - padding.right;
  const chartH = height - padding.top - padding.bottom;

  // Background grid lines
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
  ctx.lineWidth = 1;
  ctx.fillStyle = '#64748b';
  ctx.font = '10px monospace';
  ctx.textAlign = 'right';

  for (let i = 0; i <= 4; i++) {
    const y = padding.top + (chartH / 4) * i;
    const label = `${100 - i * 25}%`;
    ctx.beginPath();
    ctx.moveTo(padding.left, y);
    ctx.lineTo(width - padding.right, y);
    ctx.stroke();
    ctx.fillText(label, padding.left - 6, y + 3);
  }

  if (!metricsHistoryList || metricsHistoryList.length === 0) {
    ctx.fillStyle = '#64748b';
    ctx.textAlign = 'center';
    ctx.font = '12px sans-serif';
    ctx.fillText('메트릭 데이터 수집 대기 중...', width / 2, height / 2);
    ctx.restore();
    return;
  }

  const samples = metricsHistoryList.slice(-30);
  const n = samples.length;
  if (n < 2) {
    ctx.fillStyle = '#64748b';
    ctx.textAlign = 'center';
    ctx.font = '12px sans-serif';
    ctx.fillText('초기 샘플 축적 중...', width / 2, height / 2);
    ctx.restore();
    return;
  }

  const getX = (idx) => padding.left + (chartW / (n - 1)) * idx;

  function drawSeries(getValue, strokeColor, fillColor) {
    ctx.beginPath();
    for (let i = 0; i < n; i++) {
      const val = Math.max(0, Math.min(100, getValue(samples[i])));
      const x = getX(i);
      const y = padding.top + chartH * (1 - val / 100);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    }
    ctx.strokeStyle = strokeColor;
    ctx.lineWidth = 2;
    ctx.stroke();

    if (fillColor) {
      ctx.lineTo(getX(n - 1), padding.top + chartH);
      ctx.lineTo(getX(0), padding.top + chartH);
      ctx.closePath();
      ctx.fillStyle = fillColor;
      ctx.fill();
    }
  }

  // 1. Memory % (of 4096MB)
  drawSeries(
    s => s.server_running ? ((s.process_memory_mb || 0) / 4096) * 100 : 0,
    '#c084fc',
    'rgba(192, 132, 252, 0.08)'
  );

  // 2. CPU %
  drawSeries(
    s => s.server_running ? (s.process_cpu_percent || 0) : 0,
    '#60a5fa',
    'rgba(96, 165, 250, 0.12)'
  );

  // 3. TPS % (of 20.0)
  drawSeries(
    s => s.server_running ? ((s.tps_1m || 20) / 20) * 100 : 0,
    '#4ade80',
    null
  );

  // X-axis time labels (first, middle, last)
  ctx.fillStyle = '#64748b';
  ctx.textAlign = 'center';
  ctx.font = '9px monospace';
  if (samples[0]?.timestamp) {
    ctx.fillText(samples[0].timestamp, getX(0), height - 8);
  }
  if (samples[Math.floor(n / 2)]?.timestamp) {
    ctx.fillText(samples[Math.floor(n / 2)].timestamp, getX(Math.floor(n / 2)), height - 8);
  }
  if (samples[n - 1]?.timestamp) {
    ctx.fillText(samples[n - 1].timestamp, getX(n - 1), height - 8);
  }

  ctx.restore();
}

async function fetchMetrics() {
  try {
    const res = await fetch('/api/admin/metrics');
    if (!res.ok) return;
    const data = await res.json();
    if (data.current) {
      renderMetricsData(data.current, data.history);
    }
  } catch (err) {}
}

function initConsole() {
  const consoleForm = document.getElementById('consoleForm');
  const consoleCommand = document.getElementById('consoleCommand');
  const btnClearConsole = document.getElementById('btnClearConsole');
  const btnQuickList = document.getElementById('btnQuickList');
  const btnQuickTps = document.getElementById('btnQuickTps');
  const btnQuickSave = document.getElementById('btnQuickSave');
  const btnRefreshMetrics = document.getElementById('btnRefreshMetrics');
  const consoleOutput = document.getElementById('consoleOutput');

  if (consoleForm && consoleCommand) {
    consoleForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const cmd = consoleCommand.value.trim();
      if (!cmd) return;
      commandHistory.push(cmd);
      historyIndex = -1;
      hideSuggestions();
      consoleCommand.value = '';
      sendConsoleCommand(cmd);
    });

    consoleCommand.addEventListener('input', updateCommandSuggestions);
    consoleCommand.addEventListener('keydown', handleConsoleKeyDown);
    consoleCommand.addEventListener('blur', () => {
      setTimeout(hideSuggestions, 200);
    });
  }

  // Quick Action Console Buttons
  if (btnQuickList) {
    btnQuickList.addEventListener('click', () => sendConsoleCommand('list'));
  }
  if (btnQuickTps) {
    btnQuickTps.addEventListener('click', () => sendConsoleCommand('tps'));
  }
  if (btnQuickSave) {
    btnQuickSave.addEventListener('click', () => sendConsoleCommand('save-all'));
  }

  if (btnClearConsole && consoleOutput) {
    btnClearConsole.addEventListener('click', () => {
      consoleOutput.textContent = '';
      lastRenderedLogsText = '';
    });
  }

  if (btnRefreshMetrics) {
    btnRefreshMetrics.addEventListener('click', fetchMetrics);
  }

  window.addEventListener('resize', () => {
    renderMetricsChart();
  });
}
