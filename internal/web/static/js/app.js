// Application Orchestrator Entry Point

document.addEventListener('DOMContentLoaded', () => {
  // 1. Initialize navigation and routing
  if (typeof initNav === 'function') initNav();

  // 2. Initialize authentication & user session listeners
  if (typeof initAuth === 'function') initAuth();

  // 3. Initialize dashboard controls (process, chat, players, world env, map)
  if (typeof initDashboard === 'function') initDashboard();

  // 4. Initialize whitelist management & self-registration
  if (typeof initWhitelist === 'function') initWhitelist();

  // 5. Initialize server console, autocompletion & telemetry chart
  if (typeof initConsole === 'function') initConsole();

  // 6. Initialize server settings (schema editor, installer, plugins, backups)
  if (typeof initSettings === 'function') initSettings();

  // 7. Verify authentication & boot application state
  if (typeof checkAuth === 'function') checkAuth();
});
