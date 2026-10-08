// State Management
var currentUser = null;
var lastChatID = 0;
var socket = null;
var wsReconnectTimer = null;
var metricsHistoryList = [];
var playerPollTimer = null;
var processPollTimer = null;
var downloadPollTimer = null;
var consolePollTimer = null;
var chatPollTimer = null;
var metricsPollTimer = null;
var worldEnvPollTimer = null;
var selectedPlayerForModal = null;
var lastRenderedLogsText = '';
var schemaRegistry = [];
var latestPlayers = [];
var isDraggingTimeSlider = false;
var currentServerWeather = 'clear';
var currentServerTicks = 6000;

// Command Autocompletion & History State
var commandHistory = [];
var historyIndex = -1;
var currentSuggestions = [];
var selectedSuggestionIndex = -1;

const COMMAND_DEFINITIONS = [
  { cmd: 'help', desc: '명령어 목록 및 도움말 표시' },
  { cmd: 'list', desc: '현재 접속 중인 플레이어 목록 조회' },
  { cmd: 'tps', desc: '서버 틱 속도(TPS) 및 메모리 상태 조회' },
  { cmd: 'op', desc: '플레이어에게 관리자(OP) 권한 부여', args: ['<player>'] },
  { cmd: 'deop', desc: '플레이어의 관리자(OP) 권한 박탈', args: ['<player>'] },
  { cmd: 'gamemode', desc: '플레이어 게임 모드 변경', sub: ['survival', 'creative', 'adventure', 'spectator'] },
  { cmd: 'difficulty', desc: '게임 난이도 변경', sub: ['peaceful', 'easy', 'normal', 'hard'] },
  { cmd: 'time', desc: '월드 시간 조회 및 변경', sub: ['set day', 'set night', 'set noon', 'set midnight', 'query day'] },
  { cmd: 'weather', desc: '월드 날씨 변경', sub: ['clear', 'rain', 'thunder'] },
  { cmd: 'tp', desc: '플레이어를 대상 위치나 다른 플레이어로 텔레포트', args: ['<player>'] },
  { cmd: 'teleport', desc: '텔레포트 명령어', args: ['<player>'] },
  { cmd: 'kick', desc: '플레이어를 서버에서 강제 퇴장', args: ['<player>'] },
  { cmd: 'ban', desc: '플레이어를 서버에서 영구 차단', args: ['<player>'] },
  { cmd: 'pardon', desc: '플레이어의 차단(Ban) 해제', args: ['<player>'] },
  { cmd: 'whitelist', desc: '화이트리스트 관리', sub: ['on', 'off', 'list', 'add', 'remove', 'reload'] },
  { cmd: 'save-all', desc: '모든 월드 청크 데이터를 디스크에 즉시 저장' },
  { cmd: 'save-off', desc: '월드 자동 저장 비활성화' },
  { cmd: 'save-on', desc: '월드 자동 저장 활성화' },
  { cmd: 'say', desc: '서버 전체 공지 메시지 전송' },
  { cmd: 'tellraw', desc: '모든 플레이어에게 JSON 메시지 전송', sub: ['@a'] },
  { cmd: 'stop', desc: '서버를 안전하게 저장하고 종료' },
  { cmd: 'spark', desc: 'Spark 성능 프로파일러 제어', sub: ['tps', 'health', 'profiler open', 'sampler'] },
  { cmd: 'purpur', desc: 'Purpur 구동기 설정 재로딩 및 정보', sub: ['reload', 'version'] },
  { cmd: 'bluemap', desc: 'BlueMap 지도 제어', sub: ['reload', 'render', 'pause', 'resume', 'purge'] },
  { cmd: 'gamerule', desc: '게임 규칙 조회 및 변경', sub: ['keepInventory', 'mobGriefing', 'doDaylightCycle', 'doWeatherCycle', 'doMobSpawning'] },
  { cmd: 'give', desc: '플레이어에게 아이템 지급', args: ['<player>'] },
  { cmd: 'clear', desc: '플레이어의 인벤토리 비우기', args: ['<player>'] },
  { cmd: 'kill', desc: '엔티티 또는 플레이어 처치', args: ['<player>'] },
  { cmd: 'seed', desc: '월드 생성 시드(Seed) 확인' },
  { cmd: 'worldborder', desc: '월드 경계 크기 및 중심 제어', sub: ['set', 'add', 'center', 'get'] },
  { cmd: 'xp', desc: '플레이어에게 경험치 지급 또는 차감', sub: ['add', 'set', 'query'] }
];

// Toast Notifications
function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    setTimeout(() => toast.remove(), 200);
  }, 3000);
}

// Utility: HTML escaping
function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// Utility: Format seconds to readable uptime
function formatUptime(sec) {
  if (!sec || sec < 0) return '0s';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

// Utility: Convert Minecraft ticks (0-24000) to Korean time string & day/night phase
function formatMinecraftTimeTicks(ticks) {
  ticks = ((ticks % 24000) + 24000) % 24000;
  const hours = Math.floor((ticks / 1000 + 6) % 24);
  const mins = Math.floor(((ticks % 1000) * 60) / 1000);
  const timeStr = `${String(hours).padStart(2, '0')}:${String(mins).padStart(2, '0')}`;
  let phase = '낮';
  if (ticks >= 23000 || ticks < 1000) phase = '일출';
  else if (ticks < 6000) phase = '아침/낮';
  else if (ticks < 7000) phase = '정오';
  else if (ticks < 12000) phase = '오후';
  else if (ticks < 13500) phase = '일몰';
  else if (ticks < 17500) phase = '밤';
  else if (ticks < 18500) phase = '자정';
  else phase = '새벽';
  return { time: timeStr, phase };
}

// Timer clear helper
function clearTimers() {
  if (playerPollTimer) clearInterval(playerPollTimer);
  if (processPollTimer) clearInterval(processPollTimer);
  if (downloadPollTimer) clearInterval(downloadPollTimer);
  if (consolePollTimer) clearInterval(consolePollTimer);
  if (chatPollTimer) clearInterval(chatPollTimer);
  if (metricsPollTimer) clearInterval(metricsPollTimer);
  if (worldEnvPollTimer) clearInterval(worldEnvPollTimer);
  if (wsReconnectTimer) clearTimeout(wsReconnectTimer);
  if (socket) {
    try { socket.close(); } catch (e) {}
    socket = null;
  }
}
