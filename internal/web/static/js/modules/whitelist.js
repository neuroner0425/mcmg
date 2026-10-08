// Whitelist Management Module: Admin Controls & User Self-Registration

// Whitelist Management (Admin View)
async function fetchWhitelist() {
  try {
    const res = await fetch('/api/admin/whitelist');
    if (!res.ok) return;

    const data = await res.json();
    const wlToggleActive = document.getElementById('wlToggleActive');
    const wlToggleEnforce = document.getElementById('wlToggleEnforce');
    if (wlToggleActive) wlToggleActive.checked = !!data.enabled;
    if (wlToggleEnforce) wlToggleEnforce.checked = !!data.enforce;

    const tbody = document.getElementById('whitelistTableBody');
    if (!tbody) return;
    tbody.innerHTML = '';

    const entries = data.entries || [];
    if (entries.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-muted); padding: 24px;">등록된 화이트리스트 플레이어가 없습니다.</td></tr>';
      return;
    }

    entries.forEach((p, idx) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td style="color: var(--text-muted); font-size: 0.8rem;">${idx + 1}</td>
        <td style="font-weight: 600; display: flex; align-items: center; gap: 8px;">
          <img class="player-avatar" src="https://minotar.net/avatar/${encodeURIComponent(p.name)}/20" alt="${escapeHtml(p.name)}">
          <span>${escapeHtml(p.name)}</span>
        </td>
        <td style="font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-secondary);">${escapeHtml(p.uuid || '-')}</td>
        <td style="text-align: center;">
          <button class="btn btn-danger btn-sm" onclick="removeWhitelistPlayer('${escapeHtml(p.name)}')">제거</button>
        </td>
      `;
      tbody.appendChild(tr);
    });
  } catch (err) {}
}

async function saveWhitelistConfig() {
  const wlToggleActive = document.getElementById('wlToggleActive');
  const wlToggleEnforce = document.getElementById('wlToggleEnforce');
  const enabled = wlToggleActive ? wlToggleActive.checked : false;
  const enforce = wlToggleEnforce ? wlToggleEnforce.checked : false;

  try {
    const res = await fetch('/api/admin/whitelist/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled, enforce })
    });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '화이트리스트 설정 저장 실패', 'error');
      return;
    }
    showToast(data.message || '화이트리스트 설정이 저장되었습니다.');
    fetchWhitelist();
  } catch (err) {
    showToast('통신 오류가 발생했습니다.', 'error');
  }
}

async function handleAddWhitelistPlayer(e) {
  e.preventDefault();
  const input = document.getElementById('wlPlayerNameInput');
  const name = input ? input.value.trim() : '';
  if (!name) return;

  try {
    const res = await fetch('/api/admin/whitelist/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '화이트리스트 등록 실패', 'error');
      return;
    }
    showToast(data.message || '화이트리스트에 등록되었습니다.');
    if (input) input.value = '';
    fetchWhitelist();
  } catch (err) {
    showToast('통신 오류가 발생했습니다.', 'error');
  }
}

async function removeWhitelistPlayer(name) {
  if (!confirm(`'${name}' 플레이어를 화이트리스트에서 제거하시겠습니까?`)) return;

  try {
    const res = await fetch('/api/admin/whitelist/remove', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '화이트리스트 제거 실패', 'error');
      return;
    }
    showToast(data.message || '화이트리스트에서 제거되었습니다.');
    fetchWhitelist();
  } catch (err) {
    showToast('통신 오류가 발생했습니다.', 'error');
  }
}

// User Whitelist Self-Registration (Accessible by All Users, Without List Management)
async function handleUserWlRegister(e) {
  e.preventDefault();
  const input = document.getElementById('userWlNameInput');
  const msgEl = document.getElementById('userWlStatusMsg');
  const btn = document.getElementById('btnUserWlRegister');
  const name = input ? input.value.trim() : '';
  if (!name) return;

  if (btn) {
    btn.disabled = true;
    btn.textContent = '등록 중...';
  }

  try {
    const res = await fetch('/api/whitelist/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    });
    const data = await res.json();
    if (!res.ok) {
      const errMsg = data.error || '화이트리스트 등록에 실패했습니다.';
      showToast(errMsg, 'error');
      if (msgEl) {
        msgEl.style.display = 'block';
        msgEl.style.color = '#ef4444';
        msgEl.textContent = `✕ ${errMsg}`;
      }
      return;
    }
    const succMsg = data.message || `'${name}' 등록이 완료되었습니다.`;
    showToast(succMsg, 'success');
    if (msgEl) {
      msgEl.style.display = 'block';
      msgEl.style.color = '#10b981';
      msgEl.textContent = `✓ '${name}' 등록 완료`;
    }
    if (input) input.value = '';

    // If current user is admin, refresh the admin whitelist table if present
    if (currentUser && currentUser.role === 'admin') {
      fetchWhitelist();
    }
  } catch (err) {
    showToast('통신 오류가 발생했습니다.', 'error');
    if (msgEl) {
      msgEl.style.display = 'block';
      msgEl.style.color = '#ef4444';
      msgEl.textContent = '✕ 통신 오류가 발생했습니다.';
    }
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = '등록하기';
    }
  }
}

// Attach to window for inline HTML onclick handlers
window.removeWhitelistPlayer = removeWhitelistPlayer;

function initWhitelist() {
  const btnSaveWhitelistConfig = document.getElementById('btnSaveWhitelistConfig');
  if (btnSaveWhitelistConfig) {
    btnSaveWhitelistConfig.addEventListener('click', saveWhitelistConfig);
  }
  const wlAddForm = document.getElementById('wlAddForm');
  if (wlAddForm) {
    wlAddForm.addEventListener('submit', handleAddWhitelistPlayer);
  }
  const btnRefreshWhitelist = document.getElementById('btnRefreshWhitelist');
  if (btnRefreshWhitelist) {
    btnRefreshWhitelist.addEventListener('click', fetchWhitelist);
  }
  const userWlRegisterForm = document.getElementById('userWlRegisterForm');
  if (userWlRegisterForm) {
    userWlRegisterForm.addEventListener('submit', handleUserWlRegister);
  }
}
