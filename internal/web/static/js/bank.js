// ===== RESOLVER BANK =====
var currentResolverTab = 'active';
function updateResolversBadge(count, bankCount) {
  var badge = document.getElementById('resolversBadge');
  if (!badge) return;
  badge.textContent = count + ' / ' + (bankCount !== undefined ? bankCount : count);
  var total = bankCount !== undefined ? bankCount : count;
  if (total > 500) {
    badge.style.color = 'var(--error, #e74c3c)';
  } else {
    badge.style.color = count > 0 ? 'var(--success, #27ae60)' : 'var(--error, #e74c3c)';
  }
}
async function refreshResolversBadge() {
  try {
    var r = await fetch('/api/resolvers/bank');
    if (!r.ok) return;
    var data = await r.json();
    var activeCount = 0;
    (data.bank || []).forEach(function (b) { if (b.active) activeCount++ });
    updateResolversBadge(activeCount, data.count || 0);
  } catch (e) { }
}
var resolversRefreshTimer = null;
function _buildScoreboardTable(board, showRemove, removeFromBank) {
  if (!board.length) return '<div style="color:var(--text-dim)">' + t('no_active_resolvers') + '</div>';
  // Row-card layout — left holds resolver address + a stats line
  // (speed · score · ✅ · ❌), right holds the action buttons.
  // Replaces the old 6-column table that overflowed off the right
  // edge on phone widths and hid the × button.
  var h = '<div class="rb-rows">';
  for (var i = 0; i < board.length; i++) {
    var b = board[i];
    var scoreColor = b.score >= 0.5 ? 'var(--success)' : b.score >= 0.15 ? 'var(--text)' : 'var(--error)';
    var dot = (b.active !== undefined && b.active)
      ? ' <span style="color:var(--success);font-size:10px">' + icon('dotFilled') + '</span>' : '';
    h += '<div class="rb-row">';
    h += '<div class="rb-row-main">';
    h += '<div class="rb-row-addr">' + esc(b.addr) + dot + '</div>';
    h += '<div class="rb-row-stats">';
    h += '<span>' + (b.avgMs > 0 ? Math.round(b.avgMs) + 'ms' : '-') + '</span>';
    h += '<span style="color:' + scoreColor + ';font-weight:600">' + b.score.toFixed(2) + '</span>';
    h += '<span style="color:var(--success)">' + icon('success') + ' ' + b.success + '</span>';
    h += '<span style="color:var(--error)">' + icon('fail') + ' ' + b.failure + '</span>';
    h += '</div></div>';
    if (showRemove) {
      var fn = removeFromBank ? 'removeResolverFromBank' : 'removeResolver';
      h += '<div class="rb-row-actions">';
      if (removeFromBank) {
        h += '<button class="rb-row-btn rb-row-add" onclick="openBankAddPicker(this,\'' + esc(b.addr) + '\')" '
          + 'data-i18n-title="add_to_list" title="Add to list" aria-label="Add to list">' + icon('add') + '</button>';
      }
      h += '<button class="rb-row-btn rb-row-del" onclick="' + fn + '(\'' + esc(b.addr) + '\')" '
        + 'title="Remove" aria-label="Remove">&times;</button>';
      h += '</div>';
    }
    h += '</div>';
  }
  h += '</div>';
  return h;
}
async function _fetchActiveBoard() {
  var el = document.getElementById('resolverPanelActive');
  try {
    var r = await fetch('/api/resolvers/active');
    if (!r.ok) throw new Error(await r.text());
    var data = await r.json();
    var board = data.scoreboard || [];
    el.innerHTML = _buildScoreboardTable(board, true, false);
    // Sync the Bank tab badge so it stays current while the user
    // sits on the Active view.
    try {
      var br = await fetch('/api/resolvers/bank');
      if (br.ok) {
        var bd = await br.json();
        var bc = document.getElementById('resolverBankCount');
        if (bc) bc.textContent = bd.count || 0;
      }
    } catch (e2) { }
  } catch (e) { el.innerHTML = '<div style="color:var(--error)">' + esc(e.message) + '</div>' }
}
async function _fetchBankBoard() {
  var el = document.getElementById('resolverBankListEl');
  try {
    var r = await fetch('/api/resolvers/bank');
    if (!r.ok) throw new Error(await r.text());
    var data = await r.json();
    var bank = data.bank || [];
    var countEl = document.getElementById('resolverBankCount');
    if (countEl) {
      countEl.textContent = data.count || 0;
      countEl.style.color = (data.count || 0) > 500 ? 'var(--error)' : '';
    }
    var activeCount = 0;
    bank.forEach(function (b) { if (b.active) activeCount++ });
    updateResolversBadge(activeCount, data.count || 0);
    el.innerHTML = _buildScoreboardTable(bank, true, true);
    previewBankCleanup();
  } catch (e) { el.innerHTML = '<div style="color:var(--error)">' + esc(e.message) + '</div>' }
}
function switchResolverTab(tab) {
  currentResolverTab = tab;
  var pa = document.getElementById('resolverPanelActive');
  var pb = document.getElementById('resolverPanelBank');
  if (pa) pa.style.display = tab === 'active' ? '' : 'none';
  if (pb) pb.style.display = tab === 'bank' ? '' : 'none';
  // Tab styling lives on the dynamic strip — re-render so the
  // accent underline tracks the new selection. renderResolverTabs
  // also reads currentResolverTab and decorates the bank tab.
  renderResolverTabs();
  if (tab === 'active') _fetchActiveBoard();
  else _fetchBankBoard();
}
async function openResolversModal() {
  document.getElementById('resolversModal').classList.add('active');
  // Load autoScan from active profile
  var autoScanEl = document.getElementById('bankAutoScan');
  if (autoScanEl && profiles && profiles.profiles && activeProfileId) {
    var p = profiles.profiles.find(function (x) { return x.id === activeProfileId });
    if (p) autoScanEl.checked = p.config.autoScan !== false;
  }
  switchResolverTab('active');
  refreshResolversBadge();
  // Populate the list pill strip at the top of the modal.
  loadResolverLists();
  if (resolversRefreshTimer) clearInterval(resolversRefreshTimer);
  resolversRefreshTimer = setInterval(function () {
    if (!document.getElementById('resolversModal').classList.contains('active')) {
      clearInterval(resolversRefreshTimer); resolversRefreshTimer = null; return;
    }
    if (currentResolverTab === 'active') _fetchActiveBoard();
    else _fetchBankBoard();
  }, 3000);
}
function closeResolversModal() {
  document.getElementById('resolversModal').classList.remove('active');
  if (resolversRefreshTimer) { clearInterval(resolversRefreshTimer); resolversRefreshTimer = null; }
}
function openScannerFromBank() {
  closeResolversModal();
  openScanner();
}
async function doRescanFromBank() {
  closeResolversModal();
  showToast(t('rescan_started'));
  document.getElementById('progressPanel').innerHTML = '';
  showInitProgress();
  try { await fetch('/api/rescan', { method: 'POST' }) } catch (e) { }
  setTimeout(function () { loadChannels().then(function () { if (selectedChannel > 0) loadMessages(selectedChannel) }); refreshResolversBadge(); }, 3000);
}
async function toggleBankAutoScan() {
  var checked = document.getElementById('bankAutoScan').checked;
  if (!profiles || !profiles.profiles || !activeProfileId) return;
  var p = profiles.profiles.find(function (x) { return x.id === activeProfileId });
  if (!p) return;
  var profile = JSON.parse(JSON.stringify(p));
  profile.config.autoScan = checked ? undefined : false;
  try {
    await fetch('/api/profiles', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action: 'update', profile: profile, skipCheck: true }) });
    await loadProfiles();
  } catch (e) { showToast(e.message) }
}
async function removeResolver(addr) {
  try {
    await fetch('/api/resolvers/remove', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ addr: addr }) });
    _fetchActiveBoard();
  } catch (e) { }
}
async function removeResolverFromBank(addr) {
  try {
    await fetch('/api/resolvers/bank', { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ addrs: [addr] }) });
    _fetchBankBoard();
  } catch (e) { }
}
async function resetScoreboard() {
  try {
    await fetch('/api/resolvers/reset-stats', { method: 'POST' });
    if (currentResolverTab === 'active') _fetchActiveBoard();
    else _fetchBankBoard();
  } catch (e) { }
}
function copyResolversList() {
  var panelId = currentResolverTab === 'active' ? 'resolverPanelActive' : 'resolverBankListEl';
  // New row layout uses .rb-row-addr divs in place of <td>.
  var addrs = document.querySelectorAll('#' + panelId + ' .rb-row-addr');
  var lines = [];
  addrs.forEach(function (el) {
    // Strip the trailing active-dot span if present.
    var clone = el.cloneNode(true);
    var dot = clone.querySelector('span');
    if (dot) dot.remove();
    var t = clone.textContent.trim();
    if (t) lines.push(t);
  });
  if (!lines.length) { showToast(t('no_active_resolvers')); return }
  navigator.clipboard.writeText(lines.join('\n')).then(function () { showToast(t('copied')) });
}
async function previewBankCleanup() {
  var val = parseFloat(document.getElementById('bankCleanupSlider').value) || 0.1;
  document.getElementById('bankCleanupValue').textContent = val.toFixed(2);
  try {
    var r = await fetch('/api/resolvers/bank/cleanup', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ minScore: val, dryRun: true }) });
    if (!r.ok) return;
    var data = await r.json();
    document.getElementById('bankCleanupPreview').innerHTML = '<span style="color:var(--error)">' + data.removed + '</span> ' + t('would_be_removed') + ', <span style="color:var(--success)">' + data.remaining + '</span> ' + t('would_remain');
  } catch (e) { }
}
async function doBankCleanup() {
  var val = parseFloat(document.getElementById('bankCleanupSlider').value) || 0.1;
  try {
    var r = await fetch('/api/resolvers/bank/cleanup', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ minScore: val }) });
    if (!r.ok) { showToast('Cleanup failed'); return }
    var data = await r.json();
    showToast(t('removed') + ': ' + data.removed + ', ' + t('remaining') + ': ' + data.remaining);
    _fetchBankBoard();
    refreshResolversBadge();
  } catch (e) { showToast(e.message) }
}
async function addResolversToBank() {
  var text = document.getElementById('bankAddResolvers').value.trim();
  var resolvers = text.split(/[\n,;\s]+/).map(function (s) { return s.trim() }).filter(Boolean);
  if (!resolvers.length) return;
  try {
    var r = await fetch('/api/resolvers/bank', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ resolvers: resolvers }) });
    if (!r.ok) { showToast('Add failed'); return }
    var data = await r.json();
    showToast(t('added') + ': ' + data.added);
    document.getElementById('bankAddResolvers').value = '';
    _fetchBankBoard();
    refreshResolversBadge();
  } catch (e) { showToast(e.message) }
}

