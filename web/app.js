/* GOST 面板 · 前端逻辑 */
'use strict';

/* ---------------- 基础 ---------------- */
const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => Array.from(el.querySelectorAll(s));

// 支持访问路径前缀：/panel/ -> BASE="/panel/"
const BASE = (() => {
  let p = location.pathname;
  if (p.endsWith('/')) return p;
  return p.replace(/[^/]*$/, '');
})();

const ICONS = {
  grid: '<path d="M3 3h8v8H3zM13 3h8v8h-8zM3 13h8v8H3zM13 13h8v8h-8z"/>',
  server: '<path d="M3 4h18v6H3zM3 14h18v6H3zM7 7h.01M7 17h.01"/>',
  bell: '<path d="M18 8a6 6 0 1 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>',
  settings: '<path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 3 15a2 2 0 1 1 0-4 1.7 1.7 0 0 0 1.4-2.7l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 10 4.6V4a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 21 11a2 2 0 1 1 0 4z"/>',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
  sun: '<path d="M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8M21 3v5h-5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  copy: '<path d="M9 9h10v10H9z"/><path d="M5 15H4V4h11v1"/>',
  edit: '<path d="M11 4H4v16h16v-7"/><path d="M18.5 2.5a2.1 2.1 0 0 1 3 3L12 15l-4 1 1-4z"/>',
  trash: '<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6"/>',
  power: '<path d="M12 2v10"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/>',
  reset: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5"/>',
  qr: '<path d="M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h3v3h-3zM19 19h2v2h-2zM14 19h2v2h-2zM19 14h2v2h-2z"/>',
  link: '<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
  external: '<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><path d="M15 3h6v6M10 14L21 3"/>',
  check: '<path d="M20 6L9 17l-5-5"/>',
  x: '<path d="M18 6L6 18M6 6l12 12"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  rss: '<path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><path d="M4.5 17.5h2.5V20H4.5z"/>',
  chevron: '<path d="M6 9l6 6 6-6"/>',
};

function renderIcons(root = document) {
  $$('[data-icon]', root).forEach(el => {
    const name = el.dataset.icon;
    if (!ICONS[name]) return;
    el.innerHTML = `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="square" stroke-linejoin="miter">${ICONS[name]}</svg>`;
  });
}
const icon = (name, cls = '') =>
  `<svg class="icon ${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="square" stroke-linejoin="miter">${ICONS[name] || ''}</svg>`;

/* ---------------- 状态 ---------------- */
let state = {
  view: 'overview',
  nodes: [],
  overview: null,
  system: null,
  settings: null,
  notify: null,
  subConfig: null,
  publicHost: '',
  publicHostPrivate: false,
  lastParsed: null,
  timer: null,
};

/* ---------------- 工具 ---------------- */
async function api(path, opts = {}) {
  const res = await fetch(BASE + path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    ...opts,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  if (res.status === 401) {
    showLogin();
    throw new Error('未登录');
  }
  const text = await res.text();
  let data = null;
  if (text) { try { data = JSON.parse(text); } catch (e) { data = null; } }
  if (!res.ok) throw new Error((data && data.error) || `请求失败 (${res.status})`);
  return data;
}

let toastTimer = null;
function toast(msg, type = 'ok') {
  const el = $('#toast');
  el.textContent = msg;
  el.className = 'toast' + (type === 'err' ? ' err' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.add('hidden'), 2800);
}

function copyText(text) {
  const done = () => toast('已复制到剪贴板');
  const fallback = () => {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); done(); }
    catch (e) { toast('复制失败，请手动复制', 'err'); }
    document.body.removeChild(ta);
  };
  if (navigator.clipboard && window.isSecureContext) {
    navigator.clipboard.writeText(text).then(done).catch(fallback);
  } else fallback();
}

function fmtBytes(n) {
  n = Number(n) || 0;
  if (n < 1024) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return n.toFixed(n >= 100 ? 0 : n >= 10 ? 1 : 2) + ' ' + u[i];
}
function fmtTime(ts) {
  if (!ts) return '—';
  const d = new Date(ts * 1000);
  const p = x => String(x).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
function fmtDay(ts) {
  const d = new Date(ts * 1000);
  return `${d.getMonth() + 1}/${d.getDate()}`;
}
function fmtDuration(sec) {
  sec = Math.max(0, Math.floor(sec || 0));
  const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60);
  if (d > 0) return `${d} 天 ${h} 小时`;
  if (h > 0) return `${h} 小时 ${m} 分`;
  return `${m} 分 ${sec % 60} 秒`;
}
const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

/* ---------------- 主题（亮/暗双模） ---------------- */
function refreshThemeBtn() {
  const t = document.documentElement.dataset.theme;
  $('#themeBtn').innerHTML = icon(t === 'dark' ? 'sun' : 'moon');
  $('#themeBtn').setAttribute('aria-label', t === 'dark' ? '切换到浅色' : '切换到深色');
  $('#themeBtn').title = $('#themeBtn').getAttribute('aria-label');
}
function applyTheme(t) {
  document.documentElement.dataset.theme = t;
  localStorage.setItem('gp_theme', t);
  refreshThemeBtn();
  if (state.overview && state.view === 'overview') renderOverview();
}
function cycleTheme() {
  applyTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark');
}
function initTheme() {
  const q = new URLSearchParams(location.search);
  const themeParam = q.get('theme');
  document.documentElement.dataset.theme = (themeParam === 'light' || themeParam === 'dark')
    ? themeParam : (localStorage.getItem('gp_theme') || 'light');
  refreshThemeBtn();
}

/* ---------------- 登录 ---------------- */
async function init() {
  initTheme();
  renderIcons();
  let sess = null;
  try { sess = await api('api/session'); } catch (e) { sess = null; }
  if (sess && sess.loggedIn) showApp(); else showLogin();
}

function showLogin() {
  $('#app').classList.add('hidden');
  $('#login').classList.remove('hidden');
  clearInterval(state.timer);
  state.timer = null;
}
function showApp() {
  $('#login').classList.add('hidden');
  $('#app').classList.remove('hidden');
  renderIcons();
  refreshAll();
  if (!state.timer) state.timer = setInterval(refreshAll, 5000);
  if (!state.versionLoaded) {
    state.versionLoaded = true;
    api('api/system').then(sys => {
      const v = (sys.panel && sys.panel.version) || '';
      const fv = $('#footerVersion'); if (fv && v) fv.textContent = 'Gost-WebUI v' + String(v).replace(/^v/, '');
    }).catch(() => {});
  }
}

$('#loginBtn').onclick = async () => {
  const btn = $('#loginBtn');
  btn.disabled = true;
  const err = $('#loginErr');
  err.classList.add('hidden');
  try {
    await api('api/login', { method: 'POST', body: { username: $('#loginUser').value.trim(), password: $('#loginPass').value } });
    showApp();
  } catch (e) {
    err.textContent = e.message;
    err.classList.remove('hidden');
  } finally { btn.disabled = false; }
};
$('#loginPass').addEventListener('keydown', e => { if (e.key === 'Enter') $('#loginBtn').click(); });
$('#logoutBtn').onclick = async () => {
  try { await api('api/logout', { method: 'POST' }); } catch (e) { /* ignore */ }
  showLogin();
};
$('#refreshBtn').onclick = () => refreshAll(true);
$('#themeBtn').onclick = cycleTheme;

/* ---------------- 视图 ---------------- */
$('#nav').addEventListener('click', e => {
  const a = e.target.closest('a[data-view]');
  if (a) { e.preventDefault(); setView(a.dataset.view); }
});

const VIEW_TITLE = { overview: '网络概览', nodes: '节点管理', notify: '通知提醒', system: '系统设置' };
const VIEW_SUBTITLE = { overview: '流量与节点状态总览', nodes: '节点转发与配额管理', notify: '通知与告警设置', system: '面板与 gost 运行参数' };
function setView(view) {
  state.view = view;
  $$('#nav a').forEach(a => a.classList.toggle('active', a.dataset.view === view));
  $$('.view').forEach(v => v.classList.toggle('hidden', v.id !== 'view-' + view));
  $('#viewTitle').textContent = VIEW_TITLE[view] || '';
  $('#viewSubtitle').textContent = VIEW_SUBTITLE[view] || '';
  $$('#nav a').forEach(a => a.setAttribute('aria-current', a.dataset.view === view ? 'page' : 'false'));
  $('#sidebar').classList.remove('open');
  if (view === 'overview') renderOverview();
  if (view === 'nodes') renderNodes();
  if (view === 'notify') loadNotify();
  if (view === 'system') loadSystem();
}

/* ---------------- 数据刷新 ---------------- */
async function refreshAll(manual = false) {
  try {
    const [ov, nd] = await Promise.all([api('api/overview'), api('api/nodes')]);
    state.overview = ov;
    state.nodes = nd.nodes || [];
    state.publicHost = nd.publicHost || '';
    state.publicHostPrivate = nd.publicHostPrivate === true;
    renderStatusBar();
    renderHostWarn();
    if (state.view === 'overview') renderOverview();
    if (state.view === 'nodes') renderNodes();
    if (state.view === 'notify' && !state.notify) await loadNotify();
    if (state.view === 'system') await loadSystem();
    if (manual) toast('已刷新');
  } catch (e) {
    if (e.message !== '未登录') toast(e.message, 'err');
  }
}

function renderStatusBar() {
  const g = (state.overview && state.overview.gost) || {};
  const ok = g.running && g.reachable !== false;
  $('#gostDot').className = 'status-dot ' + (ok ? 'ok' : 'bad');
  $('#gostText').textContent = ok ? `gost 运行中 · PID ${g.pid || '—'}` : (g.running ? 'gost 异常' : 'gost 未运行');
  const hc = $('#hostChip'); if (hc) hc.textContent = state.publicHost || '未检测到地址';
  $('#brandSub').textContent = `/ ${state.nodes.length} 节点`;
}

function renderHostWarn() {
  const html = state.publicHostPrivate ? `<div class="notice warn">
    当前服务器地址 <b>${esc(state.publicHost)}</b> 是内网地址，外部客户端无法连接。
    请到「系统设置 → 服务器地址」填写公网 IP 或域名。</div>` : '';
  ['#hostWarn', '#hostWarnOverview'].forEach(sel => {
    const el = $(sel);
    if (el) el.innerHTML = html;
  });
}

/* ---------------- 概览 ---------------- */
function renderOverview() {
  const ov = state.overview;
  if (!ov) return;
  const n = ov.nodes || {}, t = ov.today || {}, m = ov.month || {}, all = ov.total || {};
  $('#statCards').innerHTML = `
    ${metricCard('节点', `${n.enabled || 0}<small> / ${n.total || 0}</small>`, n.blocked ? `${n.blocked} 个已超限暂停` : '全部正常')}
    ${metricCard('当前连接', String(n.currentConns || 0), `累计连接 ${n.totalConns || 0}`)}
    ${metricCard('今日流量', fmtBytes((t.in || 0) + (t.out || 0)), `上行 ${fmtBytes(t.in)} · 下行 ${fmtBytes(t.out)}`)}
    ${metricCard('本月流量', fmtBytes((m.in || 0) + (m.out || 0)), `上行 ${fmtBytes(m.in)} · 下行 ${fmtBytes(m.out)}`)}`;
  $('#chartTotal').textContent = `累计 ${fmtBytes((all.in || 0) + (all.out || 0))}`;

  renderHostCards((ov.host && ov.host.metrics) || {});

  const series = (ov.series || []).slice(-30);
  drawChart($('#chart'), series, { step: 86400 });

  const top = ov.top || [];
  if (!top.length) {
    $('#topNodes').innerHTML = '<div class="empty">暂无数据，先添加一个节点</div>';
  } else {
    const max = Math.max(1, ...top.map(x => (x.in || 0) + (x.out || 0)));
    $('#topNodes').innerHTML = `<div class="table-wrap"><table><thead><tr>
      <th>节点</th><th>状态</th><th style="width:38%">用量</th><th class="right">本月流量</th>
      </tr></thead><tbody>${top.map(x => {
        const sum = (x.in || 0) + (x.out || 0);
        const pct = Math.max(0, Math.round(sum / max * 100));
        return `<tr>
          <td>${esc(x.name)}</td>
          <td>${x.enabled ? (x.blocked ? '<span class="status-inline"><span class="status-dot bad"></span>超限暂停</span>' : '<span class="status-inline"><span class="status-dot ok"></span>运行中</span>') : '<span class="status-inline"><span class="status-dot"></span>已停用</span>'}</td>
          <td><div class="meter"><i style="width:${pct}%"></i></div></td>
          <td class="right mono">${fmtBytes(sum)}</td></tr>`;
      }).join('')}</tbody></table></div>`;
  }
}

function metricCard(k, v, s) {
  return `<div class="card metric"><div class="k">${k}</div><div class="v">${v}</div><div class="s">${s || ''}</div></div>`;
}

// 主机负载卡片：卡片内左侧文字、右侧状态条
function renderHostCards(met) {
  const bars = [
    { k: 'CPU 负载', v: (met.load1 || 0).toFixed(2), s: `${met.cpuCount || 0} 核 · 使用率 ${(met.cpuPercent || 0).toFixed(1)}%`, pct: met.cpuPercent || 0 },
    { k: '内存', v: `${(met.memPercent || 0).toFixed(1)}%`, s: `${fmtBytes(met.memUsed)} / ${fmtBytes(met.memTotal)}`, pct: met.memPercent || 0 },
    { k: '磁盘', v: `${(met.diskPercent || 0).toFixed(1)}%`, s: `${fmtBytes(met.diskUsed)} / ${fmtBytes(met.diskTotal)}`, pct: met.diskPercent || 0 },
  ];
  $('#hostCards').innerHTML = bars.map(b => {
    const cls = b.pct >= 90 ? 'bad' : b.pct >= 75 ? 'warn' : '';
    return `<div class="card host-card">
      <div class="host-text metric">
        <div class="k">${b.k}</div>
        <div class="v">${b.v}</div>
        <div class="s">${b.s}</div>
      </div>
      <div class="host-bar"><div class="meter"><i class="${cls}" style="width:${Math.min(100, b.pct)}%"></i></div></div>
    </div>`;
  }).join('');
}

function statusTag(enabled, blocked) {
  // 常态不显示文字（开关本身即状态）；仅超限暂停时提示
  if (enabled && blocked) return '<span class="status-inline"><span class="status-dot bad"></span>超限暂停</span>';
  return '';
}
/* 节点表专用：开关 + 异常徽标 */
function nodeStatusCell(n) {
  const blocked = n.live && n.live.quotaBlocked;
  const badge = (n.enabled && blocked) ? '<span class="status-inline"><span class="status-dot bad"></span>超限暂停</span>' : '';
  return `<button class="node-toggle" data-act="toggle" data-id="${esc(n.id)}" role="switch" aria-checked="${!!n.enabled}" aria-label="${n.enabled ? '停用' : '启用'}${esc(n.name)}"><span class="toggle-track"></span></button>${badge}`;
}

/* 流量图表：翡翠上行、淡金下行 */
function drawChart(canvas, points, opts) {
  if (!canvas) return;
  opts = opts || {};
  drawChart._impl = drawChartImpl;
  drawChartImpl(canvas, points, opts);
  attachChartHover(canvas);
}

function drawChartImpl(canvas, points, opts) {
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth || 600;
  const h = canvas.clientHeight || 240;
  canvas.width = w * dpr;
  canvas.height = h * dpr;
  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);

  const css = getComputedStyle(document.documentElement);
  const cAccent = css.getPropertyValue('--color-accent').trim() || '#1a1a1a';
  const cTertiary = css.getPropertyValue('--color-text-tertiary').trim() || '#888';
  const cBorderLight = css.getPropertyValue('--color-border-light').trim() || '#eee';
  const cGold = css.getPropertyValue('--color-gold').trim() || cTertiary;
  const cCross = css.getPropertyValue('--color-text-secondary').trim() || '#666';

  const padL = 62, padR = 26, padT = 12, padB = 28;
  const cw = w - padL - padR, ch = h - padT - padB;

  canvas._chart = null;
  if (!points.length) {
    ctx.fillStyle = cTertiary;
    ctx.font = '13px ' + css.getPropertyValue('--font-sans');
    ctx.textAlign = 'center';
    ctx.fillText('暂无流量数据', w / 2, h / 2);
    return;
  }

  const max = Math.max(1, ...points.map(p => Math.max(p.in || 0, p.out || 0)));
  const step = niceCeil(max / 4) || 1;
  const niceMax = step * 4;

  ctx.font = '11px ' + css.getPropertyValue('--font-mono');
  ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = padT + ch * i / 4;
    ctx.strokeStyle = cBorderLight;
    ctx.lineWidth = 1;
    ctx.setLineDash([3, 5]);
    ctx.beginPath();
    ctx.moveTo(padL, y + 0.5);
    ctx.lineTo(w - padR, y + 0.5);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = cTertiary;
    ctx.fillText(fmtBytes(niceMax * (4 - i) / 4), padL - 10, y + 4);
  }

  const stepX = points.length > 1 ? cw / (points.length - 1) : 0;
  const px = i => padL + stepX * i;
  const py = v => padT + ch * (1 - (v || 0) / niceMax);

  const smooth = pts => {
    // Catmull-Rom 转贝塞尔，曲线顺滑
    ctx.moveTo(pts[0][0], pts[0][1]);
    for (let i = 0; i < pts.length - 1; i++) {
      const p0 = pts[Math.max(0, i - 1)], p1 = pts[i], p2 = pts[i + 1], p3 = pts[Math.min(pts.length - 1, i + 2)];
      ctx.bezierCurveTo(
        p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6,
        p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6,
        p2[0], p2[1]);
    }
  };
  const series = (key, color, alphaTop) => {
    const pts = points.map((p, i) => [px(i), py(p[key])]);
    // 渐变填充（上线性渐隐）
    const grad = ctx.createLinearGradient(0, padT, 0, padT + ch);
    grad.addColorStop(0, color + alphaTop);
    grad.addColorStop(1, color + '00');
    ctx.beginPath(); smooth(pts);
    ctx.lineTo(px(points.length - 1), padT + ch); ctx.lineTo(padL, padT + ch); ctx.closePath();
    ctx.fillStyle = grad; ctx.fill();
    // 描边
    ctx.beginPath(); smooth(pts);
    ctx.strokeStyle = color; ctx.lineWidth = 2; ctx.lineJoin = 'round'; ctx.lineCap = 'round';
    ctx.shadowColor = color + '55'; ctx.shadowBlur = 6; ctx.shadowOffsetY = 2;
    ctx.stroke();
    ctx.shadowColor = 'transparent'; ctx.shadowBlur = 0; ctx.shadowOffsetY = 0;
    // 末端光点
    const last = pts[pts.length - 1];
    ctx.beginPath(); ctx.arc(last[0], last[1], 3, 0, Math.PI * 2);
    ctx.fillStyle = color; ctx.fill();
    ctx.beginPath(); ctx.arc(last[0], last[1], 6, 0, Math.PI * 2);
    ctx.fillStyle = color + '30'; ctx.fill();
  };
  series('in', cAccent, '33');
  series('out', cGold, '26');

  ctx.textAlign = 'center';
  ctx.fillStyle = cTertiary;
  const ticks = Math.min(6, points.length);
  for (let i = 0; i < ticks; i++) {
    const idx = Math.round(i * (points.length - 1) / Math.max(1, ticks - 1));
    ctx.fillText(opts.hourAxis ? fmtHour(points[idx].ts) : fmtDay(points[idx].ts), px(idx), h - 8);
  }

  canvas._chart = { points, px, py, padL, padT, padB, w, h, stepX,
    bucket: opts.step || (points.length > 1 ? points[1].ts - points[0].ts : 86400),
    hourAxis: !!opts.hourAxis, cAccent, cGold, cCross };
}

/* ---- hover 感应：竖直参考线 + 数据卡（触屏 touch 亦支持） ---- */
function attachChartHover(canvas) {
  if (canvas._hoverBound) return;
  canvas._hoverBound = true;
  const parent = canvas.parentElement;
  if (!parent) return;
  if (getComputedStyle(parent).position === 'static') parent.style.position = 'relative';
  let tip = parent.querySelector('.chart-tip');
  if (!tip) {
    tip = document.createElement('div');
    tip.className = 'chart-tip hidden';
    parent.appendChild(tip);
  }
  const overlay = (idx) => {
    const st = canvas._chart;
    if (!st) return;
    drawChartImpl(canvas, st.points, { step: st.bucket, hourAxis: st.hourAxis });
    if (idx == null) return;
    const g = canvas.getContext('2d');
    const dpr = window.devicePixelRatio || 1;
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    const p = st.points[idx], x = st.px(idx);
    g.strokeStyle = st.cCross; g.globalAlpha = .45;
    g.lineWidth = 1; g.setLineDash([4, 4]);
    g.beginPath(); g.moveTo(x + .5, st.padT); g.lineTo(x + .5, st.h - st.padB); g.stroke();
    g.setLineDash([]); g.globalAlpha = 1;
    [[p.in, st.cAccent], [p.out, st.cGold]].forEach(([v, c]) => {
      g.beginPath(); g.arc(x, st.py(v), 4.5, 0, Math.PI * 2);
      g.fillStyle = c; g.fill();
      g.strokeStyle = '#ffffff'; g.lineWidth = 1.5; g.stroke();
    });
  };
  const showAt = (clientX, clientY) => {
    const st = canvas._chart;
    if (!st || !st.points.length) return;
    const rect = canvas.getBoundingClientRect();
    const mx = clientX - rect.left;
    let idx = st.stepX > 0 ? Math.round((mx - st.padL) / st.stepX) : 0;
    idx = Math.max(0, Math.min(st.points.length - 1, idx));
    overlay(idx);
    const p = st.points[idx];
    const label = st.hourAxis ? fmtHourTip(p.ts) : fmtDayTip(p.ts, st.bucket);
    tip.innerHTML = `<div class="ct-date">${esc(label)}</div>
      <div class="ct-row"><i style="background:var(--color-accent)"></i>上行 <b>${fmtBytes(p.in || 0)}</b></div>
      <div class="ct-row"><i style="background:var(--color-gold)"></i>下行 <b>${fmtBytes(p.out || 0)}</b></div>
      <div class="ct-row ct-total">合计 <b>${fmtBytes((p.in || 0) + (p.out || 0))}</b></div>`;
    tip.classList.remove('hidden');
    const tw = tip.offsetWidth, th = tip.offsetHeight;
    let left = st.px(idx) + 14;
    if (left + tw > st.w - 4) left = st.px(idx) - tw - 14;
    if (left < 4) left = 4;
    let top = clientY - rect.top - th - 12;
    if (top < 0) top = clientY - rect.top + 18;
    tip.style.left = left + 'px';
    tip.style.top = top + 'px';
  };
  const clear = () => {
    tip.classList.add('hidden');
    overlay(null);
  };
  canvas.addEventListener('mousemove', ev => showAt(ev.clientX, ev.clientY));
  canvas.addEventListener('mouseleave', clear);
  canvas.addEventListener('touchstart', ev => {
    const t = ev.touches[0]; if (t) showAt(t.clientX, t.clientY);
  }, { passive: true });
  canvas.addEventListener('touchmove', ev => {
    const t = ev.touches[0]; if (t) showAt(t.clientX, t.clientY);
  }, { passive: true });
  canvas.addEventListener('touchend', () => setTimeout(clear, 1200));
}

function fmtHour(ts) {
  const d = new Date(ts * 1000);
  return `${String(d.getHours()).padStart(2, '0')}:00`;
}
function fmtHourTip(ts) {
  const d = new Date(ts * 1000);
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, '0')}:00 – ${String(d.getHours()).padStart(2, '0')}:59`;
}
function fmtDayTip(ts, bucket) {
  const d = new Date(ts * 1000);
  const day = `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`;
  return bucket >= 86400 ? `${day} 全天` : `${day} ${String(d.getHours()).padStart(2, '0')}:00 时段`;
}

function niceCeil(v) {
  if (v <= 0) return 0;
  const exp = Math.floor(Math.log10(v));
  const base = Math.pow(10, exp);
  const n = v / base;
  const m = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10;
  return m * base;
}

/* ---------------- 节点列表 ---------------- */
function renderNodes() {
  closeAllDropdowns();
  const wrap = $('#nodeTable');
  const nodes = state.nodes;
  const active = nodes.filter(n => n.enabled && !(n.live && n.live.quotaBlocked)).length;
  $('#nodesSummary').textContent = `${nodes.length} 个节点 · ${active} 个已启用 · 本月 ${fmtBytes(nodes.reduce((sum, n) => sum + (n.monthIn || 0) + (n.monthOut || 0), 0))}`;
  if (!nodes.length) {
    wrap.innerHTML = '<div class="empty"><span class="empty-title">暂无节点</span><p>添加第一个节点，开始中转。</p><button class="btn secondary" data-act="add">添加节点</button></div>';
    return;
  }
  wrap.innerHTML = `<table>
    <thead><tr><th>节点名称 / 落地机</th><th>监听端口</th><th>本月用量 / 配额</th><th>状态</th><th class="right">操作</th></tr></thead>
    <tbody>${nodes.map(n => `<tr>
      <td><button class="node-select" data-act="detail" data-id="${esc(n.id)}" aria-label="查看${esc(n.name)}详情">${icon('server')}<span><b>${esc(n.name)}</b><small>${esc(n.protocol || 'TCP')}${n.mode === 'gost' ? ' · GOST' : ''}${n.udp ? ' · TCP+UDP' : ''} / ${n.mode === 'reality' ? '本机 sing-box' : (n.mode === 'gost' && n.gostLocal ? '本机落地' : `${esc(n.targetHost)}:${n.targetPort}`)}</small></span></button></td>
      <td class="mono">${n.listenPort}</td>
      <td class="node-quota"><span class="mono">${fmtBytes((n.monthIn || 0) + (n.monthOut || 0))}</span>${quotaCell(n)}</td>
      <td class="node-status-cell">${nodeStatusCell(n)}</td>
      <td class="ops right"><div class="dropdown">
        <button class="btn ghost sm dropdown-toggle" data-act="menu" data-id="${esc(n.id)}" aria-label="${esc(n.name)}的操作" aria-expanded="false">操作${icon('chevron', 'sm')}</button>
        <div class="dropdown-menu">
          <button data-act="copy" data-id="${esc(n.id)}">${icon('copy')}复制链接</button>
          <button data-act="sub" data-id="${esc(n.id)}">${icon('rss')}订阅</button>
          <button data-act="detail" data-id="${esc(n.id)}">${icon('qr')}详情</button>
          <button data-act="edit" data-id="${esc(n.id)}">${icon('edit')}编辑</button>
          <button data-act="toggle" data-id="${esc(n.id)}">${icon('power')}${n.enabled ? '停用' : '启用'}</button>
          <button data-act="del" data-id="${esc(n.id)}" class="danger">${icon('trash')}删除</button>
        </div>
      </div></td>
    </tr>`).join('')}</tbody></table>`;
}

function quotaCell(n) {
  if (!n.quota || !n.quota.enabled || !n.quota.bytes) return '<span class="tertiary">不限</span>';
  const lv = n.live || {};
  const used = lv.quotaUsed || 0;
  const pct = Math.min(100, used / n.quota.bytes * 100);
  const cls = pct >= 95 ? 'bad' : pct >= 80 ? 'warn' : '';
  const period = { daily: '每日', monthly: '每月', total: '总量' }[n.quota.period] || '';
  return `<div class="mono" style="font-size:12px">${fmtBytes(used)} / ${fmtBytes(n.quota.bytes)} · ${period}</div>
    <div class="meter"><i class="${cls}" style="width:${Math.max(0, pct)}%"></i></div>`;
}

async function nodeTableClick(e) {
  const btn = e.target.closest('button[data-act]');
  if (!btn) return;
  const id = btn.dataset.id;
  if (btn.dataset.act === 'add') { openNodeForm(null); return; }
  // 下拉菜单开合
  if (btn.dataset.act === 'menu') {
    e.stopPropagation();
    const dd = btn.closest('.dropdown');
    const wasOpen = dd.classList.contains('open');
    closeAllDropdowns();
    if (!wasOpen) {
      dd.classList.add('open');
      btn.setAttribute('aria-expanded', 'true');
      const menu = dd.querySelector('.dropdown-menu') || menuHome.get(dd);
      // 祖先 backdrop-filter 会劫持 fixed 坐标系 → 临时挂到 body
      if (menu && menu.parentElement !== document.body) {
        menuHome.set(dd, menu);
        document.body.appendChild(menu);
      }
      const rect = btn.getBoundingClientRect();
      menu.style.display = 'flex';
      menu.style.left = '0px'; menu.style.top = '0px';
      const mr = menu.getBoundingClientRect();
      const mw = mr.width || 176;
      const mh = mr.height || 0;
      let left = rect.right - mw;
      if (left < 8) left = 8;
      if (left + mw > window.innerWidth - 8) left = window.innerWidth - mw - 8;
      let top = rect.bottom + 6;
      if (top + mh > window.innerHeight - 8) top = Math.max(8, rect.top - mh - 6);
      menu.style.left = Math.round(left) + 'px';
      menu.style.top = Math.round(top) + 'px';
    }
    return;
  }

  const node = state.nodes.find(x => x.id === id);
  if (!node) return;
  closeAllDropdowns();
  switch (btn.dataset.act) {
    case 'copy': {
      const r = await api(`api/nodes/${id}/link`).catch(err => { toast(err.message, 'err'); return null; });
      if (r && r.url) copyText(r.url);
      break;
    }
    case 'detail': openDetail(node); break;
    case 'sub': openNodeSub(node); break;
    case 'edit': openNodeForm(node); break;
    case 'toggle':
      try {
        await api(`api/nodes/${id}/toggle`, { method: 'POST', body: { enabled: !node.enabled } });
        toast(node.enabled ? '已停用' : '已启用');
        refreshAll();
      } catch (err) { toast(err.message, 'err'); }
      break;
    case 'del':
      if (!confirm(`确定删除节点「${node.name}」？会同时停止该监听端口。`)) return;
      try {
        await api(`api/nodes/${id}`, { method: 'DELETE' });
        toast('已删除');
        refreshAll();
      } catch (err) { toast(err.message, 'err'); }
      break;
  }
}
$('#nodeTable').addEventListener('click', nodeTableClick);
document.body.addEventListener('click', e => { if (e.target.closest('body > .dropdown-menu')) nodeTableClick(e); });
$('#addNodeBtn').onclick = () => openNodeForm(null);

// 点击表格以外区域时关闭所有下拉菜单
const menuHome = new WeakMap();   // dd -> 其原始菜单节点
function closeAllDropdowns() {
  $$('.dropdown.open').forEach(d => {
    d.classList.remove('open');
    d.querySelector('.dropdown-toggle')?.setAttribute('aria-expanded', 'false');
  });
  // 归还所有被提到 body 的菜单，避免节点堆积
  document.body.querySelectorAll(':scope > .dropdown-menu').forEach(m => {
    m.remove();
  });
}
document.addEventListener('click', closeAllDropdowns);
// 滚动/缩放时菜单（fixed 定位）会脱离按钮，直接关闭
window.addEventListener('resize', closeAllDropdowns);
window.addEventListener('scroll', closeAllDropdowns, true);

/* ---------------- 弹窗 ---------------- */
let modalPreviousFocus = null;
function openModal(html, wide) {
  const m = $('#modal');
  if (m.classList.contains('hidden')) modalPreviousFocus = document.activeElement;
  m.innerHTML = `<div class="modal-box${wide ? ' wide' : ''}">${html}</div>`;
  m.classList.remove('hidden');
  m.setAttribute('role', 'dialog');
  m.setAttribute('aria-modal', 'true');
  m.setAttribute('aria-label', m.querySelector('h2')?.textContent || '详情');
  m.onclick = e => { if (e.target === m) closeModal(); };
  renderIcons(m);
  $('#app').inert = true;
  m.querySelector('.modal-close')?.setAttribute('aria-label', '关闭弹窗');
  m.querySelector('button, input, select, textarea')?.focus();
}
function closeModal() {
  if ($('#modal').classList.contains('hidden')) return;
  $('#modal').classList.add('hidden');
  $('#modal').innerHTML = '';
  $('#app').inert = false;
  if (modalPreviousFocus?.isConnected) modalPreviousFocus.focus();
}

/* ---------------- 添加/编辑节点 ---------------- */
const GOST_PROTOCOLS = [
  { value: 'http', label: 'HTTP', transport: 'tcp', auth: 'userpass' },
  { value: 'http2', label: 'HTTP/2', transport: 'http2', auth: 'userpass' },
  { value: 'socks4', label: 'SOCKS4', transport: 'tcp', auth: 'user' },
  { value: 'socks4a', label: 'SOCKS4A', transport: 'tcp', auth: 'user' },
  { value: 'socks5', label: 'SOCKS5', transport: 'tcp', auth: 'userpass' },
  { value: 'ss', label: 'Shadowsocks TCP', transport: 'tcp', auth: 'ss' },
  { value: 'ssu', label: 'Shadowsocks UDP', transport: 'udp', auth: 'ss' },
  { value: 'sni', label: 'SNI 透明代理', transport: 'tcp', auth: 'none' },
  { value: 'relay', label: 'GOST Relay', transport: 'tcp', auth: 'userpass' },
];
const GOST_TRANSPORTS = [
  { value: 'tcp', label: 'TCP', network: 'tcp' },
  { value: 'mtcp', label: 'Multiplex TCP', network: 'tcp' },
  { value: 'udp', label: 'UDP', network: 'udp' },
  { value: 'tls', label: 'TLS', network: 'tcp' },
  { value: 'dtls', label: 'DTLS（客户端需证书）', network: 'udp' },
  { value: 'mtls', label: 'Multiplex TLS', network: 'tcp' },
  { value: 'ws', label: 'WebSocket', network: 'tcp', path: true },
  { value: 'wss', label: 'WebSocket TLS', network: 'tcp', path: true },
  { value: 'mws', label: 'Multiplex WebSocket', network: 'tcp', path: true },
  { value: 'mwss', label: 'Multiplex WebSocket TLS', network: 'tcp', path: true },
  { value: 'h2', label: 'HTTP/2 TLS', network: 'tcp', path: true },
  { value: 'h2c', label: 'HTTP/2 Cleartext', network: 'tcp', path: true },
  { value: 'http2', label: 'HTTP/2 Proxy Channel', network: 'tcp' },
  { value: 'grpc', label: 'gRPC', network: 'tcp', path: true },
  { value: 'pht', label: 'HTTP Tunnel', network: 'tcp' },
  { value: 'phts', label: 'HTTPS Tunnel', network: 'tcp' },
  { value: 'ssh', label: 'SSH', network: 'tcp' },
  { value: 'sshd', label: 'SSHD', network: 'tcp' },
  { value: 'kcp', label: 'KCP', network: 'udp' },
  { value: 'quic', label: 'QUIC', network: 'udp' },
  { value: 'h3', label: 'HTTP/3', network: 'udp' },
  { value: 'http3', label: 'HTTP/3 Proxy Channel', network: 'udp' },
  { value: 'wt', label: 'WebTransport', network: 'udp' },
  { value: 'ohttp', label: 'HTTP Obfuscation', network: 'tcp' },
  { value: 'otls', label: 'TLS Obfuscation', network: 'tcp' },
  { value: 'icmp', label: 'ICMPv4（本机 / 特权）', network: 'raw', localOnly: true },
  { value: 'icmp6', label: 'ICMPv6（本机 / 特权）', network: 'raw', localOnly: true },
  { value: 'ftcp', label: 'Fake TCP（本机 / 特权）', network: 'raw', localOnly: true },
];
const gostProtocol = value => GOST_PROTOCOLS.find(x => x.value === value) || GOST_PROTOCOLS[5];
const gostTransport = value => GOST_TRANSPORTS.find(x => x.value === value) || GOST_TRANSPORTS[0];

async function openNodeForm(node) {
  if (state.realityAvailable === undefined) {
    try { state.realityAvailable = (await api('api/reality/available')).available; }
    catch (e) { state.realityAvailable = false; }
  }
  const isEdit = !!node;
  const q = (node && node.quota) || { enabled: false, period: 'monthly', bytes: 0, direction: 'total' };
  const rate = (node && node.rate) || { enabled: false, inBps: 0, outBps: 0 };
  const isGost = !!(node && node.mode === 'gost');
  const isReality = !!(node && node.mode === 'reality');
  const initMode = isReality ? 'reality' : (isGost ? 'gost' : 'link');
  const rv = (v) => esc(isReality && node ? (node[v] || '') : '');
  const gp = (isGost && node.gostProtocol) || 'ss';
  const gt = (isGost && node.gostTransport) || gostProtocol(gp).transport;
  const gl = isGost ? !!node.gostLocal : true;
  const gc = (isGost && node.gostCipher) || 'aes-256-gcm';
  const cipherOpt = (v) => `<option value="${v}"${gc === v ? ' selected' : ''}>${v}</option>`;
  const protocolOpts = GOST_PROTOCOLS.map(x => `<option value="${x.value}"${gp === x.value ? ' selected' : ''}>${x.label}</option>`).join('');
  const transportOpts = GOST_TRANSPORTS.map(x => `<option value="${x.value}"${gt === x.value ? ' selected' : ''}>${x.label}</option>`).join('');

  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>${isEdit ? '编辑节点' : '添加节点'}</h2>

    <div class="tabs" id="nodeModeTabs" style="margin-bottom:16px">
      <button type="button" data-mode="link" class="${initMode === 'link' ? 'active' : ''}">粘贴链接</button>
      <button type="button" data-mode="gost" class="${initMode === 'gost' ? 'active' : ''}">GOST 体系</button>${state.realityAvailable || isReality ? `
      <button type="button" data-mode="reality" class="${initMode === 'reality' ? 'active' : ''}">VLESS + REALITY</button>` : ''}
    </div>

    <div id="modeLink" class="${initMode === 'link' ? '' : 'hidden'}">
    <div class="field">
      <label>落地机 v2rayN 链接</label>
      <textarea id="f-link" placeholder="粘贴 vmess:// / vless:// / trojan:// / ss:// / hysteria2:// / tuic:// 链接">${esc(!isGost && node ? node.landingLink : '')}</textarea>
      <div class="hint">粘贴落地机节点的分享链接，面板自动解析协议、地址与鉴权参数</div>
      <div class="btn-group" style="margin-top:12px">
        <button class="btn secondary sm" id="testBtn">测试落地机连通性</button>
        <span class="hint" id="testResult" style="margin:0"></span>
      </div>
    </div>
    <div id="parseBox"></div>
    </div>

    <div id="modeGost" class="${initMode === 'gost' ? '' : 'hidden'}">
      <div class="notice" id="gostModeNotice">GOST 原生落地支持本机直接运行，也支持远程落地后由当前服务器中转。代理协议与传输通道可独立组合。</div>
      <div class="row">
        <div class="field">
          <label>落地位置</label>
          <select id="f-gost-location">
            <option value="local" ${gl ? 'selected' : ''}>本机 · 由当前面板直接运行</option>
            <option value="remote" ${!gl ? 'selected' : ''}>远程落地机 · 当前服务器中转</option>
          </select>
        </div>
        <div class="field">
          <label>代理协议</label>
          <select id="f-gost-protocol">${protocolOpts}</select>
        </div>
      </div>
      <div class="row">
        <div class="field">
          <label>传输通道</label>
          <select id="f-gost-transport">${transportOpts}</select>
        </div>
        <div class="field" id="gostPathField">
          <label>通道路径</label>
          <input id="f-gost-path" type="text" value="${esc(isGost ? node.gostPath || '' : '')}" placeholder="如 /gost（留空使用 GOST 默认值）">
        </div>
      </div>
      <div class="row" id="gostRemoteFields">
        <div class="field">
          <label>远程落地机公网 IP / 域名</label>
          <input id="f-gost-host" type="text" value="${esc(isGost && !gl ? node.targetHost : '')}" placeholder="如 1.2.3.4 或 land.example.com">
        </div>
        <div class="field">
          <label>远程落地机端口</label>
          <input id="f-gost-port" type="number" min="1" max="65535" value="${isGost && !gl ? node.targetPort : ''}" placeholder="如 8388">
        </div>
      </div>
      <div class="row" id="gostSSAuth">
        <div class="field">
          <label>Shadowsocks 加密方式</label>
          <select id="f-gost-cipher">
            ${cipherOpt('aes-256-gcm')}
            ${cipherOpt('aes-128-gcm')}
            ${cipherOpt('chacha20-ietf-poly1305')}
          </select>
        </div>
        <div class="field">
          <label>密码</label>
          <div class="row tight">
            <input id="f-gost-ss-pass" type="text" value="${esc(isGost ? node.gostPassword : '')}" placeholder="留空自动生成">
            <button class="btn secondary sm" id="gostSSRandBtn" type="button" style="flex:0 0 auto">随机</button>
          </div>
        </div>
      </div>
      <div class="row" id="gostUserAuth">
        <div class="field">
          <label>用户名</label>
          <input id="f-gost-user" type="text" value="${esc(isGost ? node.gostUsername || '' : '')}" placeholder="留空使用 gost">
        </div>
        <div class="field" id="gostPasswordField">
          <label>密码</label>
          <div class="row tight">
            <input id="f-gost-pass" type="text" value="${esc(isGost ? node.gostPassword : '')}" placeholder="留空自动生成">
            <button class="btn secondary sm" id="gostRandBtn" type="button" style="flex:0 0 auto">随机</button>
          </div>
        </div>
      </div>
      <div class="notice" id="gostComboHint"></div>
    </div>

    <div id="modeReality" class="${initMode === 'reality' ? '' : 'hidden'}">
      <div class="notice">REALITY 由本机 sing-box 进程承载：伪装成目标网站 TLS 握手，无证书、抗探测。监听端口即客户端连接端口。</div>
      <div class="field">
        <label>伪装域名（SNI / 握手目标）</label>
        <input id="f-reality-sni" type="text" value="${rv('realitySni')}" placeholder="如 www.microsoft.com（必须支持 TLS1.3，客户端 SNI 与伪装目标一致）">
        <div class="hint">建议使用大型站点（微软/苹果/亚马逊等），不要用被墙的域名</div>
      </div>
      <div class="row">
        <div class="field">
          <label>UUID</label>
          <input id="f-reality-uuid" type="text" value="${rv('realityUuid')}" placeholder="点击右侧生成">
        </div>
        <div class="field">
          <label>Short ID</label>
          <input id="f-reality-sid" type="text" value="${rv('realityShortId')}" placeholder="8 位 hex">
        </div>
      </div>
      <div class="field">
        <label>REALITY 密钥对</label>
        <input id="f-reality-pub" type="text" value="${rv('realityPub')}" placeholder="公钥（pbk，下发给客户端）" readonly>
        <input id="f-reality-priv" type="hidden" value="${rv('realityPriv')}">
        <div class="btn-group" style="margin-top:8px">
          <button class="btn secondary sm" id="realityGenBtn" type="button">生成密钥对 / UUID / ShortID</button>
          <span class="hint" id="realityGenHint" style="margin:0">${isReality ? '编辑时留空表示保留现有凭据' : ''}</span>
        </div>
      </div>
      <div class="hint" style="margin-top:4px">REALITY 节点支持流量统计与配额（超限自动停实例、恢复周期自动拉起）；限速与并发限制暂不支持。</div>
    </div>

    <div class="row">
      <div class="field">
        <label>监听端口</label>
        <div class="row tight">
          <input id="f-port" type="number" min="1" max="65535" value="${node ? node.listenPort : ''}" placeholder="留空自动分配">
          <button class="btn secondary sm" id="randPortBtn" style="flex:0 0 auto">随机</button>
        </div>
      </div>
      <div class="field">
        <label>备注名</label>
        <input id="f-name" type="text" value="${esc(node ? node.name : '')}" placeholder="默认取链接备注">
      </div>
    </div>

    <div class="field" id="udpField">
      <label class="switch"><input type="checkbox" id="f-udp" ${node && node.udp ? 'checked' : ''}><span class="track"></span>
        <span><span class="txt" id="udpTitle">同时转发 UDP</span><span class="desc" id="udpDesc">hysteria2 / tuic / KCP / QUIC 等协议必须开启</span></span></label>
    </div>

    <details id="advOverride" class="${initMode === 'link' ? '' : 'hidden'}">
      <summary class="hint" style="cursor:pointer">高级：SNI / Host 覆盖（一般无需修改）</summary>
      <div class="row" style="margin-top:12px">
        <div class="field"><label>SNI 覆盖</label><input id="f-sni" type="text" value="${esc(node ? node.sni || '' : '')}" placeholder="留空自动"></div>
        <div class="field"><label>Host 覆盖</label><input id="f-host" type="text" value="${esc(node ? node.host || '' : '')}" placeholder="留空自动"></div>
      </div>
    </details>

    <div class="section-label">流量限制</div>
    <div class="field">
      <label class="switch"><input type="checkbox" id="f-quota" ${q.enabled ? 'checked' : ''}><span class="track"></span>
        <span><span class="txt">启用流量配额</span><span class="desc">达到额度后自动暂停该节点，进入新周期自动恢复</span></span></label>
    </div>
    <div id="quotaBox" class="${q.enabled ? '' : 'hidden'}">
      <div class="row">
        <div class="field"><label>周期</label>
          <select id="f-period">
            <option value="daily" ${q.period === 'daily' ? 'selected' : ''}>每日重置</option>
            <option value="monthly" ${q.period === 'monthly' ? 'selected' : ''}>每月重置</option>
            <option value="total" ${q.period === 'total' ? 'selected' : ''}>总量（不重置）</option>
          </select>
        </div>
        <div class="field"><label>额度</label>
          <div class="row tight quota-row">
            <input id="f-quotaSize" type="number" min="0" step="0.1" value="${q.bytes ? (q.bytes / Math.pow(1024, 3)).toFixed(2) : ''}" placeholder="如 100">
            <select id="f-quotaUnit" style="flex:0 0 74px">
              <option value="1024">GB</option>
              <option value="1048576">TB</option>
              <option value="1">MB</option>
            </select>
          </div>
        </div>
        <div class="field"><label>计数方向</label>
          <select id="f-direction">
            <option value="total" ${q.direction === 'total' ? 'selected' : ''}>双向合计</option>
            <option value="in" ${q.direction === 'in' ? 'selected' : ''}>仅上行</option>
            <option value="out" ${q.direction === 'out' ? 'selected' : ''}>仅下行</option>
          </select>
        </div>
      </div>
    </div>

    <div id="rateConnSection">
    <div class="section-label">限速与连接数</div>
    <div class="field">
      <label class="switch"><input type="checkbox" id="f-rate" ${rate.enabled ? 'checked' : ''}><span class="track"></span>
        <span><span class="txt">启用限速</span><span class="desc">按节点限制上下行速率</span></span></label>
    </div>
    <div id="rateBox" class="${rate.enabled ? '' : 'hidden'}">
      <div class="row">
        <div class="field"><label>上行限速（Mbps）</label><input id="f-rateIn" type="number" min="0" step="0.1" value="${rate.inBps ? (rate.inBps * 8 / 1e6).toFixed(1) : ''}" placeholder="不限填 0"></div>
        <div class="field"><label>下行限速（Mbps）</label><input id="f-rateOut" type="number" min="0" step="0.1" value="${rate.outBps ? (rate.outBps * 8 / 1e6).toFixed(1) : ''}" placeholder="不限填 0"></div>
      </div>
    </div>
    <div class="field">
      <label>最大并发连接数（0 表示不限）</label>
      <input id="f-connLimit" type="number" min="0" value="${node ? node.connLimit || 0 : 0}">
    </div>
    </div>

    <div class="modal-actions">
      <button class="btn secondary" onclick="closeModal()">取消</button>
      <button class="btn" id="saveNodeBtn">${isEdit ? '保存修改' : '创建节点'}</button>
    </div>
  `, true);

  $('#f-link').addEventListener('input', debounce(() => parseLink(true), 500));
  const updateGostFields = (resetTransport = false) => {
    const proto = gostProtocol($('#f-gost-protocol').value);
    if (resetTransport) $('#f-gost-transport').value = proto.transport;
    const transport = gostTransport($('#f-gost-transport').value);
    const local = $('#f-gost-location').value === 'local';
    const isSS = proto.auth === 'ss';
    const channelAuth = transport.value === 'ssh' || transport.value === 'sshd';
    const needsUser = !isSS && (proto.auth !== 'none' || channelAuth);

    $('#gostRemoteFields').classList.toggle('hidden', local);
    $('#gostSSAuth').classList.toggle('hidden', !isSS);
    $('#gostUserAuth').classList.toggle('hidden', !needsUser);
    $('#gostPasswordField').classList.toggle('hidden', proto.auth === 'user' && !channelAuth);
    $('#gostPathField').classList.toggle('hidden', !transport.path);

    const udpAddon = proto.value === 'ss' && transport.network === 'tcp';
    $('#udpField').classList.toggle('hidden', !udpAddon);
    if (!udpAddon) $('#f-udp').checked = false;
    $('#udpTitle').textContent = '同时提供 Shadowsocks UDP';
    $('#udpDesc').textContent = '在同一端口额外启动 SSU 服务；TCP 与 UDP 可同时使用';

    const warnings = [];
    if (transport.localOnly && !local) warnings.push(`${transport.label} 使用原始网络报文，只支持本机落地`);
    if (transport.localOnly) warnings.push('运行需要 root 或 CAP_NET_RAW 权限');
    if (transport.value === 'dtls') warnings.push('GOST DTLS 客户端还需自行配置 certFile / keyFile');
    if (transport.value === 'udp' && !['ssu', 'relay'].includes(proto.value)) warnings.push('UDP 通道只支持 Shadowsocks UDP 或 GOST Relay');
    if (proto.value === 'http2' && transport.value !== 'http2') warnings.push('HTTP/2 代理协议必须使用 HTTP/2 Proxy Channel');
    if (isSS && channelAuth) warnings.push('Shadowsocks 与 SSH/SSHD 不能共用一组认证信息');
    const where = local ? '服务将直接加入当前面板管理的 GOST 进程' : `当前服务器将按 ${transport.network.toUpperCase()} 原样中转到远程落地`;
    const authText = proto.auth === 'none' && !channelAuth ? '该协议本身不提供账号认证，请配合防火墙限制来源' : '凭据留空时由后端自动生成';
    $('#gostComboHint').className = `notice${warnings.length ? ' warn' : ''}`;
    $('#gostComboHint').innerHTML = `${esc(where)}。${esc(authText)}。${warnings.length ? '<br><b>' + esc(warnings.join('；')) + '</b>' : ''}`;
  };
  const updateModeUI = () => {
    const mode = $('#nodeModeTabs button.active')?.dataset.mode || 'link';
    if (mode === 'reality') {
      $('#udpField').classList.add('hidden');
      $('#rateConnSection').classList.add('hidden');
      return;
    }
    $('#rateConnSection').classList.remove('hidden');
    if (mode === 'gost') {
      updateGostFields(false);
    } else {
      $('#udpField').classList.remove('hidden');
      $('#udpField').classList.remove('hidden');
      $('#udpTitle').textContent = '同时转发 UDP';
      $('#udpDesc').textContent = 'hysteria2 / tuic / KCP / QUIC 等协议必须开启';
    }
  };
  $$('#nodeModeTabs button').forEach(b => {
    b.onclick = () => {
      const mode = b.dataset.mode;
      $$('#nodeModeTabs button').forEach(x => x.classList.toggle('active', x === b));
      const isLink = mode === 'link';
      $('#modeLink').classList.toggle('hidden', !isLink);
      $('#advOverride').classList.toggle('hidden', !isLink);
      $('#modeGost').classList.toggle('hidden', mode !== 'gost');
      const mr = $('#modeReality'); if (mr) mr.classList.toggle('hidden', mode !== 'reality');
      updateModeUI();
    };
  });
  const rg = $('#realityGenBtn');
  if (rg) rg.onclick = async () => {
    try {
      const c = await api('api/reality/credential', { method: 'POST' });
      $('#f-reality-uuid').value = c.uuid;
      $('#f-reality-sid').value = c.shortId;
      $('#f-reality-pub').value = c.pubKey;
      $('#f-reality-priv').value = c.privKey;
      $('#realityGenHint').textContent = '已生成，保存前请勿再次点击（会更换全部凭据）';
    } catch (e) { toast(e.message, 'err'); }
  };
  $('#f-gost-location').onchange = () => updateGostFields(false);
  $('#f-gost-protocol').onchange = () => updateGostFields(true);
  $('#f-gost-transport').onchange = () => updateGostFields(false);
  $('#gostSSRandBtn').onclick = () => { $('#f-gost-ss-pass').value = randPass(20); };
  $('#gostRandBtn').onclick = () => { $('#f-gost-pass').value = randPass(20); };
  $('#randPortBtn').onclick = async () => {
    try { const r = await api('api/ports/free'); $('#f-port').value = r.port; }
    catch (e) { toast(e.message, 'err'); }
  };
  $('#f-quota').onchange = e => $('#quotaBox').classList.toggle('hidden', !e.target.checked);
  $('#f-rate').onchange = e => $('#rateBox').classList.toggle('hidden', !e.target.checked);
  $('#testBtn').onclick = async () => {
    const el = $('#testResult');
    const info = state.lastParsed && state.lastParsed.raw === $('#f-link').value.trim()
      ? state.lastParsed : await parseLink(false);
    if (!info) { el.textContent = '请先粘贴可解析的链接'; return; }
    el.textContent = '测试中…';
    try {
      const r = await api('api/test-connect', { method: 'POST', body: { host: info.host, port: info.port } });
      el.textContent = r.ok ? `落地机可达，延迟 ${r.latencyMs} ms` : `不可达：${r.error}`;
      el.className = r.ok ? 'hint' : 'hint error-text';
    } catch (e) { el.textContent = e.message; el.className = 'hint error-text'; }
  };
  updateModeUI();
  if (node && initMode === 'link') parseLink(false);
  $('#saveNodeBtn').onclick = () => saveNode(node);
}

// randPass 生成前端展示用的随机密码（留空时后端也会自动生成）。
function randPass(n) {
  const alphabet = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  const buf = new Uint32Array(n);
  (window.crypto || window.msCrypto).getRandomValues(buf);
  let s = '';
  for (let i = 0; i < n; i++) s += alphabet[buf[i] % alphabet.length];
  return s;
}

function debounce(fn, ms) {
  let t = null;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
}

async function parseLink(autofill) {
  const link = $('#f-link').value.trim();
  const box = $('#parseBox');
  if (!link) { box.innerHTML = ''; return null; }
  try {
    const info = await api('api/parse', { method: 'POST', body: { link } });
    state.lastParsed = info;
    box.innerHTML = `<div class="notice">已解析：<b>${esc(info.scheme)}</b> · ${esc(info.host)}:${info.port}
      · 传输 ${esc(info.transport)}${info.tls ? ' + TLS' : ''}${info.udp ? ' · UDP' : ''}${info.name ? ' · 备注 ' + esc(info.name) : ''}</div>`;
    if (autofill) {
      $('#f-udp').checked = !!info.udp;
      if (!$('#f-name').value) $('#f-name').value = (info.name || info.host) + '-节点';
    }
    return info;
  } catch (e) {
    box.innerHTML = `<div class="notice err">${esc(e.message)}</div>`;
    return null;
  }
}

async function saveNode(node) {
  const btn = $('#saveNodeBtn');
  const activeTab = $('#nodeModeTabs button.active');
  const mode = activeTab ? activeTab.dataset.mode : 'link';
  const quotaEnabled = $('#f-quota').checked;
  const quotaSize = parseFloat($('#f-quotaSize').value || '0');
  const quotaUnit = parseInt($('#f-quotaUnit').value || '1024', 10);
  const rateEnabled = $('#f-rate').checked;
  const rateIn = parseFloat($('#f-rateIn').value || '0');
  const rateOut = parseFloat($('#f-rateOut').value || '0');
  if (quotaEnabled && !(quotaSize > 0)) { toast('请填写有效的流量额度', 'err'); return; }

  const quota = {
    enabled: quotaEnabled,
    period: $('#f-period').value,
    bytes: quotaEnabled ? Math.round(quotaSize * quotaUnit * 1024 * 1024) : 0,
    direction: $('#f-direction').value,
  };
  const rate = {
    enabled: rateEnabled,
    inBps: rateEnabled ? Math.round(rateIn * 1e6 / 8) : 0,
    outBps: rateEnabled ? Math.round(rateOut * 1e6 / 8) : 0,
  };
  const common = {
    name: $('#f-name').value.trim(),
    listenPort: parseInt($('#f-port').value || '0', 10) || 0,
    udp: $('#f-udp').checked,
    connLimit: parseInt($('#f-connLimit').value || '0', 10) || 0,
    quota,
    rate,
  };

  let body;
  if (mode === 'gost') {
    const local = $('#f-gost-location').value === 'local';
    const proto = $('#f-gost-protocol').value;
    const transport = gostTransport($('#f-gost-transport').value);
    const th = $('#f-gost-host').value.trim();
    const tp = parseInt($('#f-gost-port').value || '0', 10) || 0;
    if (!local && !th) { toast('请填写远程落地机公网 IP / 域名', 'err'); return; }
    if (!local && !(tp > 0 && tp <= 65535)) { toast('请填写有效的远程落地机端口', 'err'); return; }
    if (!local && transport.localOnly) { toast(`${transport.label} 只能作为本机落地`, 'err'); return; }
    if (transport.value === 'udp' && !['ssu', 'relay'].includes(proto)) { toast('UDP 通道只支持 Shadowsocks UDP 或 GOST Relay', 'err'); return; }
    if (proto === 'http2' && transport.value !== 'http2') { toast('HTTP/2 代理协议必须使用 HTTP/2 Proxy Channel', 'err'); return; }
    if (['ss', 'ssu'].includes(proto) && ['ssh', 'sshd'].includes(transport.value)) { toast('Shadowsocks 不能与 SSH/SSHD 通道组合', 'err'); return; }
    body = Object.assign({
      mode: 'gost',
      gostLocal: local,
      targetHost: local ? '' : th,
      targetPort: local ? 0 : tp,
      gostProtocol: proto,
      gostTransport: transport.value,
      gostUsername: $('#f-gost-user').value.trim(),
      gostCipher: $('#f-gost-cipher').value,
      gostPassword: ['ss', 'ssu'].includes(proto) ? $('#f-gost-ss-pass').value.trim() : $('#f-gost-pass').value.trim(),
      gostPath: $('#f-gost-path').value.trim(),
    }, common);
  } else if (mode === 'reality') {
    const sni = $('#f-reality-sni').value.trim();
    if (!sni) { toast('请填写伪装域名', 'err'); return; }
    body = Object.assign({
      mode: 'reality',
      udp: false,
      realitySni: sni,
      realityUuid: $('#f-reality-uuid').value.trim(),
      realityShortId: $('#f-reality-sid').value.trim(),
      realityPub: $('#f-reality-pub').value.trim(),
      realityPriv: $('#f-reality-priv').value.trim(),
    }, Object.assign({}, common, { udp: false }));
  } else {
    const link = $('#f-link').value.trim();
    if (!link) { toast('请填写落地机链接', 'err'); return; }
    body = Object.assign({
      mode: 'link',
      link,
      sni: $('#f-sni').value.trim(),
      host: $('#f-host').value.trim(),
    }, common);
  }
  btn.disabled = true;
  try {
    const saved = node
      ? await api(`api/nodes/${node.id}`, { method: 'PUT', body })
      : await api('api/nodes', { method: 'POST', body });
    toast(node ? '已保存' : '节点已创建');
    await refreshAll();
    openLinkModal(saved);
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; }
}

/* ---------------- 链接 / 详情 ---------------- */
async function openLinkModal(node) {
  let r = null;
  try { r = await api(`api/nodes/${node.id}/link`); } catch (e) { toast(e.message, 'err'); return; }
  const gostNative = node.mode === 'gost' && !(['ss'].includes(node.gostProtocol || 'ss') && (node.gostTransport || 'tcp') === 'tcp');
  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>客户端链接 · ${esc(node.name)}</h2>
    ${state.publicHostPrivate ? `<div class="notice warn">当前服务器地址 ${esc(r.host)} 是内网地址，客户端无法连接，请到「系统设置」填写公网 IP 或域名。</div>` : ''}
    <div class="notice">${gostNative ? '把下面地址用作 GOST 客户端的上游节点（<span class="mono">gost -F 地址</span>）' : '把下面链接导入兼容客户端'}，连接地址为本服务器 <b>${esc(r.host)}:${r.port}</b>。</div>
    <div class="code-box" id="linkText">${esc(r.url)}</div>
    <div style="display:flex;gap:24px;margin-top:24px;align-items:flex-start;flex-wrap:wrap">
      <button class="btn" id="copyLinkBtn">${icon('copy')} 复制链接</button>
      <div>
        <div class="qr"><img src="${BASE}api/nodes/${node.id}/qrcode?t=${Date.now()}" alt="二维码"></div>
        <div class="hint" style="text-align:center">扫码导入</div>
      </div>
    </div>
    ${node.mode === 'gost' ? '<div id="deployBox"></div>' : ''}
  `);
  $('#copyLinkBtn').onclick = () => copyText(r.url);
  if (node.mode === 'gost') renderDeploy(node, '#deployBox');
}

// renderDeploy 拉取并渲染 GOST 体系节点的落地机部署命令与步骤。
async function renderDeploy(node, sel) {
  const box = $(sel);
  if (!box) return;
  box.innerHTML = '<div class="loading">加载落地机部署命令…</div>';
  let d;
  try { d = await api(`api/nodes/${node.id}/deploy`); }
  catch (e) { box.innerHTML = `<div class="notice err">${esc(e.message)}</div>`; return; }
  if (d.managed) {
    box.innerHTML = `
      <div class="section-label">本机落地（GOST 体系）</div>
      <div class="notice">该服务已由当前面板直接管理，无需另装或另起 GOST。协议 <b>${esc(d.protocol.toUpperCase())}</b> · 通道 <b>${esc(d.transport.toUpperCase())}</b> · 端口 <b>${d.port}</b>。</div>
      <div class="notice" style="margin-top:12px">${(d.steps || []).map(s => esc(s)).join('<br>')}</div>
      <details style="margin-top:16px">
        <summary class="hint" style="cursor:pointer">查看面板生成的服务配置</summary>
        <pre class="code-box command-block" style="margin-top:10px"><code>${esc(d.config)}</code></pre>
      </details>`;
    return;
  }
  box.innerHTML = `
    <div class="section-label">远程落地机部署（GOST 体系）</div>
    <div class="notice">在远程落地机运行 GOST 服务。协议 <b>${esc(d.protocol.toUpperCase())}</b> · 通道 <b>${esc(d.transport.toUpperCase())}</b> · 端口 <b>${d.port}</b>${d.udp ? ' · 附加 SSU' : ''}。</div>
    <div class="hint" style="margin-top:12px">操作步骤</div>
    <div class="notice">${(d.steps || []).map(s => esc(s)).join('<br>')}</div>

    <div class="hint" style="margin-top:16px">① 安装 GOST · 方式一：下载预编译二进制（自动识别架构）</div>
    <pre class="code-box command-block"><code>${esc(d.installBinaryCmd)}</code></pre>
    <button class="btn secondary sm" id="copyInstallBinBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">① 安装 GOST · 方式二：官方安装脚本</div>
    <pre class="code-box command-block"><code>${esc(d.installScriptCmd)}</code></pre>
    <button class="btn secondary sm" id="copyInstallScriptBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">② 保存代理服务配置到 ${esc(d.confPath)}</div>
    <pre class="code-box command-block"><code>${esc(d.saveCommand)}</code></pre>
    <button class="btn secondary sm" id="copySaveBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">③ 注册为 systemd 后台服务（自动后台运行 + 开机自启）</div>
    <pre class="code-box command-block"><code>${esc(d.serviceCommand)}</code></pre>
    <button class="btn secondary sm" id="copyServiceBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">（可选）前台临时运行，用于快速测试</div>
    <pre class="code-box command-block"><code>${esc(d.runCommand)}</code></pre>
    <button class="btn secondary sm" id="copyRunBtn" style="margin-top:8px">${icon('copy')} 复制</button>
  `;
  $('#copyInstallBinBtn').onclick = () => copyText(d.installBinaryCmd);
  $('#copyInstallScriptBtn').onclick = () => copyText(d.installScriptCmd);
  $('#copySaveBtn').onclick = () => copyText(d.saveCommand);
  $('#copyServiceBtn').onclick = () => copyText(d.serviceCommand);
  $('#copyRunBtn').onclick = () => copyText(d.runCommand);
}

async function openDetail(node) {
  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>${esc(node.name)}</h2>
    <div id="detailBody"><div class="loading">加载中…</div></div>
    <div class="modal-actions">
      <button class="btn secondary" id="d-reset">重置流量</button>
      <button class="btn secondary" id="d-edit">编辑</button>
      <button class="btn" id="d-copy">复制链接</button>
    </div>
  `, true);
  $('#d-edit').onclick = () => openNodeForm(node);
  $('#d-copy').onclick = async () => {
    const r = await api(`api/nodes/${node.id}/link`).catch(e => { toast(e.message, 'err'); return null; });
    if (r && r.url) copyText(r.url);
  };
  $('#d-reset').onclick = async () => {
    if (!confirm('重置该节点的流量统计与配额计数？')) return;
    try {
      await api(`api/nodes/${node.id}/reset`, { method: 'POST' });
      toast('已重置');
      refreshAll();
      openDetail(node);
    } catch (e) { toast(e.message, 'err'); }
  };

  const body = $('#detailBody');
  const lv = node.live || {};
  const loadStats = async (range) => {
    range = range || '7d';
    const r = await api(`api/nodes/${node.id}/stats?range=${range}`).catch(e => { toast(e.message, 'err'); return null; });
    if (!r) return;
    const live = r.live || {};
    const used = live.quotaUsed || 0;
    const quotaHtml = node.quota && node.quota.enabled && node.quota.bytes
      ? `<div class="meter"><i style="width:${Math.min(100, used / node.quota.bytes * 100)}%"></i></div>
         <div class="hint">已用 ${fmtBytes(used)} / ${fmtBytes(node.quota.bytes)}
         · ${live.quotaUntil ? '周期结束 ' + fmtTime(live.quotaUntil) : '不重置'}
         ${live.quotaBlocked ? ' · <span class="error-text">已超限暂停</span>' : ''}</div>`
      : '<span class="tertiary">未启用流量配额</span>';
    body.innerHTML = `
      <div class="kv" style="margin-bottom:24px">
        <div class="k">状态</div><div class="v">${statusTag(node.enabled, live.quotaBlocked)}</div>
        <div class="k">落地机</div><div class="v mono">${node.mode === 'reality' ? '本机 sing-box（REALITY）' : (node.mode === 'gost' && node.gostLocal ? '本机直出' : `${esc(node.targetHost)}:${node.targetPort}`)} · ${esc(node.protocol)}</div>
        <div class="k">监听端口</div><div class="v mono">${node.listenPort}${node.udp ? ' · 附加 UDP' : ''}</div>
        <div class="k">连接数</div><div class="v">当前 ${live.currentConns || 0} · 累计 ${live.totalConns || 0}</div>
        <div class="k">今日流量</div><div class="v mono">↑ ${fmtBytes(node.todayIn)} / ↓ ${fmtBytes(node.todayOut)}</div>
        <div class="k">本月流量</div><div class="v mono">↑ ${fmtBytes(node.monthIn)} / ↓ ${fmtBytes(node.monthOut)}</div>
        <div class="k">累计流量</div><div class="v mono">↑ ${fmtBytes(node.totalIn)} / ↓ ${fmtBytes(node.totalOut)}</div>
        <div class="k">流量配额</div><div class="v">${quotaHtml}</div>
      </div>
      <div class="card-head">
        <div><h2 style="font-size:15px">流量曲线</h2></div>
        <div class="tabs">
          <button data-range="24h" class="${range === '24h' ? 'active' : ''}">24 小时</button>
          <button data-range="7d" class="${range === '7d' ? 'active' : ''}">7 天</button>
          <button data-range="30d" class="${range === '30d' ? 'active' : ''}">30 天</button>
        </div>
      </div>
      <div class="chart-wrap">
        <canvas id="detailChart" height="130" style="width:100%"></canvas>
      </div>
      ${node.mode === 'gost' ? '<div id="detailDeploy"></div>' : ''}`;
    drawChart($('#detailChart'), r.points || [], { step: r.step, hourAxis: range === '24h' });
    $$('#detailBody .tabs button').forEach(b => {
      b.onclick = () => loadStats(b.dataset.range);
    });
    if (node.mode === 'gost') renderDeploy(node, '#detailDeploy');
  };
  loadStats('7d');
}

/* ---------------- 按节点订阅 ---------------- */
// openNodeSub 拉取该节点专属订阅信息，弹窗展示链接 + 二维码 + 复制 + 重置令牌。
async function openNodeSub(node) {
  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>订阅 · ${esc(node.name)}</h2>
    <div id="subBody"><div class="loading">加载中…</div></div>
  `, true);
  const body = $('#subBody');
  const r = await api(`api/nodes/${node.id}/subscription`).catch(e => { toast(e.message, 'err'); return null; });
  if (!r) { body.innerHTML = '<div class="notice warn">无法生成订阅链接。</div>'; return; }
  const t = Date.now();
  const qr = u => `${BASE}api/qrcode?text=${encodeURIComponent(u)}&t=${t}`;
  const schemeNote = r.scheme === 'https'
    ? '已通过 <b>HTTPS</b> 提供（证书就绪）。'
    : '当前为 <b>HTTP</b>。如需 HTTPS，请到「系统设置 → 订阅与证书」配置域名并申请证书。';
  const clashItem = r.clashSupported ? `
      <div class="sub-item">
        <div class="hint">Clash Meta / mihomo（强制 YAML）</div>
        <div class="code-box">${esc(r.clashURL)}</div>
        <div class="sub-qr">
          <div class="qr"><img src="${qr(r.clashURL)}" alt="Clash 订阅二维码"></div>
          <button class="btn secondary sm" data-copy="${esc(r.clashURL)}">复制链接</button>
        </div>
      </div>` : `
      <div class="sub-item">
        <div class="hint">GOST 原生协议</div>
        <div class="notice">当前协议或通道需要 GOST 客户端。通用订阅中已包含可传给 <span class="mono">gost -F</span> 的节点地址。</div>
      </div>`;
  const linkR = await api(`api/nodes/${node.id}/link`).catch(() => null);
  const clientBlock = `
    <div class="section-label">客户端连接</div>
    <div class="sub-item">
      <div class="hint">节点连接地址 <span class="mono">${esc(state.publicHost)}:${node.listenPort}</span></div>
      <div class="code-box">${linkR && linkR.url ? esc(linkR.url) : '（无法生成：请在系统设置中填写服务器地址）'}</div>
      <div class="sub-qr">
        <div class="qr"><img src="${BASE}api/nodes/${node.id}/qrcode?t=${t}" alt="客户端连接二维码"></div>
        ${linkR && linkR.url ? `<button class="btn secondary sm" data-copy="${esc(linkR.url)}">复制连接地址</button>` : ''}
      </div>
    </div>`;
  body.innerHTML = `
    <div class="notice">${r.enabled ? '' : '<b>该节点已停用，订阅内容为空。</b> '}${schemeNote}</div>
    ${clientBlock}
    <div class="sub-grid">
      <div class="sub-item">
        <div class="hint">通用订阅（自动识别 Clash / v2rayN）</div>
        <div class="code-box">${esc(r.url)}</div>
        <div class="sub-qr">
          <div class="qr"><img src="${qr(r.url)}" alt="通用订阅二维码"></div>
          <button class="btn secondary sm" data-copy="${esc(r.url)}">复制链接</button>
        </div>
      </div>
      ${clashItem}
    </div>
    <div class="section-label">通用直链（v2rayN / Shadowrocket）</div>
    <div class="code-box">${esc(r.universalURL)}</div>
    <div class="modal-actions">
      <button class="btn secondary" id="subResetToken">重置令牌</button>
      <button class="btn secondary" data-copy="${esc(r.universalURL)}">复制直链</button>
    </div>`;
  $$('#subBody [data-copy]').forEach(b => b.onclick = () => copyText(b.dataset.copy));
  $('#subResetToken').onclick = async () => {
    if (!confirm('重置该节点订阅令牌？旧的订阅链接与二维码将立即失效。')) return;
    try {
      await api(`api/nodes/${node.id}/subscription/reset`, { method: 'POST' });
      toast('已重置订阅令牌');
      openNodeSub(node);
    } catch (e) { toast(e.message, 'err'); }
  };
}

/* ---------------- 通知设置 ---------------- */
async function loadNotify(force) {
  if (state.notify && !force) { renderNotify(); return; }
  try {
    state.notify = await api('api/notify');
    renderNotify();
  } catch (e) { toast(e.message, 'err'); }
}

function renderNotify() {
  const n = state.notify;
  if (!n) return;
  $('#tgEnabled').checked = !!n.enabled;
  $('#tgToken').value = '';
  $('#tgToken').placeholder = n.hasToken ? `已保存（${n.token}），留空不修改` : '123456:ABC-DEF...';
  $('#tgChat').value = n.chatId || '';
  $('#tgApiBase').value = n.apiBase || 'https://api.telegram.org';
  $('#tgCooldown').value = n.cooldown || 600;
  $('#thTraffic').value = (n.trafficThresholds || []).join(',');
  $('#thCPU').value = n.cpu;
  $('#thMem').value = n.mem;
  $('#thDisk').value = n.disk;

  const names = n.allEvents || {};
  const ev = n.events || {};
  $('#tgEvents').innerHTML = Object.keys(names).map(k => `
    <label class="switch"><input type="checkbox" data-event="${k}" ${ev[k] !== false ? 'checked' : ''}><span class="track"></span>
      <span><span class="txt">${esc(names[k])}</span></span></label>`).join('');
}

$('#tgSaveBtn').onclick = async () => {
  const events = {};
  $$('#tgEvents input[data-event]').forEach(i => { events[i.dataset.event] = i.checked; });
  const thresholds = $('#thTraffic').value.split(',').map(s => parseInt(s.trim(), 10)).filter(v => v > 0 && v < 100);
  const body = {
    enabled: $('#tgEnabled').checked,
    chatId: $('#tgChat').value.trim(),
    apiBase: $('#tgApiBase').value.trim(),
    cooldown: parseInt($('#tgCooldown').value || '600', 10),
    events,
    trafficThresholds: thresholds.length ? thresholds : [80, 95],
    cpu: parseFloat($('#thCPU').value || '0'),
    mem: parseFloat($('#thMem').value || '0'),
    disk: parseFloat($('#thDisk').value || '0'),
  };
  const token = $('#tgToken').value.trim();
  if (token) body.token = token;
  try {
    await api('api/notify', { method: 'PUT', body });
    toast('通知设置已保存');
    state.notify = null;
    await loadNotify(true);
  } catch (e) { toast(e.message, 'err'); }
};

$('#tgTestBtn').onclick = async () => {
  const btn = $('#tgTestBtn');
  btn.disabled = true;
  try {
    // 先保存再测试，避免"填了没保存"的困惑
    await $('#tgSaveBtn').onclick();
    await api('api/notify/test', { method: 'POST' });
    toast('测试通知已发送，请查看 Telegram');
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; }
};

/* ---------------- 系统设置 ---------------- */
async function loadSystem() {
  try {
    const [sys, st, sc] = await Promise.all([
      api('api/system'), api('api/settings'),
      api('api/sub-config').catch(() => null),
    ]);
    state.system = sys;
    state.settings = st;
    state.subConfig = sc;
    renderSystem();
  } catch (e) { toast(e.message, 'err'); }
}

function renderSystem() {
  const sys = state.system, st = state.settings;
  if (!sys || !st) return;
  const p = sys.panel || {};
  const met = (sys.host && sys.host.metrics) || {};

  if (document.activeElement !== $('#sysListen')) {
    const port = String(p.listen || '').match(/:(\d+)$/)?.[1] || String(p.listen || '').match(/^\d+$/)?.[0] || '';
    $('#sysListen').value = port;
  }
  if (document.activeElement !== $('#sysBasePath')) $('#sysBasePath').value = p.basePath || '';

  const host = location.host;
  const preview = `${p.basePath || ''}/`;
  $('#sysAccessInfo').innerHTML = `当前访问地址：<b>${esc(host + preview)}</b> · 进程启动于 ${fmtTime(p.startedAt)}（已运行 ${fmtDuration(p.uptime)}）`;

  if (document.activeElement !== $('#setHost')) $('#setHost').value = st.configured || '';
  $('#setHost').placeholder = '自动探测（当前 ' + (sys.publicHost || '未知') + '）';
  if (document.activeElement !== $('#setSample')) $('#setSample').value = st.sampleSeconds;
  if (document.activeElement !== $('#setRetention')) $('#setRetention').value = st.retentionDays;
  if (document.activeElement !== $('#setLogLevel') && st.gostLogLevel) $('#setLogLevel').value = st.gostLogLevel;

  const g = sys.gost || {};
  const proc = g.process || {};
  $('#gostInfo').innerHTML = `
    <div class="k">状态</div><div class="v">${g.reachable === false ? '<span class="status-inline"><span class="status-dot bad"></span>API 不可达</span>' : (proc.running ? '<span class="status-inline"><span class="status-dot ok"></span>运行中</span>' : '<span class="status-inline"><span class="status-dot bad"></span>未运行</span>')}</div>
    <div class="k">PID</div><div class="v mono">${proc.pid || '—'}</div>
    <div class="k">重启次数</div><div class="v">${proc.restarts || 0}</div>
    <div class="k">最近退出</div><div class="v mono">${esc(proc.lastExit || '—')}</div>`;
  $('#gostPaths').textContent = `${g.bin || ''} · ${g.configFile || ''} · ${g.logFile || ''}`;

  const fv = $('#footerVersion'); if (fv) fv.textContent = 'Gost-WebUI' + (p.version ? ' v' + String(p.version).replace(/^v/, '') : '');
  $('#aboutInfo').innerHTML = `
    <div class="k">版本</div><div class="v mono">gost-webui ${esc(p.version || '')} (${esc(p.runtimeOS || '')}/${esc(p.runtimeArch || '')})</div>
    <div class="k">主机运行</div><div class="v">${fmtDuration(met.hostUptime)}</div>
    <div class="k">配置文件</div><div class="v mono">${esc(p.configFile || '')}</div>
    <div class="k">面板账号</div><div class="v mono">${esc(st.username || 'admin')}</div>`;

  renderSubConfig();
}

// renderSubConfig 渲染「订阅与证书」卡片（出于安全不回填私钥）。
function renderSubConfig() {
  const sc = state.subConfig;
  if (!sc) return;
  const ae = document.activeElement;
  if (ae !== $('#subPort')) $('#subPort').value = sc.port || 8788;
  if (ae !== $('#subSuffix')) $('#subSuffix').value = sc.suffix || '/sub';
  if (ae !== $('#subDomain')) $('#subDomain').value = sc.domain || '';
  if (ae !== $('#subEmail')) $('#subEmail').value = sc.email || '';

  $('#subPreview').innerHTML = `订阅地址示例：<b class="mono">${esc(sc.sampleURL || '')}</b> · 当前协议 <b>${sc.tls ? 'HTTPS' : 'HTTP'}</b>`;

  const c = sc.cert || {};
  if (c.mode === 'manual' || c.mode === 'acme') {
    $('#certStatus').innerHTML = `证书状态：<b>${c.mode === 'manual' ? '手动证书' : 'ACME 自动证书'}</b>`
      + (c.domain ? ` · 域名 <b class="mono">${esc(c.domain)}</b>` : '')
      + (c.notAfter ? ` · 到期 ${fmtTime(c.notAfter)}` : '')
      + (c.issuer ? ` · 颁发者 ${esc(c.issuer)}` : '');
  } else {
    $('#certStatus').innerHTML = `证书状态：<b>未配置</b>（当前 HTTP）`
      + (c.err ? ` · ${esc(c.err)}` : ' · 填写订阅域名与 ACME 邮箱后点击「申请/续期证书」');
  }

  $('#subTlsCert').placeholder = sc.hasManualCert ? '已配置（粘贴新 PEM 可更换）' : '-----BEGIN CERTIFICATE-----（留空则用 ACME 自动证书）';
  $('#subTlsKey').placeholder = sc.hasManualCert ? '已配置（粘贴新私钥可更换）' : '-----BEGIN PRIVATE KEY-----';
  $('#manualCertHint').textContent = sc.hasManualCert
    ? '已配置手动证书（优先于 ACME）。如需更换，粘贴新的证书与私钥后保存；留空保存不会改动现有证书。'
    : '留空则使用 ACME 自动证书；粘贴证书与私钥并保存后，手动证书将优先于 ACME。';
  $('#clearCertBtn').style.display = sc.hasManualCert ? '' : 'none';
}

$('#sysSaveBtn').onclick = async () => {
  try {
    const r = await api('api/system', {
      method: 'PUT',
      body: { listen: $('#sysListen').value.trim(), basePath: $('#sysBasePath').value.trim() },
    });
    toast('已保存，重启面板后生效');
    state.system = null;
    await loadSystem();
    if (r.needRestart && confirm('配置已保存。是否立即重启面板使其生效？')) {
      await api('api/system/restart', { method: 'POST' });
      waitReboot();
    }
  } catch (e) { toast(e.message, 'err'); }
};

$('#sysRestartBtn').onclick = async () => {
  if (!confirm('确定重启面板？当前页面会短暂断开。')) return;
  try {
    await api('api/system/restart', { method: 'POST' });
    waitReboot();
  } catch (e) { toast(e.message, 'err'); }
};

// 面板重启后轮询恢复
async function waitReboot() {
  toast('面板正在重启…');
  const next = location.pathname;
  for (let i = 0; i < 60; i++) {
    await new Promise(r => setTimeout(r, 1000));
    try {
      const res = await fetch(BASE + 'api/session', { credentials: 'same-origin', cache: 'no-store' });
      if (res.ok) { toast('面板已重启'); location.href = next; return; }
    } catch (e) { /* 等待中 */ }
  }
  toast('重启超时，请手动刷新页面', 'err');
}

$('#saveHostBtn').onclick = async () => {
  try {
    await api('api/settings', { method: 'PUT', body: { publicHost: $('#setHost').value.trim() } });
    toast('已保存');
    state.system = null;
    await refreshAll();
    await loadSystem();
  } catch (e) { toast(e.message, 'err'); }
};

$('#saveStatsBtn').onclick = async () => {
  try {
    const r = await api('api/settings', {
      method: 'PUT',
      body: { sampleSeconds: parseInt($('#setSample').value, 10), retentionDays: parseInt($('#setRetention').value, 10), gostLogLevel: $('#setLogLevel').value },
    });
    if (r.needGostRestart) {
      toast('已保存，正在重启 gost 生效…');
      try { await api('api/gost/restart', { method: 'POST' }); toast('gost 已重启'); }
      catch (e) { toast('设置已保存，但 gost 重启失败：' + e.message, 'err'); }
    } else {
      toast('已保存（采样间隔重启后生效）');
    }
    state.system = null;
    await loadSystem();
  } catch (e) { toast(e.message, 'err'); }
};

// 保存订阅与证书配置（热生效，无需重启面板）。
$('#saveSubBtn').onclick = async () => {
  const certPem = $('#subTlsCert').value.trim();
  const keyPem = $('#subTlsKey').value.trim();
  if ((certPem && !keyPem) || (!certPem && keyPem)) {
    toast('证书与私钥需同时填写', 'err');
    return;
  }
  const body = {
    port: parseInt($('#subPort').value, 10),
    suffix: $('#subSuffix').value.trim(),
    domain: $('#subDomain').value.trim(),
    email: $('#subEmail').value.trim(),
  };
  // 仅在粘贴了新证书时提交，避免普通保存清除已有手动证书。
  if (certPem && keyPem) { body.tlsCert = certPem; body.tlsKey = keyPem; }
  const btn = $('#saveSubBtn');
  btn.disabled = true;
  try {
    state.subConfig = await api('api/sub-config', { method: 'PUT', body });
    $('#subTlsCert').value = ''; $('#subTlsKey').value = '';
    toast('已保存，订阅监听器已热更新');
    renderSubConfig();
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; }
};

// 清除手动证书，切回 ACME。
$('#clearCertBtn').onclick = async () => {
  if (!confirm('清除手动证书并切回 ACME 自动证书？')) return;
  try {
    state.subConfig = await api('api/sub-config', { method: 'PUT', body: { tlsCert: '', tlsKey: '' } });
    $('#subTlsCert').value = ''; $('#subTlsKey').value = '';
    toast('已清除手动证书');
    renderSubConfig();
  } catch (e) { toast(e.message, 'err'); }
};

// 申请/续期证书：先保存最新域名/邮箱，再触发 ACME 签发。
$('#issueCertBtn').onclick = async () => {
  const domain = $('#subDomain').value.trim();
  const email = $('#subEmail').value.trim();
  if (!domain || !email) { toast('请先填写订阅域名与 ACME 邮箱', 'err'); return; }
  const btn = $('#issueCertBtn');
  const old = btn.textContent;
  btn.disabled = true; btn.textContent = '申请中…（可能需数十秒）';
  try {
    await api('api/sub-config', { method: 'PUT', body: {
      port: parseInt($('#subPort').value, 10),
      suffix: $('#subSuffix').value.trim(),
      domain, email,
    } });
    const r = await api('api/cert/issue', { method: 'POST' });
    if (r && r.config) state.subConfig = r.config;
    toast('证书已签发/续期');
    renderSubConfig();
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; btn.textContent = old; }
};

$('#savePwBtn').onclick = async () => {
  try {
    await api('api/password', { method: 'POST', body: { oldPassword: $('#pwOld').value, newPassword: $('#pwNew').value } });
    $('#pwOld').value = ''; $('#pwNew').value = '';
    toast('密码已修改');
  } catch (e) { toast(e.message, 'err'); }
};

$('#gostRestartBtn').onclick = async () => {
  if (!confirm('重启 gost 会短暂中断所有转发连接，继续？')) return;
  const btn = $('#gostRestartBtn');
  btn.disabled = true;
  try {
    await api('api/gost/restart', { method: 'POST' });
    toast('gost 已重启');
    state.system = null;
    await refreshAll();
    await loadSystem();
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; }
};

$('#backupBtn').onclick = async () => {
  const btn = $('#backupBtn');
  btn.disabled = true;
  try {
    const res = await fetch(BASE + 'api/backup', { credentials: 'same-origin' });
    if (!res.ok) throw new Error('备份失败：HTTP ' + res.status);
    const blob = await res.blob();
    const cd = res.headers.get('Content-Disposition') || '';
    const m = cd.match(/filename=([\w.\-]+)/);
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = m ? m[1] : 'panel-backup.db';
    a.click();
    URL.revokeObjectURL(a.href);
    toast('备份已下载');
  } catch (e) { toast(e.message, 'err'); }
  finally { btn.disabled = false; }
};

$('#gostLogsBtn').onclick = async () => {
  try {
    const r = await api('api/gost/logs?lines=300');
    openModal(`
      <button class="modal-close" onclick="closeModal()">×</button>
      <h2>gost 日志</h2>
      <pre class="logs">${esc((r.lines || []).join('\n')) || '（暂无日志）'}</pre>
      <div class="modal-actions"><button class="btn secondary" onclick="closeModal()">关闭</button></div>
    `, true);
  } catch (e) { toast(e.message, 'err'); }
};

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') { closeModal(); closeAllDropdowns(); }
  if (e.key === 'Tab' && !$('#modal').classList.contains('hidden')) {
    const items = $$('button, input, select, textarea, a[href], [tabindex="0"]', $('#modal'))
      .filter(el => !el.disabled && el.getClientRects().length);
    const first = items[0], last = items[items.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last?.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus(); }
  }
});
let chartResizeTimer;
window.addEventListener('resize', () => { clearTimeout(chartResizeTimer); chartResizeTimer = setTimeout(() => { if (state.view === 'overview' && state.overview) renderOverview(); }, 120); });

/* ---------------- 启动 ---------------- */
init();
