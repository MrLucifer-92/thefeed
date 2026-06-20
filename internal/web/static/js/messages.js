// ===== MESSAGES =====
// Tracks channels we've already auto-fetched on first open so we don't
// re-trigger a refresh for genuinely empty channels.
var autoFetchedChannels = {};
async function loadMessages(chNum) {
  try {
    var r = await fetch('/api/messages/' + chNum); if (chNum !== selectedChannel) return;
    var data = await r.json(); if (chNum !== selectedChannel) return;
    renderMessages(data.messages || [], data.gaps || []);
    // If the server has nothing cached for this channel and we haven't
    // already kicked off a fetch this session, trigger one. Covers the
    // post-clear / fresh-restart case where the on-disk cache is empty.
    if ((!data.messages || data.messages.length === 0) && !autoFetchedChannels[chNum] && !refreshingChannels[chNum]) {
      autoFetchedChannels[chNum] = true;
      refreshingChannels[chNum] = true;
      var ch = channels[chNum - 1];
      if (ch) showChannelFetchProgress(chNum, ch.Name || ch.name || '');
      try { await fetch('/api/refresh?channel=' + chNum, { method: 'POST' }); } catch (e) { }
      // Fail-safe: if SSE never delivers the 'channels' update (server
      // refresh failed silently, transport dropped, etc.), clear the
      // flag after 60s so the user can manually retry.
      setTimeout(function () {
        if (refreshingChannels[chNum]) {
          delete refreshingChannels[chNum];
          var fb = document.getElementById('prog-fetch-ch-' + chNum);
          if (fb) fb.remove();
        }
      }, 60000);
    }
    if (!refreshingChannels[chNum]) {
      var fetchBar = document.getElementById('prog-fetch-ch-' + chNum); if (fetchBar) fetchBar.remove();
    }
    if (channels[chNum - 1]) {
      var cn = channels[chNum - 1].Name || channels[chNum - 1].name || '';
      rememberSeen(
        cn,
        channels[chNum - 1].LastMsgID || channels[chNum - 1].lastMsgID || 0,
        channels[chNum - 1].ContentHash || channels[chNum - 1].contentHash || 0
      );
      renderChannels();
    }
  } catch (e) { }
}

function renderPollCard(pollBody) {
  var lines = pollBody.split('\n');
  var html = '<div class="poll-card">';
  var hasContent = false;
  for (var i = 0; i < lines.length; i++) {
    var ln = lines[i];
    if (ln.indexOf('📊 ') === 0) {
      html += '<div class="poll-question">' + esc(ln.substring(2).trim()) + '</div>';
      hasContent = true;
    } else if (ln.indexOf('○ ') === 0) {
      html += '<div class="poll-option">' + esc(ln) + '</div>';
      hasContent = true;
    } else if (ln.trim()) {
      html += '<div>' + linkify(ln) + '</div>';
      hasContent = true;
    }
  }
  if (!hasContent) html += '<div class="poll-question" style="opacity:.5">' + t('poll_placeholder') + '</div>';
  html += '</div>';
  return html;
}

