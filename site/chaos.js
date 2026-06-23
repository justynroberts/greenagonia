/* ============================================================
   Greenagonia — Chaos Engine + PagerDuty Events API v2
   ------------------------------------------------------------
   One scenario: bad-payment-deploy. Step data is loaded from
   /scenarios.json (generated from the Go CLI) so the site and
   CLI always fire identical events. Dedup keys match — resolve
   from either side.
   ============================================================ */

const PD_ENDPOINT      = "https://events.pagerduty.com/v2/enqueue";
const LS_KEY_ROUTING   = "gn_pd_routing_key";
const LS_KEY_CHANGE    = "gn_pd_change_key";
const LS_KEY_LD        = "gn_pd_ld_key";
const LS_KEY_INCIDENTS = "gn_pd_active_incidents";

// Resolved from ?pdenv= URL param; used in dedup keys to match the CLI.
let _pdenv = "demo";

// Populated from scenarios.json on init.
let _scenariosDoc  = null;
let _activeScenario = null; // the bad-payment-deploy scenario object from JSON

/* ------------------------------------------------------------
   Chaos controller
   ------------------------------------------------------------ */
const Chaos = {
  _cascadeTimers: [],

  get routingKey() { return localStorage.getItem(LS_KEY_ROUTING) || ""; },
  set routingKey(v) {
    localStorage.setItem(LS_KEY_ROUTING, v.trim());
    this.renderPdStatus();
  },

  get changeKey() { return localStorage.getItem(LS_KEY_CHANGE) || ""; },
  set changeKey(v) { localStorage.setItem(LS_KEY_CHANGE, v.trim()); },

  get ldKey() { return localStorage.getItem(LS_KEY_LD) || ""; },
  set ldKey(v) { localStorage.setItem(LS_KEY_LD, v.trim()); },

  get activeIncidents() {
    try { return JSON.parse(localStorage.getItem(LS_KEY_INCIDENTS)) || []; }
    catch { return []; }
  },
  set activeIncidents(list) {
    localStorage.setItem(LS_KEY_INCIDENTS, JSON.stringify(list));
    this.renderIncidentCount();
  },

  // UI fields used by the checkout flow.
  get failStep()    { return _activeScenario ? _activeScenario.ui.fail_step    : null; },
  get slowFactor()  { return _activeScenario ? _activeScenario.ui.slow_factor  : 1; },
  get errorCode()   { return _activeScenario ? _activeScenario.ui.error_code   : ""; },
  get userMessage() { return _activeScenario ? _activeScenario.ui.user_message : ""; },
  get component()   { return _activeScenario ? _activeScenario.ui.component    : ""; },

  // Returns the checkout behaviour object expected by index.html.
  current() {
    if (!this.routingKey || !_activeScenario) {
      return { failStep: null, slowFactor: 1, errorCode: null, userMessage: null, component: null };
    }
    return {
      failStep:    _activeScenario.ui.fail_step,
      slowFactor:  _activeScenario.ui.slow_factor,
      errorCode:   _activeScenario.ui.error_code,
      userMessage: _activeScenario.ui.user_message,
      component:   _activeScenario.ui.component,
    };
  },

  /* ---------- PagerDuty Events API v2 ---------- */

  async sendEvent(body) {
    if (!this.routingKey) {
      this.logEvent("error", "No routing key configured — event not sent");
      toast("⚠️ No PagerDuty routing key — alert not sent. Double-click the logo to add one.", "error");
      return { ok: false, reason: "no routing key configured" };
    }
    body.routing_key = this.routingKey;
    try {
      const res = await fetch(PD_ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const data = await res.json().catch(() => ({}));
      if (res.status === 202) return { ok: true, data };
      const reason = data.message || `HTTP ${res.status}`;
      this.logEvent("error", `PD rejected event (${res.status}): ${reason}`);
      toast(`⚠️ PagerDuty rejected the event: ${reason}`, "error");
      return { ok: false, reason };
    } catch (err) {
      this.logEvent("error", `Network error sending to PagerDuty: ${err.message}`);
      toast(`⚠️ Couldn't reach PagerDuty: ${err.message}`, "error");
      return { ok: false, reason: err.message };
    }
  },

  async sendChangeEvent(integrationKey, payload) {
    if (!integrationKey) return { ok: false };
    try {
      const res = await fetch(PD_ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ routing_key: integrationKey, event_action: "change", payload }),
      });
      return { ok: res.status === 202 };
    } catch (err) {
      return { ok: false, reason: err.message };
    }
  },

  // Fire change events for the active scenario. LaunchDarkly changes use ldKey;
  // everything else uses changeKey.
  async sendChangeEvents(sc) {
    const isoAt = (msBefore) => new Date(Date.now() - msBefore).toISOString();
    for (const ch of (sc.changes || [])) {
      const key = ch.source_tool === "LaunchDarkly" ? this.ldKey : this.changeKey;
      if (!key) continue;
      const r = await this.sendChangeEvent(key, {
        summary:        ch.summary,
        timestamp:      isoAt(ch.ago_minutes * 60 * 1000),
        source:         ch.source_tool,
        custom_details: { ...ch.custom },
        links:          ch.links || [],
      });
      if (r.ok) {
        const label = ch.summary.length > 60 ? ch.summary.slice(0, 57) + "…" : ch.summary;
        this.logEvent("change", `${ch.source_tool} · ${label} (backdated ${ch.ago_minutes}m)`);
      }
    }
  },

  // Fire the bad-payment-deploy cascade. Called on checkout failure AND from
  // the ops console "Fire incident" button.
  async triggerIncident() {
    if (!_activeScenario) return { ok: false, reason: "scenarios not loaded" };
    const sc = _activeScenario;
    const doc = _scenariosDoc;

    await this.sendChangeEvents(sc);

    const t0 = Date.now();

    const primaryHost = (service) => {
      const first = (sc.steps || []).find((s) => s.service === service);
      return first ? first.source : service;
    };

    const mergeDetails = (service, reportingHost, extra) => ({
      ...(doc ? doc.common : {}),
      ...(doc && doc.service_meta ? (doc.service_meta[service] || {}) : {}),
      ...(sc.shared || {}),
      service,
      scenario:       sc.name,
      host:           primaryHost(service),
      reporting_host: reportingHost,
      synthetic:      true,
      ...extra,
    });

    const fireStep = async (i) => {
      const st = sc.steps[i];
      const dk = `greenagonia/${_pdenv}/${sc.name}/${st.service}/${i}`;
      const result = await this.sendEvent({
        event_action: "trigger",
        dedup_key:    dk,
        payload: {
          summary:   st.summary,
          source:    st.source,
          severity:  st.severity,
          component: st.service,
          group:     sc.name,
          class:     "greenagonia-scenario",
          custom_details: mergeDetails(st.service, st.source, { ...st.extra, description: st.desc }),
        },
      });
      if (result.ok) {
        const incidents = this.activeIncidents;
        if (!incidents.includes(dk)) incidents.push(dk);
        this.activeIncidents = incidents;
        this.logEvent("trigger", `${st.severity.toUpperCase()} · ${st.service} · ${st.summary}`);
      }
      return result;
    };

    const first = await fireStep(0);

    this._cascadeTimers.forEach(clearTimeout);
    this._cascadeTimers = sc.steps.slice(1).map((st, idx) => {
      const i = idx + 1;
      const delayMs = st.delay_sec * 1000 - (Date.now() - t0);
      return setTimeout(() => fireStep(i), Math.max(0, delayMs));
    });

    if (first.ok) toast("📟 PagerDuty alert cascade started — bad-payment-deploy", "pd");
    return first;
  },

  async sendTestAlert() {
    const result = await this.sendEvent({
      event_action: "trigger",
      dedup_key: `greenagonia-test-${Date.now()}`,
      client: "Greenagonia Storefront",
      client_url: location.href,
      payload: {
        summary:   "[Greenagonia] Test alert from the operations console",
        source:    "greenagonia-checkout-prod",
        severity:  "info",
        component: "ops-console",
        group:     "commerce",
        class:     "TEST_EVENT",
        custom_details: { service: "payment-gateway", note: "If you can read this, the integration works." },
      },
    });
    if (result.ok) {
      this.logEvent("trigger", "INFO · test alert sent successfully");
      this.renderPdStatus("ok", "Test alert accepted by PagerDuty ✓");
      toast("📟 Test alert accepted by PagerDuty", "pd");
    } else {
      this.renderPdStatus("err", `Failed: ${result.reason}`);
    }
  },

  async resolveAll() {
    const incidents = this.activeIncidents;
    if (!incidents.length) { toast("No active incidents to resolve"); return; }

    this._cascadeTimers.forEach(clearTimeout);
    this._cascadeTimers = [];

    let resolved = 0;
    for (const dk of incidents) {
      // greenagonia/{env}/{scenario}/{service}/{i}
      const parts = dk.split("/");
      const service = parts.length >= 4 ? parts[3] : "payment-gateway";
      const result = await this.sendEvent({
        event_action: "resolve",
        dedup_key:    dk,
        payload: {
          summary:  "resolved",
          source:   "greenagonia-checkout-prod",
          severity: "info",
          custom_details: { service },
        },
      });
      if (result.ok) { resolved++; this.logEvent("resolve", `resolved · ${dk}`); }
    }
    this.activeIncidents = [];
    toast(`✓ Resolved ${resolved} incident${resolved === 1 ? "" : "s"}`);
  },

  /* ---------- UI ---------- */

  logEvent(kind, msg) {
    const log = document.getElementById("event-log");
    const empty = log.querySelector(".event-log__empty");
    if (empty) empty.remove();
    const row = document.createElement("div");
    row.className = "event-log__row";
    const time = new Date().toLocaleTimeString("en-GB", { hour12: false });
    row.innerHTML = `
      <span class="event-log__time">${time}</span>
      <span class="event-log__badge event-log__badge--${kind}">${kind}</span>
      <span class="event-log__msg">${msg}</span>`;
    log.prepend(row);
  },

  renderPdStatus(cls, text) {
    const el = document.getElementById("pd-status");
    if (cls) { el.className = `chaos__status ${cls}`; el.textContent = text; return; }
    if (this.routingKey) {
      el.className = "chaos__status ok";
      el.textContent = `Key configured (…${this.routingKey.slice(-4)})`;
    } else {
      el.className = "chaos__status";
      el.textContent = "No key configured";
    }
  },

  renderIncidentCount() {
    const el = document.getElementById("incident-count");
    const n = this.activeIncidents.length;
    el.textContent = n;
    el.classList.toggle("hot", n > 0);
  },

  async init() {
    // Read env from URL param — must match the CLI env for dedup keys to align.
    const params = new URLSearchParams(location.search);
    if (params.get("pdenv")) _pdenv = params.get("pdenv");

    // Load scenario definitions from the CLI-generated JSON.
    try {
      const res = await fetch("/scenarios.json");
      if (res.ok) {
        _scenariosDoc   = await res.json();
        _activeScenario = (_scenariosDoc.scenarios || []).find((s) => s.name === "bad-payment-deploy") || null;
      }
    } catch (e) {
      console.warn("Could not load scenarios.json:", e.message);
    }

    this.renderPdStatus();
    this.renderIncidentCount();

    const keyInput = document.getElementById("pd-routing-key");
    keyInput.value = this.routingKey;
    keyInput.addEventListener("change", () => (this.routingKey = keyInput.value));

    const changeInput = document.getElementById("pd-change-key");
    if (changeInput) {
      changeInput.value = this.changeKey;
      changeInput.addEventListener("change", () => (this.changeKey = changeInput.value));
    }

    const ldInput = document.getElementById("pd-ld-key");
    if (ldInput) {
      ldInput.value = this.ldKey;
      ldInput.addEventListener("change", () => (this.ldKey = ldInput.value));
    }

    document.getElementById("pd-test").addEventListener("click", () => this.sendTestAlert());
    document.getElementById("pd-fire-cascade").addEventListener("click", () => this.triggerIncident());
    document.getElementById("pd-resolve-all").addEventListener("click", () => this.resolveAll());
    document.getElementById("log-clear").addEventListener("click", () => {
      document.getElementById("event-log").innerHTML =
        '<div class="event-log__empty">No events dispatched yet</div>';
    });

    const panel   = document.getElementById("chaos-panel");
    const overlay = document.getElementById("chaos-overlay");
    const open  = () => { panel.classList.add("open");    overlay.classList.add("show"); };
    const close = () => { panel.classList.remove("open"); overlay.classList.remove("show"); };

    document.getElementById("chaos-close").addEventListener("click", close);
    const opsLink = document.getElementById("ops-link");
    if (opsLink) opsLink.addEventListener("click", (e) => { e.preventDefault(); open(); });
    overlay.addEventListener("click", close);
    document.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        panel.classList.contains("open") ? close() : open();
      }
      if (e.key === "Escape") close();
    });

    const brand = document.getElementById("footer-brand");
    if (brand) {
      let clicks = 0, timer = null;
      brand.addEventListener("click", () => {
        clicks++;
        clearTimeout(timer);
        timer = setTimeout(() => (clicks = 0), 600);
        if (clicks >= 3) { clicks = 0; open(); }
      });
    }

    let keyParamsFound = false;
    if (params.get("pdkey")) {
      this.routingKey = params.get("pdkey");
      keyInput.value  = this.routingKey;
      keyParamsFound  = true;
      params.delete("pdkey");
    }
    if (params.get("pdchangekey")) {
      this.changeKey = params.get("pdchangekey");
      if (changeInput) changeInput.value = this.changeKey;
      keyParamsFound = true;
      params.delete("pdchangekey");
    }
    if (params.get("pdldkey")) {
      this.ldKey = params.get("pdldkey");
      if (ldInput) ldInput.value = this.ldKey;
      keyParamsFound = true;
      params.delete("pdldkey");
    }
    params.delete("pdenv");

    if (keyParamsFound) {
      this.logEvent("resolve", `PagerDuty keys loaded from URL (env: ${_pdenv})`);
      const clean = location.pathname + (params.size ? `?${params}` : "") + location.hash;
      history.replaceState(null, "", clean);
      toast("📟 PagerDuty keys saved", "pd");
    }
    if (new URLSearchParams(location.search).get("ops") === "1") open();

    this.initKeyPrompt();
  },

  /* ---------- first-run routing key prompt ---------- */

  initKeyPrompt() {
    const modal        = document.getElementById("pdkey-modal");
    const input        = document.getElementById("pdkey-input");
    const clearBtn     = document.getElementById("pdkey-clear");
    const consoleInput = document.getElementById("pd-routing-key");

    const openPrompt  = () => { input.value = ""; clearBtn.hidden = !this.routingKey; modal.hidden = false; setTimeout(() => input.focus(), 60); };
    const closePrompt = () => (modal.hidden = true);

    const saveKey = () => {
      const v = input.value.trim();
      if (!v) return;
      this.routingKey = v;
      if (consoleInput) consoleInput.value = v;
      this.logEvent("resolve", "routing key saved");
      toast("📟 PagerDuty routing key saved", "pd");
      closePrompt();
    };

    document.getElementById("pdkey-form").addEventListener("submit", (e) => { e.preventDefault(); saveKey(); });
    input.addEventListener("input", () => { if (input.value.trim().length >= 32) saveKey(); });
    document.getElementById("pdkey-skip").addEventListener("click", () => { sessionStorage.setItem("gn_pdkey_skip", "1"); closePrompt(); });
    clearBtn.addEventListener("click", () => {
      localStorage.removeItem(LS_KEY_ROUTING);
      this.renderPdStatus();
      if (consoleInput) consoleInput.value = "";
      clearBtn.hidden = true;
      toast("Routing key removed");
    });

    const logo = document.getElementById("header-logo");
    if (logo) {
      let clickTimer = null;
      logo.addEventListener("click", (e) => {
        e.preventDefault();
        if (clickTimer) { clearTimeout(clickTimer); clickTimer = null; openPrompt(); }
        else { clickTimer = setTimeout(() => { clickTimer = null; location.href = logo.getAttribute("href"); }, 280); }
      });
    }

    if (!this.routingKey && !sessionStorage.getItem("gn_pdkey_skip")) openPrompt();
  },
};
