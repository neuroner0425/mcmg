// Navigation & Route Management Module

// Subtab Switcher for Consolidated Server Settings
function switchSubTab(subId) {
  const items = document.querySelectorAll('.sub-nav-item');
  items.forEach(item => {
    if (item.dataset.sub === subId) {
      item.classList.add('active');
    } else {
      item.classList.remove('active');
    }
  });

  const panes = document.querySelectorAll('.sub-pane');
  panes.forEach(pane => {
    if (pane.id === subId) {
      pane.classList.add('active');
    } else {
      pane.classList.remove('active');
    }
  });

  // Fetch relevant data on subtab switch
  if (subId === 'subConfig') {
    if (typeof fetchConfigSchema === 'function') fetchConfigSchema();
  } else if (subId === 'subPlugins') {
    if (typeof fetchPlugins === 'function') fetchPlugins();
  } else if (subId === 'subInstaller') {
    if (typeof fetchInstalledPurpur === 'function' && typeof fetchPurpurVersions === 'function') {
      fetchInstalledPurpur().then(() => fetchPurpurVersions());
    }
  } else if (subId === 'subBackups') {
    if (typeof fetchBackups === 'function') fetchBackups();
  } else if (subId === 'subDatapacks') {
    if (typeof fetchDatapacks === 'function') fetchDatapacks();
  } else if (subId === 'subUpdate') {
    if (typeof checkSystemUpdate === 'function') checkSystemUpdate(false);
  }
}

// Route Mapping & Permission Definitions (6 Core Pages + Backward Compatibility)
const ROUTE_MAP = {
  '/': { target: 'dashboardPane', path: '/dashboard', adminOnly: false },
  '/dashboard': { target: 'dashboardPane', path: '/dashboard', adminOnly: false },
  '/map': { target: 'mapPane', path: '/map', adminOnly: false },
  '/whitelist': { target: 'whitelistPane', path: '/whitelist', adminOnly: false },
  '/admin/metrics': { target: 'metricsPane', path: '/admin/metrics', adminOnly: true },
  '/admin/console': { target: 'consolePane', path: '/admin/console', adminOnly: true },
  '/admin/settings': { target: 'settingsPane', path: '/admin/settings', sub: 'subConfig', adminOnly: true },

  // Compatibility aliases
  '/admin/whitelist': { target: 'whitelistPane', path: '/whitelist', adminOnly: false },
  '/admin/config': { target: 'settingsPane', path: '/admin/settings', sub: 'subConfig', adminOnly: true },
  '/admin/datapacks': { target: 'settingsPane', path: '/admin/settings', sub: 'subDatapacks', adminOnly: true },
  '/admin/plugins': { target: 'settingsPane', path: '/admin/settings', sub: 'subPlugins', adminOnly: true },
  '/admin/installer': { target: 'settingsPane', path: '/admin/settings', sub: 'subInstaller', adminOnly: true },
  '/admin/backups': { target: 'settingsPane', path: '/admin/settings', sub: 'subBackups', adminOnly: true },
  '/admin/update': { target: 'settingsPane', path: '/admin/settings', sub: 'subUpdate', adminOnly: true }
};

function navigateByPath(pathname, push = false) {
  const cleanPath = pathname.endsWith('/') && pathname !== '/' ? pathname.slice(0, -1) : pathname;
  const route = ROUTE_MAP[cleanPath] || ROUTE_MAP['/dashboard'];

  // Permission Restriction: check admin access
  if (route.adminOnly && (!currentUser || currentUser.role !== 'admin')) {
    showToast('관리자 전용 페이지입니다. 접근이 제한되었습니다.', 'error');
    navigateByPath('/dashboard', true);
    return;
  }

  if (push && window.location.pathname !== route.path) {
    history.pushState(null, '', route.path);
  }

  // Active Sidebar Tab Button
  const mainTabs = document.querySelectorAll('.nav-item');
  mainTabs.forEach(b => {
    if (b.dataset.target === route.target || b.dataset.path === route.path) {
      b.classList.add('active');
    } else {
      b.classList.remove('active');
    }
  });

  // Active Main Pane
  document.querySelectorAll('.tab-pane').forEach(p => p.classList.remove('active'));
  const targetPane = document.getElementById(route.target);
  if (targetPane) {
    targetPane.classList.add('active');
  }

  // Activate Settings Subtab if target is settingsPane
  if (route.target === 'settingsPane') {
    if (route.sub) {
      switchSubTab(route.sub);
    } else {
      const activeSub = document.querySelector('.sub-pane.active');
      const activeId = activeSub ? activeSub.id : 'subConfig';
      switchSubTab(activeId);
    }
  }

  // Trigger relevant page data fetches
  if (route.target === 'consolePane') {
    if (typeof fetchLogs === 'function') fetchLogs();
    const consoleOutput = document.getElementById('consoleOutput');
    if (consoleOutput) consoleOutput.scrollTop = consoleOutput.scrollHeight;
  } else if (route.target === 'metricsPane') {
    if (typeof fetchMetrics === 'function') fetchMetrics();
    if (typeof renderMetricsChart === 'function') {
      requestAnimationFrame(() => renderMetricsChart());
    }
  } else if (route.target === 'whitelistPane') {
    if (currentUser && currentUser.role === 'admin' && typeof fetchWhitelist === 'function') {
      fetchWhitelist();
    }
  } else if (route.target === 'dashboardPane' || route.target === 'mapPane') {
    if (typeof fetchProcessStatus === 'function') {
      fetchProcessStatus();
    }
  }
}

// Navigation Tabs Setup
function initNav() {
  const mainTabs = document.querySelectorAll('.nav-item');
  mainTabs.forEach(btn => {
    btn.addEventListener('click', () => {
      const path = btn.dataset.path || '/dashboard';
      navigateByPath(path, true);
    });
  });

  // Settings Sub-tab Navigation
  const subNavItems = document.querySelectorAll('.sub-nav-item');
  subNavItems.forEach(btn => {
    btn.addEventListener('click', () => {
      const subTarget = btn.dataset.sub;
      switchSubTab(subTarget);
    });
  });

  window.addEventListener('popstate', () => {
    navigateByPath(window.location.pathname, false);
  });
}
