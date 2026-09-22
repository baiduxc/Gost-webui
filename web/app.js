/* GOST 面板 · 前端逻辑 · 幽谷灵境 */
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
  selectedNodeId: null,
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

/* ---------------- 主题 ---------------- */
function applyTheme(t) {
  document.documentElement.dataset.theme = t;
  localStorage.setItem('gp_theme', t);
  $('#themeBtn').innerHTML = icon(t === 'dark' ? 'sun' : 'moon');
  $('#themeBtn').setAttribute('aria-label', t === 'dark' ? '切换浅色主题' : '切换深色主题');
  if (state.overview && state.view === 'overview') renderOverview();
}
function initTheme() {
  applyTheme(localStorage.getItem('gp_theme') || 'dark');
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
  if (!state.timer) state.timer = setInterval(refreshAll, 20000);
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
$('#themeBtn').onclick = () => applyTheme(document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark');

/* ---------------- 视图 ---------------- */
$('#nav').addEventListener('click', e => {
  const a = e.target.closest('a[data-view]');
  if (a) { e.preventDefault(); setView(a.dataset.view); }
});

const VIEW_TITLE = { overview: '网络概览', nodes: '节点管理', notify: '通知提醒', system: '系统设置' };
const VIEW_SUBTITLE = { overview: '观流量起落，守每一程连接。', nodes: '连接有序，流转自如。', notify: '重要的消息，自会如期而至。', system: '静心调校，让连接安稳如常。' };
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
  $('#hostChip').textContent = state.publicHost || '未检测到地址';
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
  drawChart($('#chart'), series);

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
          <td>${statusTag(x.enabled, x.blocked)}</td>
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
  if (!enabled) return '<span class="status-inline"><span class="status-dot"></span>已停用</span>';
  if (blocked) return '<span class="status-inline"><span class="status-dot bad"></span>超限暂停</span>';
  return '<span class="status-inline"><span class="status-dot ok"></span>运行中</span>';
}

/* 流量图表：翡翠上行、淡金下行 */
function drawChart(canvas, points) {
  if (!canvas) return;
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

  const padL = 62, padR = 26, padT = 12, padB = 28;
  const cw = w - padL - padR, ch = h - padT - padB;

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
    ctx.beginPath();
    ctx.moveTo(padL, y + 0.5);
    ctx.lineTo(w - padR, y + 0.5);
    ctx.stroke();
    ctx.fillStyle = cTertiary;
    ctx.fillText(fmtBytes(niceMax * (4 - i) / 4), padL - 10, y + 4);
  }

  const stepX = points.length > 1 ? cw / (points.length - 1) : 0;
  const px = i => padL + stepX * i;
  const py = v => padT + ch * (1 - (v || 0) / niceMax);

  const series = (key, color) => {
    ctx.beginPath();
    points.forEach((p, i) => { i ? ctx.lineTo(px(i), py(p[key])) : ctx.moveTo(px(i), py(p[key])); });
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.stroke();
    // 实心浅色填充（无渐变）
    ctx.lineTo(px(points.length - 1), padT + ch);
    ctx.lineTo(padL, padT + ch);
    ctx.closePath();
    ctx.globalAlpha = 0.08;
    ctx.fillStyle = color;
    ctx.fill();
    ctx.globalAlpha = 1;
  };
  series('in', cAccent);
  series('out', cGold);

  ctx.textAlign = 'center';
  ctx.fillStyle = cTertiary;
  const ticks = Math.min(6, points.length);
  for (let i = 0; i < ticks; i++) {
    const idx = Math.round(i * (points.length - 1) / Math.max(1, ticks - 1));
    ctx.fillText(fmtDay(points[idx].ts), px(idx), h - 8);
  }
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
  const wrap = $('#nodeTable');
  const nodes = state.nodes;
  const active = nodes.filter(n => n.enabled && !(n.live && n.live.quotaBlocked)).length;
  $('#nodesSummary').textContent = `${nodes.length} 个节点 · ${active} 个已启用 · 本月 ${fmtBytes(nodes.reduce((sum, n) => sum + (n.monthIn || 0) + (n.monthOut || 0), 0))}`;
  if (!nodes.some(n => n.id === state.selectedNodeId)) state.selectedNodeId = nodes[0]?.id || null;
  if (!nodes.length) {
    wrap.innerHTML = '<div class="empty"><span class="empty-title">此间尚无连接</span><p>添加第一个节点，开启你的中转之旅。</p><button class="btn secondary" data-act="add">添加节点</button></div>';
    renderNodeInspector();
    return;
  }
  wrap.innerHTML = `<table>
    <thead><tr><th>节点名称 / 落地机</th><th>监听端口</th><th>本月用量 / 配额</th><th>状态</th><th class="right">操作</th></tr></thead>
    <tbody>${nodes.map(n => `<tr class="${n.id === state.selectedNodeId ? 'selected' : ''}">
      <td><button class="node-select" data-act="select" data-id="${esc(n.id)}" aria-pressed="${n.id === state.selectedNodeId}">${icon('server')}<span><b>${esc(n.name)}</b><small>${esc(n.protocol || 'TCP')}${n.mode === 'gost' ? ' · GOST' : ''}${n.udp ? ' · TCP+UDP' : ''} / ${esc(n.targetHost)}:${n.targetPort}</small></span></button></td>
      <td class="mono">${n.listenPort}</td>
      <td class="node-quota"><span class="mono">${fmtBytes((n.monthIn || 0) + (n.monthOut || 0))}</span>${quotaCell(n)}</td>
      <td><button class="node-toggle" data-act="toggle" data-id="${esc(n.id)}" role="switch" aria-checked="${!!n.enabled}" aria-label="${n.enabled ? '停用' : '启用'}${esc(n.name)}"><span class="toggle-track"></span></button>${statusTag(n.enabled, n.live && n.live.quotaBlocked)}</td>
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
  renderNodeInspector();
}

function renderNodeInspector() {
  const n = state.nodes.find(n => n.id === state.selectedNodeId);
  const panel = $('#nodeInspector');
  if (!n) { panel.innerHTML = '<div class="empty"><span class="empty-title">静候连接</span><p>节点的状态与用量<br>将在这里一目了然。</p></div>'; return; }
  panel.innerHTML = `<div class="inspector-heading"><span class="eyebrow">NODE / 连接详情</span><h2>${esc(n.name)}</h2>${statusTag(n.enabled, n.live && n.live.quotaBlocked)}</div>
    <dl class="node-facts"><div><dt>监听端口</dt><dd>${n.listenPort}</dd></div><div><dt>当前连接</dt><dd>${n.live?.currentConns || 0}</dd></div><div><dt>传输协议</dt><dd>${esc(n.protocol || 'TCP')}${n.udp ? ' / UDP' : ''}</dd></div><div><dt>今日上行</dt><dd>${fmtBytes(n.todayIn)}</dd></div><div><dt>今日下行</dt><dd>${fmtBytes(n.todayOut)}</dd></div></dl>
    <div class="inspector-quota"><div class="desc">流量配额</div>${quotaCell(n)}</div>
    <dl class="node-facts"><div><dt>本月用量</dt><dd>${fmtBytes((n.monthIn || 0) + (n.monthOut || 0))}</dd></div><div><dt>创建时间</dt><dd>${n.createdAt ? fmtTime(n.createdAt) : '—'}</dd></div></dl>
    <div class="inspector-actions"><button class="btn secondary" data-inspect="copy">${icon('copy')}复制链接</button><button class="btn secondary" data-inspect="edit">${icon('edit')}编辑节点</button></div>`;
}

$('#nodeInspector').addEventListener('click', async e => {
  const btn = e.target.closest('button[data-inspect]');
  const node = state.nodes.find(n => n.id === state.selectedNodeId);
  if (!btn || !node) return;
  if (btn.dataset.inspect === 'edit') { openNodeForm(node); return; }
  try { const r = await api(`api/nodes/${node.id}/link`); if (r?.url) await copyText(r.url); }
  catch (err) { toast(err.message, 'err'); }
});

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

$('#nodeTable').addEventListener('click', async e => {
  const btn = e.target.closest('button[data-act]');
  if (!btn) return;
  const id = btn.dataset.id;
  if (btn.dataset.act === 'add') { openNodeForm(null); return; }
  if (btn.dataset.act === 'select') { state.selectedNodeId = id; renderNodes(); return; }

  // 下拉菜单开合
  if (btn.dataset.act === 'menu') {
    e.stopPropagation();
    const dd = btn.closest('.dropdown');
    const wasOpen = dd.classList.contains('open');
    closeAllDropdowns();
    if (!wasOpen) {
      dd.classList.add('open');
      btn.setAttribute('aria-expanded', 'true');
      const menu = dd.querySelector('.dropdown-menu');
      const rect = btn.getBoundingClientRect();
      const mw = menu.offsetWidth || 132;
      const mh = menu.offsetHeight || 0;
      let left = rect.right - mw;
      if (left < 8) left = 8;
      if (left + mw > window.innerWidth - 8) left = window.innerWidth - mw - 8;
      let top = rect.bottom + 4;
      if (top + mh > window.innerHeight - 8) top = Math.max(8, rect.top - mh - 4);
      menu.style.left = left + 'px';
      menu.style.top = top + 'px';
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
});
$('#addNodeBtn').onclick = () => openNodeForm(null);

// 点击表格以外区域时关闭所有下拉菜单
function closeAllDropdowns() {
  $$('.dropdown.open').forEach(d => { d.classList.remove('open'); d.querySelector('.dropdown-toggle')?.setAttribute('aria-expanded', 'false'); });
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
function openNodeForm(node) {
  const isEdit = !!node;
  const q = (node && node.quota) || { enabled: false, period: 'monthly', bytes: 0, direction: 'total' };
  const rate = (node && node.rate) || { enabled: false, inBps: 0, outBps: 0 };
  const isGost = !!(node && node.mode === 'gost');
  const initMode = isGost ? 'gost' : 'link';
  const gc = (isGost && node.gostCipher) || 'aes-256-gcm';
  const cipherOpt = (v) => `<option value="${v}"${gc === v ? ' selected' : ''}>${v}</option>`;

  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>${isEdit ? '编辑节点' : '添加节点'}</h2>

    <div class="tabs" id="nodeModeTabs" style="margin-bottom:16px">
      <button type="button" data-mode="link" class="${initMode === 'link' ? 'active' : ''}">粘贴链接</button>
      <button type="button" data-mode="gost" class="${initMode === 'gost' ? 'active' : ''}">GOST 体系</button>
    </div>

    <div id="modeLink" class="${initMode === 'link' ? '' : 'hidden'}">
    <div class="field">
      <label>落地机 v2rayN 链接</label>
      <textarea id="f-link" placeholder="粘贴 vmess:// / vless:// / trojan:// / ss:// / hysteria2:// / tuic:// 链接">${esc(!isGost && node ? node.landingLink : '')}</textarea>
      <div class="hint">粘贴落地机节点的分享链接，面板自动解析协议、地址与鉴权参数</div>
      <div class="hint">纯透传模式：面板只把落地机链接的地址端口换成中转机，其余参数（UUID/SNI/Reality 公钥/flow 等）原样保留，加密握手端到端直达落地机。</div>
      <div class="btn-group" style="margin-top:12px">
        <button class="btn secondary sm" id="testBtn">测试落地机连通性</button>
        <span class="hint" id="testResult" style="margin:0"></span>
      </div>
    </div>
    <div id="parseBox"></div>
    </div>

    <div id="modeGost" class="${initMode === 'gost' ? '' : 'hidden'}">
      <div class="notice">落地机只需安装 GOST 并运行下方生成的 Shadowsocks 服务，无需 Xray/sing-box；中转机对其做纯 TCP 透传，客户端 ss 握手端到端直达落地机。保存后会给出落地机一键部署命令。</div>
      <div class="row">
        <div class="field">
          <label>落地机公网 IP / 域名</label>
          <input id="f-ghost-host" type="text" value="${esc(isGost ? node.targetHost : '')}" placeholder="如 1.2.3.4 或 land.example.com">
        </div>
        <div class="field">
          <label>落地机端口</label>
          <input id="f-ghost-port" type="number" min="1" max="65535" value="${isGost ? node.targetPort : ''}" placeholder="如 8388">
        </div>
      </div>
      <div class="row">
        <div class="field">
          <label>加密方式</label>
          <select id="f-ghost-cipher">
            ${cipherOpt('aes-256-gcm')}
            ${cipherOpt('aes-128-gcm')}
            ${cipherOpt('chacha20-ietf-poly1305')}
          </select>
        </div>
        <div class="field">
          <label>密码</label>
          <div class="row tight">
            <input id="f-ghost-pass" type="text" value="${esc(isGost ? node.gostPassword : '')}" placeholder="留空自动生成">
            <button class="btn secondary sm" id="ghostRandBtn" style="flex:0 0 auto">随机</button>
          </div>
        </div>
      </div>
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

    <div class="field">
      <label class="switch"><input type="checkbox" id="f-udp" ${node && node.udp ? 'checked' : ''}><span class="track"></span>
        <span><span class="txt">同时转发 UDP</span><span class="desc">hysteria2 / tuic / KCP / QUIC 等协议必须开启</span></span></label>
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
          <div class="row tight">
            <input id="f-quotaSize" type="number" min="0" step="0.1" value="${q.bytes ? (q.bytes / Math.pow(1024, 3)).toFixed(2) : ''}" placeholder="如 100">
            <select id="f-quotaUnit" style="flex:0 0 92px">
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

    <div class="modal-actions">
      <button class="btn secondary" onclick="closeModal()">取消</button>
      <button class="btn" id="saveNodeBtn">${isEdit ? '保存修改' : '创建节点'}</button>
    </div>
  `, true);

  $('#f-link').addEventListener('input', debounce(() => parseLink(true), 500));
  $$('#nodeModeTabs button').forEach(b => {
    b.onclick = () => {
      const mode = b.dataset.mode;
      $$('#nodeModeTabs button').forEach(x => x.classList.toggle('active', x === b));
      const isLink = mode === 'link';
      $('#modeLink').classList.toggle('hidden', !isLink);
      $('#advOverride').classList.toggle('hidden', !isLink);
      $('#modeGost').classList.toggle('hidden', isLink);
    };
  });
  $('#ghostRandBtn').onclick = () => { $('#f-ghost-pass').value = randPass(20); };
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
    const th = $('#f-ghost-host').value.trim();
    const tp = parseInt($('#f-ghost-port').value || '0', 10) || 0;
    if (!th) { toast('请填写落地机公网 IP / 域名', 'err'); return; }
    if (!(tp > 0 && tp <= 65535)) { toast('请填写有效的落地机端口', 'err'); return; }
    body = Object.assign({
      mode: 'gost',
      targetHost: th,
      targetPort: tp,
      gostCipher: $('#f-ghost-cipher').value,
      gostPassword: $('#f-ghost-pass').value.trim(),
    }, common);
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
  openModal(`
    <button class="modal-close" onclick="closeModal()">×</button>
    <h2>客户端链接 · ${esc(node.name)}</h2>
    ${state.publicHostPrivate ? `<div class="notice warn">当前服务器地址 ${esc(r.host)} 是内网地址，客户端无法连接，请到「系统设置」填写公网 IP 或域名。</div>` : ''}
    <div class="notice">把下面链接导入 v2rayN 等客户端即可，地址已指向本服务器 <b>${esc(r.host)}:${r.port}</b>。</div>
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
  box.innerHTML = `
    <div class="section-label">落地机部署（GOST 体系）</div>
    <div class="notice">在落地机安装并运行 GOST 的 Shadowsocks 服务即可，无需 Xray。加密 <b>${esc(d.cipher)}</b> · 端口 <b>${d.port}</b>${d.udp ? ' · TCP+UDP' : ' · TCP'}。</div>
    <div class="hint" style="margin-top:12px">操作步骤</div>
    <div class="notice">${(d.steps || []).map(s => esc(s)).join('<br>')}</div>

    <div class="hint" style="margin-top:16px">① 安装 GOST · 方式一：下载预编译二进制（自动识别架构）</div>
    <div class="code-box">${esc(d.installBinaryCmd)}</div>
    <button class="btn secondary sm" id="copyInstallBinBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">① 安装 GOST · 方式二：官方安装脚本</div>
    <div class="code-box">${esc(d.installScriptCmd)}</div>
    <button class="btn secondary sm" id="copyInstallScriptBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">② 保存 ss 服务配置到 ${esc(d.confPath)}</div>
    <div class="code-box">${esc(d.saveCommand)}</div>
    <button class="btn secondary sm" id="copySaveBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">③ 注册为 systemd 后台服务（自动后台运行 + 开机自启）</div>
    <div class="code-box">${esc(d.serviceCommand)}</div>
    <button class="btn secondary sm" id="copyServiceBtn" style="margin-top:8px">${icon('copy')} 复制</button>

    <div class="hint" style="margin-top:16px">（可选）前台临时运行，用于快速测试</div>
    <div class="code-box">${esc(d.runCommand)}</div>
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
        <div class="k">落地机</div><div class="v mono">${esc(node.targetHost)}:${node.targetPort} · ${esc(node.protocol)}</div>
        <div class="k">监听端口</div><div class="v mono">${node.listenPort} ${node.udp ? '(TCP+UDP)' : '(TCP)'}</div>
        <div class="k">连接数</div><div class="v">当前 ${live.currentConns || 0} · 累计 ${live.totalConns || 0}</div>
        <div class="k">今日流量</div><div class="v mono">↑ ${fmtBytes(node.todayIn)} / ↓ ${fmtBytes(node.todayOut)}</div>
        <div class="k">本月流量</div><div class="v mono">↑ ${fmtBytes(node.monthIn)} / ↓ ${fmtBytes(node.monthOut)}</div>
        <div class="k">累计流量</div><div class="v mono">↑ ${fmtBytes(node.totalIn)} / ↓ ${fmtBytes(node.totalOut)}</div>
        <div class="k">流量配额</div><div class="v">${quotaHtml}</div>
      </div>
      <div class="card-head">
        <div><h2 style="font-size:15px">流量曲线</h2></div>
        <div class="tabs">
          <button data-range="24h">24 小时</button>
          <button data-range="7d" class="active">7 天</button>
          <button data-range="30d">30 天</button>
        </div>
      </div>
      <canvas id="detailChart" height="130" style="width:100%"></canvas>
      <div class="section-label">客户端链接</div>
      <div class="code-box" id="detailLink">加载中…</div>
      <div style="display:flex;gap:24px;margin-top:16px;align-items:flex-start;flex-wrap:wrap">
        <div class="qr"><img id="detailQR" src="${BASE}api/nodes/${node.id}/qrcode?t=${Date.now()}" alt="二维码"></div>
        <div class="hint">扫码或在客户端粘贴链接导入<br>连接地址 <span class="mono">${esc(state.publicHost)}:${node.listenPort}</span></div>
      </div>
      ${node.mode === 'gost' ? '<div id="detailDeploy"></div>' : ''}`;
    drawChart($('#detailChart'), r.points || []);
    $$('#detailBody .tabs button').forEach(b => {
      b.onclick = () => {
        $$('#detailBody .tabs button').forEach(x => x.classList.toggle('active', x === b));
        loadStats(b.dataset.range);
      };
    });
    const lr = await api(`api/nodes/${node.id}/link`).catch(() => null);
    const el = $('#detailLink');
    if (el) el.textContent = lr && lr.url ? lr.url : '（无法生成：请在系统设置中填写服务器地址）';
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
  body.innerHTML = `
    <div class="notice">${r.enabled ? '' : '<b>该节点已停用，订阅内容为空。</b> '}${schemeNote}</div>
    <div class="sub-grid">
      <div class="sub-item">
        <div class="hint">通用订阅（自动识别 Clash / v2rayN）</div>
        <div class="code-box">${esc(r.url)}</div>
        <div class="sub-qr">
          <div class="qr"><img src="${qr(r.url)}" alt="通用订阅二维码"></div>
          <button class="btn secondary sm" data-copy="${esc(r.url)}">复制链接</button>
        </div>
      </div>
      <div class="sub-item">
        <div class="hint">Clash Meta / mihomo（强制 YAML）</div>
        <div class="code-box">${esc(r.clashURL)}</div>
        <div class="sub-qr">
          <div class="qr"><img src="${qr(r.clashURL)}" alt="Clash 订阅二维码"></div>
          <button class="btn secondary sm" data-copy="${esc(r.clashURL)}">复制链接</button>
        </div>
      </div>
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

  if (document.activeElement !== $('#sysListen')) $('#sysListen').value = p.listen || '';
  if (document.activeElement !== $('#sysBasePath')) $('#sysBasePath').value = p.basePath || '';

  const host = location.host;
  const preview = `${p.basePath || ''}/`;
  $('#sysAccessInfo').innerHTML = `当前访问地址：<b>${esc(host + preview)}</b> · 进程启动于 ${fmtTime(p.startedAt)}（已运行 ${fmtDuration(p.uptime)}）`;

  if (document.activeElement !== $('#setHost')) $('#setHost').value = st.configured || '';
  $('#setHost').placeholder = '自动探测（当前 ' + (sys.publicHost || '未知') + '）';
  if (document.activeElement !== $('#setSample')) $('#setSample').value = st.sampleSeconds;
  if (document.activeElement !== $('#setRetention')) $('#setRetention').value = st.retentionDays;

  const g = sys.gost || {};
  const proc = g.process || {};
  $('#gostInfo').innerHTML = `
    <div class="k">状态</div><div class="v">${g.reachable === false ? '<span class="status-inline"><span class="status-dot bad"></span>API 不可达</span>' : (proc.running ? '<span class="status-inline"><span class="status-dot ok"></span>运行中</span>' : '<span class="status-inline"><span class="status-dot bad"></span>未运行</span>')}</div>
    <div class="k">PID</div><div class="v mono">${proc.pid || '—'}</div>
    <div class="k">重启次数</div><div class="v">${proc.restarts || 0}</div>
    <div class="k">最近退出</div><div class="v mono">${esc(proc.lastExit || '—')}</div>`;
  $('#gostPaths').textContent = `${g.bin || ''} · ${g.configFile || ''} · ${g.logFile || ''}`;

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
    await api('api/settings', {
      method: 'PUT',
      body: { sampleSeconds: parseInt($('#setSample').value, 10), retentionDays: parseInt($('#setRetention').value, 10) },
    });
    toast('已保存（采样间隔重启后生效）');
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
