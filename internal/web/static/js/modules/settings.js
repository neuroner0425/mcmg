// Server Settings & Administration Module: Detailed Config, Purpur Installer, Plugins & Backups

// Config Schema Handling
async function fetchConfigSchema() {
  const schemaFormContainer = document.getElementById('schemaFormContainer');
  try {
    const res = await fetch('/api/admin/config/schema');
    if (!res.ok) return;

    const data = await res.json();
    schemaRegistry = data.schema || [];
    renderSchemaForm(schemaRegistry);
  } catch (err) {
    if (schemaFormContainer) {
      schemaFormContainer.innerHTML = '<div style="text-align: center; padding: 30px; color: var(--status-stopped);">설정을 불러오지 못했습니다.</div>';
    }
  }
}

function renderSchemaForm(items) {
  const schemaFormContainer = document.getElementById('schemaFormContainer');
  if (!schemaFormContainer) return;
  schemaFormContainer.innerHTML = '';

  // Group by category
  const categories = {};
  items.forEach(item => {
    const cat = item.category || '기타';
    if (!categories[cat]) categories[cat] = [];
    categories[cat].push(item);
  });

  for (const [catName, list] of Object.entries(categories)) {
    const groupWrap = document.createElement('div');
    groupWrap.className = 'config-category-group';
    groupWrap.dataset.category = catName;

    const groupHeader = document.createElement('div');
    groupHeader.className = 'config-group-header';
    groupHeader.textContent = catName;
    groupWrap.appendChild(groupHeader);

    list.forEach(item => {
      const row = document.createElement('div');
      row.className = 'config-item';
      row.dataset.key = item.key.toLowerCase();
      row.dataset.label = item.label_kr.toLowerCase();
      row.dataset.desc = item.description.toLowerCase();
      row.dataset.category = catName;

      const inputId = `config_field_${item.source}_${item.key.replace(/[^a-zA-Z0-9]/g, '_')}`;
      const defVal = item.default_val !== undefined ? item.default_val : '';
      const defBtnHtml = defVal !== '' ? 
        `<button type="button" class="btn-def-restore" data-target-id="${inputId}" data-default-val="${escapeHtml(defVal)}" title="이 항목을 기본값(${escapeHtml(defVal)})으로 변경">기본: ${escapeHtml(defVal)}</button>` : '';

      // Left info
      const info = document.createElement('div');
      info.className = 'config-info';
      info.innerHTML = `
        <div class="config-title-row">
          <span class="config-label-kr">${escapeHtml(item.label_kr)}</span>
          <span class="config-source-tag">${escapeHtml(item.source)}</span>
          <span class="config-key-name">${escapeHtml(item.key)}</span>
          ${defBtnHtml}
        </div>
        <div class="config-desc">${escapeHtml(item.description)}</div>
      `;

      // Right input control according to type
      const control = document.createElement('div');
      control.className = 'config-control';

      if (item.type === 'bool') {
        const isChecked = (item.current_val.toLowerCase() === 'true');
        control.innerHTML = `
          <label class="checkbox-toggle">
            <input type="checkbox" id="${inputId}" data-source="${item.source}" data-key="${item.key}" data-default-val="${escapeHtml(defVal)}" data-original="${isChecked ? 'true' : 'false'}" ${isChecked ? 'checked' : ''}>
            <span class="checkbox-slider"></span>
          </label>
        `;
      } else if (item.type === 'select') {
        let optHtml = '';
        (item.options || []).forEach(opt => {
          const sel = (item.current_val === opt) ? 'selected' : '';
          optHtml += `<option value="${opt}" ${sel}>${opt}</option>`;
        });
        control.innerHTML = `
          <select class="select" id="${inputId}" data-source="${item.source}" data-key="${item.key}" data-default-val="${escapeHtml(defVal)}" data-original="${escapeHtml(item.current_val)}" style="width: 140px;">
            ${optHtml}
          </select>
        `;
      } else if (item.type === 'number') {
        control.innerHTML = `
          <input type="number" class="input" id="${inputId}" data-source="${item.source}" data-key="${item.key}" data-default-val="${escapeHtml(defVal)}" data-original="${escapeHtml(item.current_val)}" value="${escapeHtml(item.current_val)}" style="width: 110px; text-align: right;">
        `;
      } else {
        // String
        control.innerHTML = `
          <input type="text" class="input" id="${inputId}" data-source="${item.source}" data-key="${item.key}" data-default-val="${escapeHtml(defVal)}" data-original="${escapeHtml(item.current_val)}" value="${escapeHtml(item.current_val)}" style="width: 220px;">
        `;
      }

      row.appendChild(info);
      row.appendChild(control);
      groupWrap.appendChild(row);
    });

    schemaFormContainer.appendChild(groupWrap);
  }
}

function filterSchemaForm() {
  const configSearch = document.getElementById('configSearch');
  const configCategoryFilter = document.getElementById('configCategoryFilter');
  const schemaFormContainer = document.getElementById('schemaFormContainer');
  if (!configSearch || !configCategoryFilter || !schemaFormContainer) return;

  const query = configSearch.value.trim().toLowerCase();
  const selectedCat = configCategoryFilter.value;

  const groups = schemaFormContainer.querySelectorAll('.config-category-group');
  groups.forEach(group => {
    const catName = group.dataset.category;
    let visibleInGroup = 0;

    const items = group.querySelectorAll('.config-item');
    items.forEach(item => {
      const matchCat = (selectedCat === 'all' || catName === selectedCat);
      const matchQuery = !query || 
        item.dataset.key.includes(query) || 
        item.dataset.label.includes(query) || 
        item.dataset.desc.includes(query);

      if (matchCat && matchQuery) {
        item.style.display = 'flex';
        visibleInGroup++;
      } else {
        item.style.display = 'none';
      }
    });

    group.style.display = (visibleInGroup > 0) ? 'block' : 'none';
  });
}

async function saveSchemaConfig() {
  const saveConfigBtn = document.getElementById('saveConfigBtn');
  const schemaFormContainer = document.getElementById('schemaFormContainer');
  if (!schemaFormContainer) return;

  const propUpdates = {};
  const purpurUpdates = {};
  const modifiedInputs = [];

  const inputs = schemaFormContainer.querySelectorAll('input, select');
  inputs.forEach(el => {
    const source = el.dataset.source;
    const key = el.dataset.key;
    if (!source || !key) return;

    let val = '';
    if (el.type === 'checkbox') {
      val = el.checked ? 'true' : 'false';
    } else {
      val = el.value.trim();
    }

    const original = (el.dataset.original !== undefined) ? el.dataset.original.trim() : '';
    if (val === original) {
      return; // 변경되지 않은 항목은 서버 전송 제외
    }

    modifiedInputs.push({ el, val });
    if (source === 'properties') {
      propUpdates[key] = val;
    } else if (source === 'purpur') {
      purpurUpdates[key] = val;
    }
  });

  if (Object.keys(propUpdates).length === 0 && Object.keys(purpurUpdates).length === 0) {
    showToast('변경된 설정 항목이 없습니다.');
    return;
  }

  try {
    if (saveConfigBtn) {
      saveConfigBtn.disabled = true;
      saveConfigBtn.textContent = '저장 중...';
    }

    const res = await fetch('/api/admin/config/save', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        properties: propUpdates,
        purpur: purpurUpdates
      })
    });

    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '저장 실패', 'error');
      return;
    }

    // 성공 시 변경 기준점(original) 갱신
    modifiedInputs.forEach(({ el, val }) => {
      el.dataset.original = val;
    });

    showToast(data.message || '설정이 저장되었습니다.');
  } catch (err) {
    showToast('저장 중 통신 오류가 발생했습니다.', 'error');
  } finally {
    if (saveConfigBtn) {
      saveConfigBtn.disabled = false;
      saveConfigBtn.textContent = '설정 일괄 저장 (자동 백업)';
    }
  }
}

// Purpur Version, Build & Installed Metadata Handling
let installedServerInfo = null;

async function fetchInstalledPurpur() {
  try {
    const res = await fetch('/api/admin/purpur/installed');
    if (!res.ok) return;

    const data = await res.json();
    installedServerInfo = data.installed || null;
    renderInstalledPurpur(installedServerInfo);
  } catch (err) {}
}

function renderInstalledPurpur(info) {
  const jarStatusBadge = document.getElementById('jarStatusBadge');
  const installedPurpurVersionText = document.getElementById('installedPurpurVersionText');
  const installedPurpurBuildText = document.getElementById('installedPurpurBuildText');
  const installedJarFileText = document.getElementById('installedJarFileText');
  const installedPurpurDateText = document.getElementById('installedPurpurDateText');
  const installedVersionHint = document.getElementById('installedVersionHint');

  if (!info || !info.exists) {
    if (jarStatusBadge) {
      jarStatusBadge.className = 'badge badge-disabled';
      jarStatusBadge.textContent = '구동기 없음';
    }
    if (installedPurpurVersionText) installedPurpurVersionText.textContent = '설치된 파일 없음';
    if (installedPurpurBuildText) installedPurpurBuildText.textContent = '-';
    if (installedJarFileText) installedJarFileText.textContent = info?.jar_name || 'server.jar (부재)';
    if (installedPurpurDateText) installedPurpurDateText.textContent = '-';
    if (installedVersionHint) installedVersionHint.textContent = '';
    return;
  }

  const ver = info.version || '미확인';
  const bld = info.build || 'latest';
  const sizeMb = (info.jar_size / (1024 * 1024)).toFixed(1);
  const dlDate = info.downloaded_at ? new Date(info.downloaded_at).toLocaleString() : '확인됨';

  if (jarStatusBadge) {
    jarStatusBadge.className = 'badge badge-enabled';
    jarStatusBadge.textContent = `설치됨 (v${ver} #${bld})`;
  }
  if (installedPurpurVersionText) installedPurpurVersionText.textContent = `Purpur ${ver}`;
  if (installedPurpurBuildText) installedPurpurBuildText.textContent = `Build #${bld}`;
  if (installedJarFileText) installedJarFileText.textContent = `${info.jar_name || 'server.jar'} (${sizeMb} MB)`;
  if (installedPurpurDateText) installedPurpurDateText.textContent = dlDate;
  if (installedVersionHint) installedVersionHint.textContent = `설치됨: v${ver}`;
}

async function fetchPurpurVersions() {
  const purpurVersionSelect = document.getElementById('purpurVersionSelect');
  if (!purpurVersionSelect) return;

  try {
    const res = await fetch('/api/admin/purpur/versions');
    if (!res.ok) return;

    const data = await res.json();
    let versions = data.versions || [];

    // Safety check: ensure descending order (newest version first: 26.3, 1.21.4, ...)
    if (versions.length > 1) {
      const first = versions[0];
      const last = versions[versions.length - 1];
      if (first.localeCompare(last, undefined, { numeric: true }) < 0) {
        versions = versions.slice().reverse();
      }
    }

    purpurVersionSelect.innerHTML = '';

    versions.forEach(v => {
      const opt = document.createElement('option');
      opt.value = v;
      opt.textContent = v;
      purpurVersionSelect.appendChild(opt);
    });

    if (versions.length === 0) return;

    let targetVersion = versions[0];
    let preferredBuild = null;

    if (installedServerInfo && installedServerInfo.version && versions.includes(installedServerInfo.version)) {
      targetVersion = installedServerInfo.version;
      preferredBuild = installedServerInfo.build;
    }

    purpurVersionSelect.value = targetVersion;
    fetchPurpurBuilds(targetVersion, preferredBuild);
  } catch (err) {}
}

async function fetchPurpurBuilds(version, preferredBuild = null) {
  const purpurBuildSelect = document.getElementById('purpurBuildSelect');
  if (!purpurBuildSelect || !version) return;

  try {
    purpurBuildSelect.innerHTML = '<option value="latest">최신 빌드 (Latest)</option>';
    const res = await fetch(`/api/admin/purpur/builds?version=${encodeURIComponent(version)}`);
    if (!res.ok) return;

    const data = await res.json();
    const builds = data.builds || [];

    builds.slice().reverse().forEach(b => {
      const opt = document.createElement('option');
      opt.value = b;
      opt.textContent = `Build #${b}`;
      purpurBuildSelect.appendChild(opt);
    });

    if (preferredBuild && preferredBuild !== 'latest' && builds.includes(preferredBuild)) {
      purpurBuildSelect.value = preferredBuild;
    } else {
      purpurBuildSelect.value = 'latest';
    }
  } catch (err) {}
}

function pollDownloadProgress() {
  if (downloadPollTimer) clearInterval(downloadPollTimer);

  const dlProgressBar = document.getElementById('dlProgressBar');
  const dlPercentText = document.getElementById('dlPercentText');
  const dlStatusText = document.getElementById('dlStatusText');
  const btnDownloadPurpur = document.getElementById('btnDownloadPurpur');

  downloadPollTimer = setInterval(async () => {
    try {
      const res = await fetch('/api/admin/purpur/download/status');
      if (!res.ok) return;

      const data = await res.json();
      const pct = typeof data.progress === 'number' ? data.progress : 0;
      const isBusy = data.is_busy ?? data.in_progress ?? false;
      const statusText = data.status_text || data.message || '다운로드 중...';

      if (dlProgressBar) dlProgressBar.style.width = `${pct}%`;
      if (dlPercentText) dlPercentText.textContent = `${pct}%`;
      if (dlStatusText) dlStatusText.textContent = statusText;

      // When download finishes
      if (!isBusy) {
        clearInterval(downloadPollTimer);
        downloadPollTimer = null;
        if (btnDownloadPurpur) btnDownloadPurpur.disabled = false;
        if (dlProgressBar) dlProgressBar.style.width = '100%';
        if (dlPercentText) dlPercentText.textContent = '100%';
        if (dlStatusText) dlStatusText.textContent = statusText || '다운로드 및 설치 완료';

        await fetchInstalledPurpur();
        if (typeof fetchProcessStatus === 'function') fetchProcessStatus();
        showToast('구동기 다운로드 및 설치가 완료되었습니다!');
      }
    } catch (err) {
      console.warn('[Downloader] polling error:', err);
    }
  }, 500);
}

// Plugin List & Soft Toggle Handling
async function fetchPlugins() {
  const pluginsTableBody = document.getElementById('pluginsTableBody');
  if (!pluginsTableBody) return;

  try {
    const res = await fetch('/api/admin/plugins');
    if (!res.ok) return;

    const data = await res.json();
    pluginsTableBody.innerHTML = '';

    if (!data.plugins || data.plugins.length === 0) {
      pluginsTableBody.innerHTML = '<tr><td colspan="4" style="text-align: center; padding: 24px; color: var(--text-muted);">설치된 플러그인이 없습니다.</td></tr>';
      return;
    }

    data.plugins.forEach(p => {
      const tr = document.createElement('tr');
      const sizeKb = (p.size / 1024).toFixed(1);
      const isEnabled = p.enabled !== false;
      const statusBadge = isEnabled
        ? `<span class="badge badge-enabled">활성</span>`
        : `<span class="badge badge-disabled">비활성</span>`;
      const toggleText = isEnabled ? '비활성화' : '활성화';
      const toggleClass = isEnabled ? 'btn-sm' : 'btn-primary btn-sm';

      tr.innerHTML = `
        <td style="font-weight: 500;">${escapeHtml(p.name)}</td>
        <td style="font-family: var(--font-mono); color: var(--text-secondary);">${sizeKb} KB</td>
        <td>${statusBadge}</td>
        <td style="display: flex; gap: 6px; justify-content: flex-end;">
          <button class="btn ${toggleClass}" onclick="togglePlugin('${escapeHtml(p.name)}')">${toggleText}</button>
          <button class="btn btn-danger btn-sm" onclick="deletePlugin('${escapeHtml(p.name)}')">삭제</button>
        </td>
      `;
      pluginsTableBody.appendChild(tr);
    });
  } catch (err) {}
}

async function uploadPluginFiles(files) {
  if (!files || files.length === 0) return;
  const jarFiles = Array.from(files).filter(f => f.name.toLowerCase().endsWith('.jar'));
  if (jarFiles.length === 0) {
    showToast('.jar 형식의 플러그인 파일만 업로드할 수 있습니다.', 'error');
    return;
  }

  showToast(`${jarFiles.length}개의 플러그인 업로드 중...`);
  let successCount = 0;
  for (const file of jarFiles) {
    const formData = new FormData();
    formData.append('plugin', file);
    try {
      const res = await fetch('/api/admin/plugins/upload', {
        method: 'POST',
        body: formData
      });
      if (res.ok) successCount++;
    } catch (err) {}
  }

  if (successCount > 0) {
    showToast(`${successCount}개 플러그인이 성공적으로 업로드되었습니다.`);
    fetchPlugins();
  } else {
    showToast('플러그인 업로드에 실패했습니다.', 'error');
  }
}

async function togglePlugin(name) {
  try {
    const res = await fetch(`/api/admin/plugins/toggle/${encodeURIComponent(name)}`, { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '플러그인 상태 변경 실패', 'error');
      return;
    }
    showToast(data.message || '플러그인 상태가 변경되었습니다.');
    fetchPlugins();
  } catch (err) {
    showToast('통신 오류가 발생했습니다.', 'error');
  }
}

async function deletePlugin(name) {
  if (!confirm(`정말로 플러그인 '${name}'을(를) 삭제하시겠습니까?`)) return;

  try {
    const res = await fetch(`/api/admin/plugins/${encodeURIComponent(name)}`, { method: 'DELETE' });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '삭제 실패', 'error');
      return;
    }
    showToast('플러그인이 삭제되었습니다.');
    fetchPlugins();
  } catch (err) {
    showToast('삭제 중 오류가 발생했습니다.', 'error');
  }
}

// World Backups Management
async function fetchBackups() {
  const tbody = document.getElementById('backupsTableBody');
  if (!tbody) return;

  try {
    const res = await fetch('/api/admin/backups');
    if (!res.ok) return;

    const data = await res.json();
    tbody.innerHTML = '';

    const list = data.backups || [];
    if (list.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-muted); padding: 24px;">생성된 백업 파일이 없습니다.</td></tr>';
      return;
    }

    list.forEach(b => {
      const tr = document.createElement('tr');
      const fname = b.file_name || b.filename || '';
      const sizeMb = (typeof b.size_mb === 'number') ? b.size_mb.toFixed(2) : ((b.size || 0) / (1024 * 1024)).toFixed(2);
      const timeStr = b.created_at ? b.created_at.replace('T', ' ').split('.')[0] : '-';
      tr.innerHTML = `
        <td>
          <div style="font-weight: 500;">${escapeHtml(fname)}</div>
          ${b.description ? `<div style="font-size: 0.75rem; color: var(--text-muted);">${escapeHtml(b.description)}</div>` : ''}
        </td>
        <td style="font-family: var(--font-mono); color: var(--text-secondary);">${sizeMb} MB</td>
        <td style="font-size: 0.8rem; color: var(--text-muted);">${timeStr}</td>
        <td style="text-align: right; display: flex; gap: 6px; justify-content: flex-end;">
          <a href="/api/admin/backups/download/${encodeURIComponent(fname)}" class="btn btn-sm" download>다운로드</a>
          <button class="btn btn-danger btn-sm" onclick="deleteBackup('${escapeHtml(fname)}')">삭제</button>
        </td>
      `;
      tbody.appendChild(tr);
    });
  } catch (err) {}
}

async function handleCreateBackup() {
  const descInput = document.getElementById('backupDescInput');
  const desc = descInput ? descInput.value.trim() : '';

  showToast('월드 데이터 저장 및 압축 백업 중입니다...');
  try {
    const res = await fetch('/api/admin/backups/create', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ description: desc })
    });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '백업 생성 실패', 'error');
      return;
    }
    showToast('월드 백업이 완료되었습니다!');
    if (descInput) descInput.value = '';
    fetchBackups();
  } catch (err) {
    showToast('백업 요청 중 오류가 발생했습니다.', 'error');
  }
}

async function deleteBackup(filename) {
  if (!confirm(`정말로 백업 파일 '${filename}'을(를) 영구 삭제하시겠습니까?`)) return;

  try {
    const res = await fetch(`/api/admin/backups/${encodeURIComponent(filename)}`, { method: 'DELETE' });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '백업 삭제 실패', 'error');
      return;
    }
    showToast('백업 파일이 삭제되었습니다.');
    fetchBackups();
  } catch (err) {
    showToast('삭제 중 오류가 발생했습니다.', 'error');
  }
}

// Window attachments for dynamic onclicks
window.togglePlugin = togglePlugin;
window.deletePlugin = deletePlugin;
window.deleteBackup = deleteBackup;

function initSettings() {
  // Config Filter & Save
  const configSearch = document.getElementById('configSearch');
  const configCategoryFilter = document.getElementById('configCategoryFilter');
  const saveConfigBtn = document.getElementById('saveConfigBtn');
  const btnResetDefaults = document.getElementById('btnResetDefaults');
  const schemaFormContainer = document.getElementById('schemaFormContainer');

  if (configSearch) configSearch.addEventListener('input', filterSchemaForm);
  if (configCategoryFilter) configCategoryFilter.addEventListener('change', filterSchemaForm);
  if (saveConfigBtn) saveConfigBtn.addEventListener('click', saveSchemaConfig);

  if (btnResetDefaults && schemaFormContainer) {
    btnResetDefaults.addEventListener('click', () => {
      if (!confirm('모든 설정값을 시스템 권장 기본값으로 되돌리시겠습니까? (저장 버튼을 눌러야 반영됩니다)')) return;
      const inputs = schemaFormContainer.querySelectorAll('input, select');
      let restoredCount = 0;
      inputs.forEach(el => {
        const defVal = el.dataset.defaultVal;
        if (defVal === undefined || defVal === '') return;
        if (el.type === 'checkbox') {
          el.checked = (defVal.toLowerCase() === 'true');
        } else {
          el.value = defVal;
        }
        restoredCount++;
      });
      showToast(`${restoredCount}개 설정 항목을 기본값으로 복원했습니다. '설정 일괄 저장'을 눌러 적용하세요.`);
    });
  }

  // Individual default restore buttons via delegation
  if (schemaFormContainer) {
    schemaFormContainer.addEventListener('click', (e) => {
      const btn = e.target.closest('.btn-def-restore');
      if (!btn) return;
      const targetId = btn.dataset.targetId;
      const defVal = btn.dataset.defaultVal;
      const el = document.getElementById(targetId);
      if (!el || defVal === undefined) return;

      if (el.type === 'checkbox') {
        el.checked = (defVal.toLowerCase() === 'true');
      } else {
        el.value = defVal;
      }
      showToast(`기본값(${defVal})으로 설정되었습니다.`);
    });
  }

  // Purpur Version & Installer
  const purpurVersionSelect = document.getElementById('purpurVersionSelect');
  const purpurBuildSelect = document.getElementById('purpurBuildSelect');
  const installerForm = document.getElementById('installerForm');
  const btnDownloadPurpur = document.getElementById('btnDownloadPurpur');
  const downloadProgressArea = document.getElementById('downloadProgressArea');
  const dlProgressBar = document.getElementById('dlProgressBar');
  const dlPercentText = document.getElementById('dlPercentText');
  const dlStatusText = document.getElementById('dlStatusText');
  const btnRefreshInstallerInfo = document.getElementById('btnRefreshInstallerInfo');

  if (purpurVersionSelect) {
    purpurVersionSelect.addEventListener('change', (e) => {
      fetchPurpurBuilds(e.target.value);
    });
  }

  if (installerForm && purpurVersionSelect && purpurBuildSelect) {
    installerForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const version = purpurVersionSelect.value;
      const build = purpurBuildSelect.value;
      if (!version) return;

      try {
        if (btnDownloadPurpur) btnDownloadPurpur.disabled = true;
        if (downloadProgressArea) downloadProgressArea.style.display = 'block';
        if (dlProgressBar) dlProgressBar.style.width = '0%';
        if (dlPercentText) dlPercentText.textContent = '0%';
        if (dlStatusText) dlStatusText.textContent = '다운로드 준비 중...';

        const res = await fetch('/api/admin/purpur/download', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ version, build })
        });

        const data = await res.json();
        if (!res.ok) {
          showToast(data.error || '다운로드 요청 실패', 'error');
          if (btnDownloadPurpur) btnDownloadPurpur.disabled = false;
          return;
        }

        showToast('구동기 다운로드가 시작되었습니다.');
        pollDownloadProgress();
      } catch (err) {
        showToast('통신 오류가 발생했습니다.', 'error');
        if (btnDownloadPurpur) btnDownloadPurpur.disabled = false;
      }
    });
  }

  if (btnRefreshInstallerInfo) {
    btnRefreshInstallerInfo.addEventListener('click', () => {
      fetchInstalledPurpur();
      fetchPurpurVersions();
    });
  }

  // Plugins Refresh & Drag-and-drop
  const btnRefreshPlugins = document.getElementById('btnRefreshPlugins');
  if (btnRefreshPlugins) {
    btnRefreshPlugins.addEventListener('click', fetchPlugins);
  }

  const pluginDropzone = document.getElementById('pluginDropzone');
  const pluginFileInput = document.getElementById('pluginFileInput');
  if (pluginDropzone) {
    pluginDropzone.addEventListener('click', () => {
      if (pluginFileInput) pluginFileInput.click();
    });

    ['dragenter', 'dragover'].forEach(eventName => {
      pluginDropzone.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        pluginDropzone.classList.add('dragover');
      });
    });

    ['dragleave', 'drop'].forEach(eventName => {
      pluginDropzone.addEventListener(eventName, (e) => {
        e.preventDefault();
        e.stopPropagation();
        pluginDropzone.classList.remove('dragover');
      });
    });

    pluginDropzone.addEventListener('drop', (e) => {
      const dt = e.dataTransfer;
      if (dt && dt.files && dt.files.length > 0) {
        uploadPluginFiles(dt.files);
      }
    });
  }

  if (pluginFileInput) {
    pluginFileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files.length > 0) {
        uploadPluginFiles(e.target.files);
        e.target.value = '';
      }
    });
  }

  const pluginUrlForm = document.getElementById('pluginUrlForm');
  const pluginUrlInput = document.getElementById('pluginUrlInput');
  if (pluginUrlForm && pluginUrlInput) {
    pluginUrlForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const url = pluginUrlInput.value.trim();
      if (!url) return;

      showToast('원격 URL에서 플러그인 다운로드 중...');
      try {
        const res = await fetch('/api/admin/plugins/download', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: url })
        });
        const data = await res.json();
        if (!res.ok) {
          showToast(data.error || '플러그인 다운로드 실패', 'error');
          return;
        }
        showToast('플러그인 다운로드 완료!');
        pluginUrlInput.value = '';
        fetchPlugins();
      } catch (err) {
        showToast('다운로드 중 통신 오류가 발생했습니다.', 'error');
      }
    });
  }

  const btnAutoInstallSquaremap = document.getElementById('btnAutoInstallSquaremap') || document.getElementById('btnAutoInstallBlueMap2');
  if (btnAutoInstallSquaremap) {
    btnAutoInstallSquaremap.addEventListener('click', async () => {
      if (!currentUser || currentUser.role !== 'admin') return;
      showToast('서버 버전에 맞는 squaremap 자동 다운로드 및 설정 적용 중...');
      try {
        const res = await fetch('/api/admin/squaremap/auto-install', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) {
          showToast(data.error || 'squaremap 자동 설정 실패', 'error');
          return;
        }
        showToast('squaremap 자동 다운로드 및 설정 완료!');
        fetchPlugins();
      } catch (err) {
        showToast('서버 통신 실패', 'error');
      }
    });
  }

  // Backups
  const btnCreateBackup = document.getElementById('btnCreateBackup');
  if (btnCreateBackup) {
    btnCreateBackup.addEventListener('click', handleCreateBackup);
  }

  const btnRefreshBackups = document.getElementById('btnRefreshBackups');
  if (btnRefreshBackups) {
    btnRefreshBackups.addEventListener('click', fetchBackups);
  }

  // Datapacks & Experiments
  const btnApplyExperiments = document.getElementById('btnApplyExperiments');
  if (btnApplyExperiments) {
    btnApplyExperiments.addEventListener('click', handleApplyExperiments);
  }

  const btnRefreshDatapacks = document.getElementById('btnRefreshDatapacks');
  if (btnRefreshDatapacks) {
    btnRefreshDatapacks.addEventListener('click', fetchDatapacks);
  }

  // Zero-Downtime System Update
  const btnCheckUpdate = document.getElementById('btnCheckUpdate');
  if (btnCheckUpdate) {
    btnCheckUpdate.addEventListener('click', () => checkSystemUpdate(true));
  }

  const btnApplyUpdate = document.getElementById('btnApplyUpdate');
  if (btnApplyUpdate) {
    btnApplyUpdate.addEventListener('click', handleApplySystemUpdate);
  }
}

// Datapacks & Experimental Gameplay Module
async function fetchDatapacks() {
  if (!currentUser || currentUser.role !== 'admin') return;
  const container = document.getElementById('experimentsListContainer');
  const enabledList = document.getElementById('enabledPacksList');
  const disabledList = document.getElementById('disabledPacksList');
  const liveBadge = document.getElementById('datapackLiveBadge');
  const rconBox = document.getElementById('datapackRconBox');
  const rconText = document.getElementById('datapackRconText');

  try {
    const res = await fetch('/api/admin/datapacks');
    if (!res.ok) {
      if (container) container.innerHTML = '<div style="color: var(--accent); padding: 16px;">데이터팩 상태 조회 실패</div>';
      return;
    }

    const data = await res.json();

    if (liveBadge) {
      const count = (data.enabled_packs || []).length;
      liveBadge.textContent = `${count}개 활성화됨`;
      liveBadge.className = 'badge badge-enabled';
    }

    // 1. Render Experiments Checklist
    if (container) {
      container.innerHTML = '';
      if (!data.experiments || data.experiments.length === 0) {
        container.innerHTML = '<div style="color: var(--text-muted); padding: 16px;">사용 가능한 실험 기능이 없습니다.</div>';
      } else {
        data.experiments.forEach(exp => {
          const card = document.createElement('label');
          card.style.display = 'flex';
          card.style.alignItems = 'flex-start';
          card.style.gap = '12px';
          card.style.padding = '14px 16px';
          card.style.background = exp.enabled ? 'rgba(74, 222, 128, 0.05)' : 'rgba(255, 255, 255, 0.02)';
          card.style.border = exp.enabled ? '1px solid rgba(74, 222, 128, 0.3)' : '1px solid var(--border-subtle)';
          card.style.borderRadius = 'var(--radius-sm)';
          card.style.cursor = 'pointer';
          card.style.transition = 'all 0.2s ease';

          card.innerHTML = `
            <input type="checkbox" class="exp-checkbox" value="${escapeHtml(exp.id)}" ${exp.enabled ? 'checked' : ''} style="margin-top: 3px; accent-color: var(--accent); width: 16px; height: 16px;">
            <div style="flex: 1;">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                <span style="font-size: 0.875rem; font-weight: 600; color: #fff;">${escapeHtml(exp.name)}</span>
                <span class="badge ${exp.enabled ? 'badge-enabled' : 'badge-disabled'}" style="font-size: 0.7rem;">${exp.enabled ? '적용됨' : '미적용'}</span>
              </div>
              <div style="font-size: 0.775rem; color: var(--text-secondary); line-height: 1.4;">${escapeHtml(exp.description)}</div>
              <div style="font-size: 0.7rem; color: var(--text-muted); font-family: var(--font-mono); margin-top: 4px;">ID: ${escapeHtml(exp.id)}</div>
            </div>
          `;
          container.appendChild(card);
        });
      }
    }

    // 2. Render Enabled Packs List
    if (enabledList) {
      enabledList.innerHTML = '';
      if (!data.enabled_packs || data.enabled_packs.length === 0) {
        enabledList.innerHTML = '<span style="color: var(--text-muted); font-size: 0.8rem;">활성화된 팩 없음</span>';
      } else {
        data.enabled_packs.forEach(p => {
          const badge = document.createElement('span');
          badge.className = 'badge';
          badge.style.background = 'rgba(74, 222, 128, 0.15)';
          badge.style.color = '#4ade80';
          badge.style.border = '1px solid rgba(74, 222, 128, 0.3)';
          badge.style.padding = '4px 10px';
          badge.style.fontFamily = 'var(--font-mono)';
          badge.style.fontSize = '0.775rem';
          badge.textContent = p;
          enabledList.appendChild(badge);
        });
      }
    }

    // 3. Render Disabled Packs List
    if (disabledList) {
      disabledList.innerHTML = '';
      if (!data.disabled_packs || data.disabled_packs.length === 0) {
        disabledList.innerHTML = '<span style="color: var(--text-muted); font-size: 0.8rem;">비활성화된 팩 없음</span>';
      } else {
        data.disabled_packs.forEach(p => {
          const badge = document.createElement('span');
          badge.className = 'badge';
          badge.style.background = 'rgba(255, 255, 255, 0.05)';
          badge.style.color = 'var(--text-muted)';
          badge.style.border = '1px solid var(--border-subtle)';
          badge.style.padding = '4px 10px';
          badge.style.fontFamily = 'var(--font-mono)';
          badge.style.fontSize = '0.775rem';
          badge.textContent = p;
          disabledList.appendChild(badge);
        });
      }
    }

    // 4. Render RCON Output if available
    if (rconBox && rconText) {
      if (data.rcon_output && data.rcon_output.trim() !== '') {
        rconText.textContent = data.rcon_output;
        rconBox.style.display = 'block';
      } else {
        rconBox.style.display = 'none';
      }
    }

  } catch (err) {
    if (container) container.innerHTML = '<div style="color: var(--accent); padding: 16px;">통신 오류가 발생했습니다.</div>';
  }
}

async function handleApplyExperiments() {
  if (!currentUser || currentUser.role !== 'admin') return;
  const checkboxes = document.querySelectorAll('.exp-checkbox:checked');
  const selected = Array.from(checkboxes).map(cb => cb.value);

  const btn = document.getElementById('btnApplyExperiments');
  if (btn) {
    btn.disabled = true;
    btn.textContent = '적용 중...';
  }

  showToast('실험적 기능(Experimental Gameplay)을 월드 및 서버에 적용 중...');

  try {
    const res = await fetch('/api/admin/datapacks/experiments', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ experiments: selected })
    });

    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '실험적 기능 적용 실패', 'error');
      return;
    }

    showToast(data.message || '실험적 기능이 성공적으로 적용되었습니다!');
    await fetchDatapacks();
  } catch (err) {
    showToast('서버 통신 실패', 'error');
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = '선택한 실험 기능 적용하기';
    }
  }
}

// Zero-Downtime System Update Module
let updateCheckData = null;

async function checkSystemUpdate(manual = false) {
  if (!currentUser || currentUser.role !== 'admin') return;

  const btnCheck = document.getElementById('btnCheckUpdate');
  const btnApply = document.getElementById('btnApplyUpdate');
  const badge = document.getElementById('updateStatusBadge');
  const detailsCard = document.getElementById('updateDetailsCard');
  const currentHashEl = document.getElementById('currentCommitHash');
  const branchEl = document.getElementById('currentBranchName');
  const checkTimeEl = document.getElementById('lastCheckTime');

  if (currentHashEl && (!currentHashEl.textContent || currentHashEl.textContent === '-')) currentHashEl.textContent = '조회 중...';
  if (branchEl && (!branchEl.textContent || branchEl.textContent === '-')) branchEl.textContent = '조회 중...';

  if (btnCheck) {
    btnCheck.disabled = true;
    btnCheck.innerHTML = '<span>⏳</span> 확인 중...';
  }

  try {
    const res = await fetch('/api/admin/system/update/check');
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || '업데이트 확인 실패');
    }
    const data = await res.json();
    updateCheckData = data;

    if (currentHashEl) currentHashEl.textContent = data.current_commit;
    if (branchEl) branchEl.textContent = data.branch;
    if (checkTimeEl) checkTimeEl.textContent = data.checked_at;

    if (data.has_update) {
      if (badge) {
        badge.className = 'badge badge-warning';
        badge.textContent = `신규 업데이트 발견 (${data.pending_commits.length}개 커밋)`;
      }
      if (btnCheck) {
        btnCheck.style.display = 'none';
      }
      if (btnApply) {
        btnApply.style.display = 'inline-flex';
        btnApply.innerHTML = `<span>🚀</span> 지금 업데이트 적용 (${data.latest_commit})`;
      }
      if (detailsCard) {
        detailsCard.style.display = 'block';
        const msgEl = document.getElementById('updateLatestMessage');
        const listEl = document.getElementById('pendingCommitsList');
        if (msgEl) msgEl.textContent = data.latest_message;
        if (listEl) {
          listEl.innerHTML = data.pending_commits.map(c => `<li style="padding: 6px 10px; background: rgba(0,0,0,0.25); border-radius: 4px; border: 1px solid rgba(255,255,255,0.05); font-family: monospace; font-size: 0.85rem; color: #e2e8f0;">${escapeHtml(c)}</li>`).join('');
        }
      }
      if (manual) {
        showToast(`새로운 업데이트가 발견되었습니다! (${data.pending_commits.length}개 커밋)`);
      }
    } else {
      if (badge) {
        badge.className = 'badge badge-enabled';
        badge.textContent = '최신 버전입니다';
      }
      if (btnCheck) {
        btnCheck.style.display = 'inline-flex';
        btnCheck.innerHTML = '<span>🔍</span> 업데이트 확인';
      }
      if (btnApply) {
        btnApply.style.display = 'none';
      }
      if (detailsCard) {
        detailsCard.style.display = 'none';
      }
      if (manual) {
        showToast('현재 최신 버전입니다.');
      }
    }
  } catch (err) {
    if (manual) showToast(err.message, 'error');
  } finally {
    if (btnCheck) {
      btnCheck.disabled = false;
      if (!updateCheckData || !updateCheckData.has_update) {
        btnCheck.innerHTML = '<span>🔍</span> 업데이트 확인';
      }
    }
  }
}

async function handleApplySystemUpdate() {
  if (!currentUser || currentUser.role !== 'admin') return;

  const confirmed = confirm(
    '실행 중인 마인크래프트 서버는 중단 없이 그대로 유지됩니다!\n\n' +
    'git pull 및 새 바이너리 컴파일 후 웹 매니저가 무중단 재기동됩니다. 계속 진행하시겠습니까?'
  );
  if (!confirmed) return;

  const btnApply = document.getElementById('btnApplyUpdate');
  if (btnApply) {
    btnApply.disabled = true;
    btnApply.innerHTML = '<span>⏳</span> 업데이트 빌드 중...';
  }

  showToast('무중단 업데이트 적용 시작: Git Pull 및 컴파일 진행 중...', 'info');

  try {
    const res = await fetch('/api/admin/system/update/apply', { method: 'POST' });
    const data = await res.json();
    if (!res.ok) {
      showToast(data.error || '업데이트 적용 실패', 'error');
      if (btnApply) {
        btnApply.disabled = false;
        btnApply.innerHTML = '<span>🚀</span> 업데이트 재시도';
      }
      return;
    }

    showToast(data.message, 'success');

    // Show reloading modal overlay
    const overlay = document.createElement('div');
    overlay.id = 'updateReloadOverlay';
    overlay.style.cssText = 'position: fixed; inset: 0; background: rgba(0,0,0,0.85); z-index: 99999; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; color: #fff; text-align: center; backdrop-filter: blur(8px);';
    overlay.innerHTML = `
      <div class="spinner" style="width: 48px; height: 48px; border: 4px solid rgba(255,255,255,0.2); border-top-color: #8b5cf6; border-radius: 50%; animation: spin 0.8s linear infinite;"></div>
      <h2 style="font-size: 1.5rem; font-weight: 700; margin: 0;">웹 매니저 무중단 재기동 중</h2>
      <p style="color: var(--text-secondary); max-width: 420px; margin: 0; font-size: 0.95rem;">
        마인크래프트 서버는 중단 없이 계속 실행 중입니다.<br>새 매니저가 기동되면 자동으로 연결을 복구합니다...
      </p>
      <div id="reconnectAttempts" style="font-size: 0.8rem; color: var(--text-muted);">재연결 확인 중...</div>
    `;
    document.body.appendChild(overlay);

    // Poll until the server comes back up
    let attempts = 0;
    const interval = setInterval(async () => {
      attempts++;
      const attemptEl = document.getElementById('reconnectAttempts');
      if (attemptEl) attemptEl.textContent = `재연결 시도 중 (${attempts}회)...`;

      try {
        const pingRes = await fetch('/api/public/status', { cache: 'no-store' });
        if (pingRes.ok) {
          clearInterval(interval);
          if (attemptEl) attemptEl.textContent = '연결 복원 완료! 새로고침합니다...';
          setTimeout(() => {
            window.location.reload();
          }, 1000);
        }
      } catch (e) {
        // still reloading
      }

      if (attempts > 30) {
        clearInterval(interval);
        if (attemptEl) attemptEl.textContent = '재연결 시간이 초과되었습니다. 페이지를 새로고침해주세요.';
      }
    }, 1500);

  } catch (err) {
    showToast('업데이트 요청 중 오류가 발생했습니다: ' + err.message, 'error');
    if (btnApply) {
      btnApply.disabled = false;
      btnApply.innerHTML = '<span>🚀</span> 업데이트 재시도';
    }
  }
}

// Window attachments for navigation subtab switcher
window.checkSystemUpdate = checkSystemUpdate;
window.handleApplySystemUpdate = handleApplySystemUpdate;

