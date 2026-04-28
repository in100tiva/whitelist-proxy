// =================================================================
// Whitelist Proxy — front-end vanilla.
// Sem framework, sem build. Mantenha simples e legível.
// =================================================================

const TOKEN_KEY = "wlp_token";
const THEME_KEY = "wlp_theme";

const state = {
  rules: [],          // [{pattern, type, action, note, _id?}]
  mode: "blacklist",  // "blacklist" | "whitelist"
  dirty: false,
  search: "",
  logsTimer: null,
  statusTimer: null,
  dashboardTimer: null,
  recentDecisions: [],
  filterAction: "",
  filterHost: "",
  currentTab: "dashboard",
};

let _idSeq = 1;
const newId = () => "r" + (_idSeq++);

// --------------- bootstrap ---------------

document.addEventListener("DOMContentLoaded", () => {
  applyTheme(localStorage.getItem(THEME_KEY) || "dark");

  // Token via ?t= no link.
  const url = new URL(location.href);
  const fromQuery = url.searchParams.get("t");
  if (fromQuery) {
    localStorage.setItem(TOKEN_KEY, fromQuery);
    url.searchParams.delete("t");
    history.replaceState({}, "", url.toString());
  }

  bindGlobal();
  bindLogin();
  bindDashboard();
  bindWhitelist();
  bindLogs();
  bindSettings();
  bindModalGlobal();

  if (localStorage.getItem(TOKEN_KEY)) {
    enterApp();
  } else {
    showLogin();
  }
});

// --------------- HTTP ---------------

async function api(path, opts = {}) {
  const token = localStorage.getItem(TOKEN_KEY);
  const headers = Object.assign(
    {},
    opts.body ? { "Content-Type": "application/json" } : {},
    opts.headers || {},
    token ? { "Authorization": "Bearer " + token } : {}
  );
  const resp = await fetch(path, Object.assign({}, opts, { headers }));
  if (resp.status === 401) {
    localStorage.removeItem(TOKEN_KEY);
    showLogin();
    throw new Error("unauthorized");
  }
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error((text || resp.statusText).trim());
  }
  const ct = resp.headers.get("Content-Type") || "";
  return ct.includes("application/json") ? resp.json() : resp.text();
}

// --------------- THEME ---------------

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem(THEME_KEY, theme);
  document.querySelectorAll("[data-theme-set]").forEach(el => {
    el.classList.toggle("active", el.dataset.themeSet === theme);
  });
}

// Troca de tema com animação circular a partir do ponto de clique.
// Claro → escuro: círculo fecha (colapsa) no ponto clicado.
// Escuro → claro: círculo abre (expande) a partir do ponto clicado.
// Animação circular via Web Animations API (WAAPI).
// Evita problemas de batching de estilos que afetam CSS transitions.
// Funciona em Chrome, Firefox e Edge modernos.
function applyThemeWithAnimation(theme, event) {
  if (!event || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    applyTheme(theme);
    return;
  }

  const x  = event.clientX;
  const y  = event.clientY;
  const r  = Math.hypot(
    Math.max(x, window.innerWidth  - x),
    Math.max(y, window.innerHeight - y)
  );
  const bg = theme === 'dark' ? '#0a0b0d' : '#f7f8fa';

  const el = document.createElement('div');
  el.style.cssText = `position:fixed;inset:0;z-index:99999;pointer-events:none;background:${bg}`;
  document.body.appendChild(el);

  const anim = el.animate(
    [
      { clipPath: `circle(0px at ${x}px ${y}px)` },
      { clipPath: `circle(${r}px at ${x}px ${y}px)` },
    ],
    { duration: 500, easing: 'ease-in-out', fill: 'forwards' }
  );

  anim.onfinish = () => {
    applyTheme(theme);
    el.remove();
  };
}

// --------------- TOKEN BASEADO NO HORÁRIO ---------------

function getCurrentTimeToken() {
  const now = new Date();
  return String(now.getHours()).padStart(2, '0') + String(now.getMinutes()).padStart(2, '0');
}

function bindGlobal() {
  document.getElementById("theme-toggle").addEventListener("click", (e) => {
    const cur = document.documentElement.dataset.theme;
    applyThemeWithAnimation(cur === "dark" ? "light" : "dark", e);
  });
  document.getElementById("logout").addEventListener("click", () => {
    localStorage.removeItem(TOKEN_KEY);
    location.reload();
  });
  document.querySelectorAll(".nav-item").forEach(b => {
    b.addEventListener("click", () => goTo(b.dataset.tab));
  });
  document.querySelectorAll("[data-goto]").forEach(b => {
    b.addEventListener("click", () => goTo(b.dataset.goto));
  });
  // Atalhos globais.
  document.addEventListener("keydown", e => {
    if ((e.ctrlKey || e.metaKey) && e.key === "s" && state.currentTab === "whitelist") {
      e.preventDefault();
      if (state.dirty) saveRules();
    }
    if (e.key === "Escape" && !document.getElementById("modal-root").hidden) {
      closeModal();
    }
  });
}

// --------------- LOGIN ---------------

function bindLogin() {
  const input    = document.getElementById("token-input");
  const nowEl    = document.getElementById("login-token-now");
  const secsEl   = document.getElementById("login-countdown");
  let lastToken  = getCurrentTimeToken();

  function tick() {
    const tok  = getCurrentTimeToken();
    const secs = 60 - new Date().getSeconds();
    if (nowEl)  nowEl.textContent  = tok;
    if (secsEl) secsEl.textContent = secs;
    if (tok !== lastToken) {
      lastToken = tok;
      if (!input.dataset.userTyped) input.value = tok;
    }
  }

  input.value = lastToken;
  tick();
  setInterval(tick, 1000);
  input.addEventListener("input", () => { input.dataset.userTyped = "1"; });

  const submit = () => {
    const tok = input.value.trim();
    if (!tok) return;
    localStorage.setItem(TOKEN_KEY, tok);
    enterApp();
  };
  document.getElementById("token-submit").addEventListener("click", submit);
  input.addEventListener("keydown", e => { if (e.key === "Enter") submit(); });
}

function showLogin() {
  document.getElementById("view-login").hidden = false;
  document.getElementById("view-app").hidden = true;
  stopAllTimers();
  const input = document.getElementById("token-input");
  if (input) {
    input.value = getCurrentTimeToken();
    delete input.dataset.userTyped;
    input.focus();
  }
  document.getElementById("login-error").hidden = true;
}

async function enterApp() {
  document.getElementById("view-login").hidden = true;
  document.getElementById("view-app").hidden = false;
  try {
    await refreshStatus();   // valida token
  } catch (err) {
    showLogin();
    document.getElementById("login-error").textContent = "Token inválido ou serviço offline.";
    document.getElementById("login-error").hidden = false;
    return;
  }
  // Carrega regras uma vez para popular o badge da nav.
  refreshRules();
  goTo("dashboard");
}

// --------------- NAV ---------------

function goTo(tab) {
  state.currentTab = tab;
  document.querySelectorAll(".page").forEach(p => p.hidden = true);
  document.getElementById("page-" + tab).hidden = false;
  document.querySelectorAll(".nav-item").forEach(n => {
    n.classList.toggle("active", n.dataset.tab === tab);
  });

  stopAllTimers();
  switch (tab) {
    case "dashboard":
      refreshDashboard();
      state.dashboardTimer = setInterval(refreshDashboard, 5000);
      break;
    case "whitelist":
      refreshRules();
      break;
    case "logs":
      refreshLogs();
      if (document.getElementById("auto-refresh").checked) {
        state.logsTimer = setInterval(refreshLogs, 3000);
      }
      break;
    case "settings":
      refreshSettings();
      loadBrowsers();
      state.statusTimer = setInterval(refreshSettings, 5000);
      break;
  }
}

function stopAllTimers() {
  ["logsTimer", "statusTimer", "dashboardTimer"].forEach(k => {
    if (state[k]) { clearInterval(state[k]); state[k] = null; }
  });
}

// --------------- STATUS / CONNECTION INDICATOR ---------------

async function refreshStatus() {
  const s = await api("/api/status");
  setConn(true);
  return s;
}

function setConn(online) {
  const dot = document.getElementById("conn-dot");
  const txt = document.getElementById("conn-text");
  dot.className = "conn-dot " + (online ? "online" : "offline");
  txt.textContent = online ? "online" : "offline";
}

// --------------- DASHBOARD ---------------

function bindDashboard() {
  document.getElementById("dash-test-submit").addEventListener("click", () => runDashTest());
  document.getElementById("dash-test-host").addEventListener("keydown", e => {
    if (e.key === "Enter") runDashTest();
  });
}

async function refreshDashboard() {
  try {
    const [s, logs] = await Promise.all([
      api("/api/status"),
      api("/api/logs/recent?n=200"),
    ]);
    setConn(true);
    document.getElementById("dash-allow").textContent  = formatNum(s.allow_count);
    document.getElementById("dash-block").textContent  = formatNum(s.block_count);
    document.getElementById("dash-rules").textContent  = formatNum(s.rule_count);
    document.getElementById("dash-uptime").textContent = formatUptime(s.uptime_seconds);
    document.getElementById("nav-rule-count").textContent = s.rule_count;

    // Bloqueios recentes (últimos 8).
    const decisions = (logs.decisions || []).filter(d => d.action === "block").slice().reverse().slice(0, 8);
    state.recentDecisions = decisions;
    renderRecentBlocks(decisions);
  } catch (err) {
    setConn(false);
  }
}

function renderRecentBlocks(decisions) {
  const root = document.getElementById("dash-recent-blocks");
  if (decisions.length === 0) {
    root.innerHTML = `<div class="empty-mini">Sem bloqueios recentes — tudo dentro da whitelist.</div>`;
    return;
  }
  root.innerHTML = decisions.map(d => `
    <div class="log-row">
      <span class="log-time">${formatTimeShort(d.time)}</span>
      <span class="log-action log-action-block">block</span>
      <span class="log-host">
        ${escapeHtml(d.host || "(sem host)")}
        <span class="log-meta">${escapeHtml(d.proto || "")} · ${escapeHtml(d.client || "")}</span>
      </span>
      <button class="btn btn-ghost btn-sm log-allow-btn" data-allow-host="${escapeAttr(d.host || "")}">
        + Permitir
      </button>
    </div>
  `).join("");
  root.querySelectorAll("[data-allow-host]").forEach(btn => {
    btn.addEventListener("click", () => openAddModal({ initial: btn.dataset.allowHost }));
    btn.style.opacity = 1; // sempre visível na lista compacta do dashboard
  });
}

async function runDashTest() {
  const host = document.getElementById("dash-test-host").value.trim();
  if (!host) return;
  const result = document.getElementById("dash-test-result");
  try {
    const data = await api("/api/test?host=" + encodeURIComponent(host), { method: "POST" });
    result.hidden = false;
    result.className = "test-result-card " + (data.allowed ? "allow" : "block");
    result.innerHTML = data.allowed
      ? `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
         <span><strong class="mono">${escapeHtml(data.host)}</strong> seria <strong>permitido</strong>.</span>`
      : `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>
         <span><strong class="mono">${escapeHtml(data.host)}</strong> seria <strong>bloqueado</strong>.</span>`;
  } catch (err) {
    toast({ type: "error", title: "Erro ao testar", message: err.message });
  }
}

// --------------- WHITELIST ---------------

function bindWhitelist() {
  document.getElementById("open-add-rule").addEventListener("click", () => openAddModal());
  document.getElementById("save-rules").addEventListener("click", saveRules);
  document.getElementById("reload-disk").addEventListener("click", reloadFromDisk);
  document.getElementById("rule-search").addEventListener("input", e => {
    state.search = e.target.value.toLowerCase();
    renderRules();
  });
  document.querySelectorAll('[data-action="add-first"]').forEach(b => {
    b.addEventListener("click", () => openAddModal());
  });
  document.querySelectorAll(".mode-btn").forEach(btn => {
    btn.addEventListener("click", async () => {
      if (btn.dataset.mode === state.mode) return;
      state.mode = btn.dataset.mode;
      applyModeUI();
      markDirty();
    });
  });
}

function applyModeUI() {
  const mode = state.mode;
  document.querySelectorAll(".mode-btn").forEach(btn => {
    btn.classList.toggle("active", btn.dataset.mode === mode);
  });
  const subtitle = document.getElementById("mode-subtitle");
  if (subtitle) {
    subtitle.textContent = mode === "blacklist"
      ? "Blacklist — tudo permitido por padrão. Apenas domínios bloqueados são barrados."
      : "Whitelist — tudo bloqueado por padrão. Apenas domínios permitidos são liberados.";
  }
}

async function refreshRules() {
  if (state.dirty) return; // não derruba edições
  try {
    const data = await api("/api/whitelist");
    state.mode = data.mode || "blacklist";
    state.rules = (data.rules || []).map(r => ({
      _id: newId(),
      pattern: r.pattern || "",
      type: r.type || "exact",
      action: r.action || "allow",
      note: r.note || "",
    }));
    state.dirty = false;
    updateDirtyUI();
    applyModeUI();
    renderRules();
    document.getElementById("nav-rule-count").textContent = state.rules.length;
  } catch (err) {
    toast({ type: "error", title: "Erro carregando whitelist", message: err.message });
  }
}

function renderRules() {
  const tbody = document.getElementById("rules-body");
  const empty = document.getElementById("rules-empty");
  const table = tbody.closest("table");

  let visible = state.rules;
  if (state.search) {
    visible = state.rules.filter(r =>
      r.pattern.toLowerCase().includes(state.search) ||
      (r.note || "").toLowerCase().includes(state.search)
    );
  }

  // Empty state (lista vazia, sem busca)
  if (state.rules.length === 0) {
    table.style.display = "none";
    empty.hidden = false;
    return;
  }
  table.style.display = "";
  empty.hidden = true;

  if (visible.length === 0) {
    tbody.innerHTML = `<tr><td colspan="4" style="text-align:center;padding:32px;color:var(--fg-tertiary)">
      Nenhuma regra bate com "${escapeHtml(state.search)}".
    </td></tr>`;
    return;
  }

  tbody.innerHTML = visible.map(rule => `
    <tr data-id="${rule._id}" class="${rule.action === 'block' ? 'rule-block' : ''}">
      <td>
        <input type="text" class="editable-input mono" data-field="pattern"
               value="${escapeAttr(rule.pattern)}" placeholder="ex.: *.exemplo.com">
      </td>
      <td>
        <select class="editable-select" data-field="type">
          <option value="exact"    ${rule.type === "exact"    ? "selected" : ""}>exact</option>
          <option value="wildcard" ${rule.type === "wildcard" ? "selected" : ""}>wildcard</option>
          <option value="regex"    ${rule.type === "regex"    ? "selected" : ""}>regex</option>
        </select>
      </td>
      <td>
        <select class="editable-select action-select" data-field="action">
          <option value="allow" ${(rule.action || "allow") === "allow" ? "selected" : ""}>allow</option>
          <option value="block" ${rule.action === "block" ? "selected" : ""}>block</option>
        </select>
      </td>
      <td>
        <input type="text" class="editable-input" data-field="note"
               value="${escapeAttr(rule.note)}" placeholder="(opcional)">
      </td>
      <td class="row-actions">
        <button class="icon-btn btn-danger-icon" data-action="delete" title="Remover">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="3 6 5 6 21 6"/>
            <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </button>
      </td>
    </tr>
  `).join("");

  // Eventos por linha.
  tbody.querySelectorAll("tr").forEach(tr => {
    const id = tr.dataset.id;
    tr.querySelectorAll("[data-field]").forEach(input => {
      const handleChange = () => {
        const rule = state.rules.find(r => r._id === id);
        if (!rule) return;
        rule[input.dataset.field] = input.value;
        if (input.dataset.field === "action") {
          tr.classList.toggle("rule-block", input.value === "block");
        }
        // Normaliza e auto-detecta tipo ao editar o padrão na tabela.
        if (input.dataset.field === "pattern") {
          const normalized = normalizePattern(input.value);
          if (normalized !== input.value) {
            input.value = normalized;
            rule.pattern = normalized;
          }
          const detected = detectRuleType(normalized || input.value);
          if (detected) {
            const typeSelect = tr.querySelector("[data-field='type']");
            if (typeSelect && typeSelect.value !== detected) {
              typeSelect.value = detected;
              rule.type = detected;
            }
          }
        }
        tr.classList.add("dirty");
        markDirty();
      };
      input.addEventListener("input", handleChange);
      input.addEventListener("change", handleChange);
    });
    tr.querySelector('[data-action="delete"]').addEventListener("click", () => {
      confirmDialog({
        title: "Remover regra?",
        message: "A regra será marcada para exclusão. Clique em Salvar para confirmar.",
        danger: true,
        confirmLabel: "Remover",
      }).then(ok => {
        if (!ok) return;
        state.rules = state.rules.filter(r => r._id !== id);
        markDirty();
        renderRules();
      });
    });
  });
}

function markDirty() {
  state.dirty = true;
  updateDirtyUI();
}

function updateDirtyUI() {
  document.getElementById("dirty-flag").hidden = !state.dirty;
  document.getElementById("save-rules").disabled = !state.dirty;
}

async function saveRules() {
  const cleaned = state.rules
    .filter(r => r.pattern.trim() !== "")
    .map(r => ({
      pattern: r.pattern.trim(),
      type: r.type,
      action: r.action === "block" ? "block" : undefined,
      note: (r.note || "").trim() || undefined,
    }));
  try {
    const result = await api("/api/whitelist", {
      method: "PUT",
      body: JSON.stringify({ mode: state.mode, rules: cleaned }),
    });
    state.dirty = false;
    state.rules = cleaned.map(r => ({
      _id: newId(),
      pattern: r.pattern,
      type: r.type,
      action: r.action || "allow",
      note: r.note || "",
    }));
    updateDirtyUI();
    renderRules();
    document.getElementById("nav-rule-count").textContent = state.rules.length;
    toast({ type: "success", title: "Salvo!", message: `${result.saved} regras gravadas em disco.` });
  } catch (err) {
    toast({ type: "error", title: "Falha ao salvar", message: err.message });
  }
}

async function reloadFromDisk() {
  if (state.dirty) {
    const ok = await confirmDialog({
      title: "Descartar alterações?",
      message: "Você tem alterações não salvas. Recarregar do disco vai descartá-las.",
      danger: true,
      confirmLabel: "Descartar e recarregar",
    });
    if (!ok) return;
  }
  try {
    const result = await api("/api/whitelist/reload", { method: "POST" });
    state.dirty = false;
    updateDirtyUI();
    refreshRules();
    toast({ type: "success", title: "Recarregado", message: `${result.reloaded} regras lidas do arquivo.` });
  } catch (err) {
    toast({ type: "error", title: "Erro ao recarregar", message: err.message });
  }
}

// ---------- Smart-add modal ----------

function detectRuleType(pattern) {
  const p = pattern.trim();
  if (p === "") return null;
  if (p.startsWith("*.") || p.startsWith("*")) return "wildcard";
  if (/[\^$()|\[\]\\+?{}]/.test(p)) return "regex";
  return "exact";
}

// Normaliza o padrão: "*foo.com" → "*.foo.com", remove scheme/path.
function normalizePattern(pattern) {
  let p = pattern.trim();
  // Remove scheme (https://, http://)
  const schemeIdx = p.indexOf("://");
  if (schemeIdx !== -1) p = p.slice(schemeIdx + 3);
  // Remove path (/foo/bar)
  const slashIdx = p.indexOf("/");
  if (slashIdx !== -1) p = p.slice(0, slashIdx);
  // "*foo.com" → "*.foo.com"
  if (p.startsWith("*") && !p.startsWith("*.") && p.length > 1) {
    p = "*." + p.slice(1);
  }
  return p;
}

function openAddModal({ initial = "" } = {}) {
  showModal({
    title: "Adicionar regra",
    body: `
      <div class="form-group">
        <label>Padrão <span id="detected-chip"></span></label>
        <input id="add-pattern" type="text" class="mono" autocomplete="off"
               placeholder="ex.: docs.google.com ou *.exemplo.com" value="${escapeAttr(initial)}">
        <div class="help" id="add-help">
          Use <code>*.exemplo.com</code> para liberar todos os subdomínios.
          O tipo é detectado automaticamente — você pode trocar abaixo.
        </div>
      </div>
      <div class="form-group">
        <label>Tipo</label>
        <select id="add-type">
          <option value="exact">exact — bate exatamente</option>
          <option value="wildcard">wildcard — *.dominio.com</option>
          <option value="regex">regex — expressão Go</option>
        </select>
      </div>
      <div class="form-group">
        <label>Ação</label>
        <select id="add-action">
          <option value="allow">allow — permitir acesso</option>
          <option value="block">block — bloquear acesso</option>
        </select>
      </div>
      <div class="form-group">
        <label>Nota <span class="tertiary" style="font-weight:400;text-transform:none;letter-spacing:0">(opcional)</span></label>
        <input id="add-note" type="text" placeholder="ex.: Liberado pelo time de TI">
      </div>
    `,
    actions: [
      { label: "Cancelar", className: "btn btn-ghost", onClick: () => closeModal() },
      { label: "Adicionar", className: "btn btn-primary", primary: true, onClick: submitAddModal },
    ],
  });

  const $pattern = document.getElementById("add-pattern");
  const $type    = document.getElementById("add-type");
  const $chip    = document.getElementById("detected-chip");
  const $help    = document.getElementById("add-help");

  const updateDetection = () => {
    // Normaliza o padrão antes de detectar (ex.: *foo.com → *.foo.com).
    const normalized = normalizePattern($pattern.value);
    if (normalized !== $pattern.value) {
      const pos = $pattern.selectionStart;
      $pattern.value = normalized;
      $pattern.setSelectionRange(pos, pos);
    }
    const t = detectRuleType(normalized);
    if (t) {
      $type.value = t;
      $chip.innerHTML = `<span class="detected-type">detectado: ${t}</span>`;
    } else {
      $chip.innerHTML = "";
    }
    $help.classList.remove("error", "success");
    if ($type.value === "regex" && normalized.trim()) {
      try { new RegExp(normalized); }
      catch (e) { $help.classList.add("error"); $help.textContent = "Regex inválida: " + e.message; return; }
    }
    $help.textContent = "Use *.exemplo.com para liberar todos os subdomínios. O tipo é detectado automaticamente — você pode trocar abaixo.";
  };

  $pattern.addEventListener("input", updateDetection);
  $type.addEventListener("change", updateDetection);
  updateDetection();
  $pattern.focus();
  $pattern.select();

  // Enter envia.
  $pattern.addEventListener("keydown", e => { if (e.key === "Enter") submitAddModal(); });
  document.getElementById("add-note").addEventListener("keydown", e => { if (e.key === "Enter") submitAddModal(); });
}

async function submitAddModal() {
  const pattern = normalizePattern(document.getElementById("add-pattern").value);
  const type    = document.getElementById("add-type").value;
  const action  = document.getElementById("add-action").value;
  const note    = document.getElementById("add-note").value.trim();
  if (!pattern) {
    toast({ type: "warning", title: "Padrão vazio", message: "Informe um domínio ou padrão." });
    return;
  }

  // Adiciona em memória + salva imediatamente.
  state.rules.push({ _id: newId(), pattern, type, action, note });
  closeModal();
  // Salva direto pra dar feedback imediato.
  try {
    const cleaned = state.rules.map(r => ({
      pattern: r.pattern.trim(),
      type: r.type,
      action: r.action === "block" ? "block" : undefined,
      note: (r.note || "").trim() || undefined,
    }));
    await api("/api/whitelist", { method: "PUT", body: JSON.stringify({ mode: state.mode, rules: cleaned }) });
    state.dirty = false;
    updateDirtyUI();
    refreshRules();
    refreshDashboard();
    const verb = action === "block" ? "bloqueado" : "permitido";
    toast({ type: "success", title: "Regra adicionada", message: `"${pattern}" agora é ${verb}.` });
  } catch (err) {
    // Reverte em caso de erro.
    state.rules = state.rules.filter(r => r.pattern !== pattern || r.type !== type);
    toast({ type: "error", title: "Falha ao adicionar", message: err.message });
  }
}

// --------------- LOGS ---------------

function bindLogs() {
  document.getElementById("refresh-logs").addEventListener("click", refreshLogs);
  document.getElementById("auto-refresh").addEventListener("change", e => {
    if (state.logsTimer) { clearInterval(state.logsTimer); state.logsTimer = null; }
    if (e.target.checked) state.logsTimer = setInterval(refreshLogs, 3000);
  });
  document.querySelectorAll("[data-action-filter]").forEach(btn => {
    btn.addEventListener("click", () => {
      state.filterAction = btn.dataset.actionFilter;
      document.querySelectorAll("[data-action-filter]").forEach(b => {
        b.classList.toggle("active", b.dataset.actionFilter === state.filterAction);
      });
      refreshLogs();
    });
  });
  document.getElementById("log-search").addEventListener("input", debounce(e => {
    state.filterHost = e.target.value.toLowerCase().trim();
    refreshLogs();
  }, 200));
}

async function refreshLogs() {
  try {
    const data = await api("/api/logs/recent?n=400");
    setConn(true);
    let decisions = data.decisions || [];
    if (state.filterAction) decisions = decisions.filter(d => d.action === state.filterAction);
    if (state.filterHost)   decisions = decisions.filter(d => (d.host || "").toLowerCase().includes(state.filterHost));

    const list = document.getElementById("logs-list");
    const empty = document.getElementById("logs-empty");
    if (decisions.length === 0) {
      list.innerHTML = "";
      empty.hidden = false;
      return;
    }
    empty.hidden = true;
    decisions.reverse(); // mais recentes em cima

    list.innerHTML = decisions.map(d => `
      <div class="log-row">
        <span class="log-time">${formatTimeShort(d.time)}</span>
        <span class="log-action log-action-${d.action}">${d.action}</span>
        <span class="log-host">
          ${escapeHtml(d.host || (d.action === "info" ? d.reason : "(sem host)"))}
          ${d.action !== "info" ? `<span class="log-meta">${escapeHtml(d.proto || "")} · ${escapeHtml(d.client || "")}${d.reason ? " · " + escapeHtml(d.reason) : ""}</span>` : ""}
        </span>
        ${d.action === "block" && d.host ? `
          <button class="btn btn-ghost btn-sm log-allow-btn" data-allow-host="${escapeAttr(d.host)}">
            + Permitir
          </button>
        ` : "<span></span>"}
      </div>
    `).join("");

    list.querySelectorAll("[data-allow-host]").forEach(btn => {
      btn.addEventListener("click", () => openAddModal({ initial: btn.dataset.allowHost }));
    });
  } catch (err) {
    setConn(false);
  }
}

// --------------- SETTINGS ---------------

function bindSettings() {
  document.querySelectorAll("[data-theme-set]").forEach(btn => {
    btn.addEventListener("click", (e) => {
      applyThemeWithAnimation(btn.dataset.themeSet, e);
    });
  });
  document.getElementById("configure-all-browsers").addEventListener("click", () => configureAllBrowsers());
  document.getElementById("refresh-browsers").addEventListener("click", () => loadBrowsers());
}

async function refreshSettings() {
  try {
    const s = await api("/api/status");
    setConn(true);
    document.getElementById("set-proxy").textContent     = s.proxy_addr;
    document.getElementById("set-admin").textContent     = s.admin_addr;
    document.getElementById("set-whitelist").textContent = s.whitelist_path;
    document.getElementById("set-started").textContent   = formatTime(s.started_at);
    document.getElementById("set-uptime").textContent    = formatUptime(s.uptime_seconds);
  } catch (err) {
    setConn(false);
  }
}

// --------------- BROWSERS ---------------

async function loadBrowsers() {
  try {
    const data = await api("/api/browsers");
    renderBrowsers(data.browsers || []);
  } catch (err) {
    // silencioso
  }
}

function renderBrowsers(browsers) {
  const list = document.getElementById("browsers-list");
  if (browsers.length === 0) {
    list.innerHTML = '<p class="muted" style="margin:0">Nenhum navegador detectado.</p>';
    return;
  }
  list.innerHTML = browsers.map(b => `
    <div class="browser-row">
      <div class="browser-info">
        <span class="browser-name">${escapeHtml(b.name)}</span>
        ${b.detected
          ? (b.configured
              ? '<span class="badge badge-success">configurado</span>'
              : '<span class="badge">não configurado</span>')
          : '<span class="badge" style="opacity:.5">não instalado</span>'}
        ${b.error ? `<span class="badge badge-danger" title="${escapeAttr(b.error)}">erro</span>` : ''}
      </div>
      ${b.detected ? `
        <button class="btn btn-ghost btn-sm" data-configure-browser="${escapeAttr(b.id)}">
          ${b.configured ? 'Reconfigurar' : 'Configurar'}
        </button>
      ` : ''}
    </div>
  `).join("");
  list.querySelectorAll("[data-configure-browser]").forEach(btn => {
    btn.addEventListener("click", () => configureBrowser(btn.dataset.configureBrowser));
  });
}

async function configureBrowser(id) {
  try {
    const data = await api("/api/browsers/configure", {
      method: "POST",
      body: JSON.stringify({ id }),
    });
    renderBrowsers(data.browsers || []);
    toast({ type: "success", title: "Proxy configurado", message: "Reinicie o navegador para aplicar." });
  } catch (err) {
    toast({ type: "error", title: "Erro ao configurar", message: err.message });
  }
}

async function configureAllBrowsers() {
  try {
    const data = await api("/api/browsers/configure", {
      method: "POST",
      body: JSON.stringify({}),
    });
    renderBrowsers(data.browsers || []);
    const configured = (data.browsers || []).filter(b => b.configured).length;
    toast({ type: "success", title: "Concluído", message: `${configured} navegador(es) configurado(s). Reinicie-os para aplicar.` });
  } catch (err) {
    toast({ type: "error", title: "Erro", message: err.message });
  }
}

// =================================================================
// MODAL SYSTEM
// =================================================================

let modalActions = [];

function bindModalGlobal() {
  document.getElementById("modal-root").addEventListener("click", e => {
    if (e.target.id === "modal-root") closeModal();
  });
  document.querySelectorAll("[data-modal-close]").forEach(b => {
    b.addEventListener("click", closeModal);
  });
}

function showModal({ title, body, actions = [] }) {
  document.getElementById("modal-title").textContent = title;
  document.getElementById("modal-body").innerHTML = body;
  const footer = document.getElementById("modal-footer");
  footer.innerHTML = "";
  modalActions = actions;
  actions.forEach((a, i) => {
    const btn = document.createElement("button");
    btn.className = a.className || "btn";
    btn.textContent = a.label;
    btn.addEventListener("click", () => a.onClick && a.onClick());
    footer.appendChild(btn);
    if (a.primary) setTimeout(() => btn.focus(), 0);
  });
  document.getElementById("modal-root").hidden = false;
}

function closeModal() {
  document.getElementById("modal-root").hidden = true;
  document.getElementById("modal-body").innerHTML = "";
  document.getElementById("modal-footer").innerHTML = "";
  modalActions = [];
}

function confirmDialog({ title, message, confirmLabel = "Confirmar", danger = false }) {
  return new Promise(resolve => {
    showModal({
      title,
      body: `<p style="margin:0;color:var(--fg-secondary);font-size:14px;line-height:1.6">${escapeHtml(message)}</p>`,
      actions: [
        { label: "Cancelar", className: "btn btn-ghost", onClick: () => { closeModal(); resolve(false); } },
        {
          label: confirmLabel,
          className: danger ? "btn btn-danger" : "btn btn-primary",
          primary: true,
          onClick: () => { closeModal(); resolve(true); },
        },
      ],
    });
  });
}

// =================================================================
// TOASTS
// =================================================================

const TOAST_ICONS = {
  success: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  error:   '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
  warning: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
  info:    '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>',
};

function toast({ type = "info", title, message, duration = 3500 }) {
  const root = document.getElementById("toast-root");
  const el = document.createElement("div");
  el.className = "toast toast-" + type;
  el.innerHTML = `
    <div class="toast-icon">${TOAST_ICONS[type] || TOAST_ICONS.info}</div>
    <div class="toast-body">
      <div class="toast-title">${escapeHtml(title || "")}</div>
      ${message ? `<div class="toast-message">${escapeHtml(message)}</div>` : ""}
    </div>
  `;
  root.appendChild(el);
  setTimeout(() => {
    el.classList.add("removing");
    setTimeout(() => el.remove(), 200);
  }, duration);
}

// =================================================================
// HELPERS
// =================================================================

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"
  }[c]));
}
function escapeAttr(s) { return escapeHtml(s); }

function formatTime(t) {
  if (!t) return "—";
  const d = new Date(t);
  if (isNaN(d.getTime())) return t;
  return d.toLocaleString();
}

function formatTimeShort(t) {
  if (!t) return "";
  const d = new Date(t);
  if (isNaN(d.getTime())) return t;
  return d.toLocaleTimeString();
}

function formatUptime(sec) {
  if (sec == null) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function formatNum(n) {
  if (n == null) return "0";
  return new Intl.NumberFormat("pt-BR").format(n);
}

function debounce(fn, ms) {
  let t;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}
