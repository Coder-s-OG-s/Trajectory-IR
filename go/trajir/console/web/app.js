(() => {
  const $ = (id) => document.getElementById(id);
  const LIVE_MS = 4000;
  const state = {
    id: null,
    events: [],
    summary: null,
    packages: [],
    savings: null,
    runs: [],
    query: "",
    tab: "overview",
    live: true,
    inflight: false,
  };

  const KIND_LABELS = {
    "node.appended": "Step logged",
    "seal.created": "Decision locked",
    "seal.verified": "Lock checked",
    "context.projected": "Context trimmed",
    "redaction.applied": "Secrets hidden",
    "export.started": "File send started",
    "export.completed": "File sent",
    "import.completed": "File received",
  };

  function sameJSON(a, b) {
    try {
      return JSON.stringify(a) === JSON.stringify(b);
    } catch (_) {
      return false;
    }
  }

  function currentTheme() {
    return document.documentElement.getAttribute("data-theme") === "light" ? "light" : "dark";
  }

  function applyTheme(theme) {
    const next = theme === "light" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    try {
      localStorage.setItem("trajir_console_theme", next);
    } catch (_) {}
    const btn = $("theme-toggle");
    if (btn) {
      btn.textContent = next === "light" ? "Dark mode" : "Light mode";
      btn.setAttribute("aria-pressed", next === "light" ? "true" : "false");
    }
  }

  function applyLive(on) {
    state.live = !!on;
    try {
      localStorage.setItem("trajir_console_live", state.live ? "on" : "off");
    } catch (_) {}
    const btn = $("live-toggle");
    const label = $("live-label");
    if (btn) btn.setAttribute("aria-pressed", state.live ? "true" : "false");
    if (label) label.textContent = state.live ? "Live" : "Paused";
  }

  function stampLive(ok) {
    const el = $("live-stamp");
    if (!el) return;
    const t = new Date().toISOString().slice(11, 19);
    el.textContent = ok ? "Updated " + t + " UTC" : "Update failed " + t + " UTC";
  }

  function token() {
    return ($("token").value || "").trim();
  }

  function saveToken() {
    sessionStorage.setItem("trajir_console_token", token());
  }

  function loadToken() {
    const t = sessionStorage.getItem("trajir_console_token") || "";
    $("token").value = t;
  }

  function headers() {
    const h = { Accept: "application/json" };
    const t = token();
    if (t) h.Authorization = `Bearer ${t}`;
    return h;
  }

  function showBanner(msg, isError) {
    const el = $("banner");
    if (!msg) {
      el.classList.add("hidden");
      el.textContent = "";
      return;
    }
    el.textContent = msg;
    el.classList.remove("hidden");
    el.classList.toggle("error", !!isError);
  }

  async function api(path) {
    const res = await fetch(path, { headers: headers() });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (_) {
      data = { error: text };
    }
    if (!res.ok) {
      const err = (data && data.error) || res.statusText || "request failed";
      throw new Error(err);
    }
    return data;
  }

  function setQueryId(id) {
    const url = new URL(window.location.href);
    if (id) url.searchParams.set("id", id);
    else url.searchParams.delete("id");
    history.replaceState({}, "", url);
  }

  function parseLocal(dt) {
    if (!dt) return null;
    const d = new Date(dt.endsWith("Z") ? dt : dt + "Z");
    return Number.isNaN(d.getTime()) ? null : d;
  }

  function inRange(ts) {
    const from = parseLocal($("from").value);
    const to = parseLocal($("to").value);
    if (!from && !to) return true;
    const t = new Date(ts);
    if (Number.isNaN(t.getTime())) return true;
    if (from && t < from) return false;
    if (to && t > to) return false;
    return true;
  }

  function filteredEvents() {
    return state.events.filter((e) => inRange(e.ts));
  }

  function fmt(n) {
    if (n === null || n === undefined) return "none";
    return String(n);
  }

  function fmtTime(ts) {
    if (!ts) return "none";
    return String(ts).replace("T", " ").replace("Z", " UTC");
  }

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function kindLabel(kind) {
    return KIND_LABELS[kind] || kind || "Event";
  }

  function dotClass(kind, payload) {
    if (kind === "seal.verified" && payload && payload.ok === false) return "bad";
    if (kind === "seal.created" || kind === "seal.verified") return "ok";
    if (kind === "context.projected" || kind === "redaction.applied") return "gold";
    return "";
  }

  function barPct(part, whole) {
    if (part === null || part === undefined || !whole) return 0;
    const n = (100 * Number(part)) / Number(whole);
    if (n < 0) return 0;
    if (n > 100) return 100;
    return n;
  }

  function stat(label, value, cls) {
    return `<div class="stat"><div class="label">${esc(label)}</div><div class="value ${cls || ""}">${esc(fmt(value))}</div></div>`;
  }

  function kpi(label, value, cls, tab) {
    return `<button type="button" class="kpi" data-jump="${esc(tab || "")}"><div class="label">${esc(label)}</div><div class="value ${cls || ""}">${esc(fmt(value))}</div></button>`;
  }

  function visibleRuns() {
    const q = (state.query || "").toLowerCase();
    return (state.runs || []).filter((r) => !q || String(r.trajectory_id).toLowerCase().includes(q));
  }

  function renderSavings() {
    const el = $("savings-total");
    if (!el) return;
    const s = state.savings || {};
    const n = s.tokens_avoided_estimated;
    el.textContent = n === null || n === undefined ? "none" : String(n);
    el.classList.toggle("ok", n !== null && n !== undefined);
  }

  function renderRunList() {
    const list = $("traj-list");
    const runs = visibleRuns();
    list.innerHTML = "";
    $("traj-empty").classList.toggle("hidden", (state.runs || []).length > 0);
    runs.forEach((run) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "traj-item";
      btn.dataset.id = run.trajectory_id;
      if (run.trajectory_id === state.id) btn.setAttribute("aria-current", "true");
      const saved = run.tokens_avoided_estimated;
      btn.innerHTML = `<span class="name">${esc(run.trajectory_id)}</span><span class="meta">${esc(fmt(run.event_count))} events, saved ${esc(fmt(saved))}</span>`;
      btn.addEventListener("click", () => selectTrajectory(run.trajectory_id));
      list.appendChild(btn);
    });
  }

  function renderHome() {
    const savings = state.savings || {};
    const runs = visibleRuns();
    const failed = (state.runs || []).reduce((n, r) => n + (r.seal_verified_fail || 0), 0);
    const files = (state.runs || []).reduce((n, r) => n + (r.exports_ok || 0) + (r.imports_ok || 0), 0);
    $("home").innerHTML = `
      <p class="lede">Everything on this page lives on this computer. Click a card to open a run. Token numbers are estimates, not a provider invoice.</p>
      <div class="stats">
        ${kpi("Tokens saved (est.)", savings.tokens_avoided_estimated, savings.tokens_avoided_estimated != null ? "ok" : "", "")}
        ${kpi("Runs", (state.runs || []).length, "", "")}
        ${kpi("Failed lock checks", failed, failed ? "bad" : "", "seals")}
        ${kpi("File moves", files, "", "transfers")}
      </div>
      <div class="run-grid">
        ${
          runs.length
            ? runs
                .map((run) => {
                  const bad = run.seal_verified_fail ? "bad" : "ok";
                  return `<button type="button" class="run-card" data-id="${esc(run.trajectory_id)}">
                    <div class="name">${esc(run.trajectory_id)}</div>
                    <div class="muted tiny">${esc(fmtTime(run.last_ts))}</div>
                    <div class="chips">
                      <span class="chip gold">saved ${esc(fmt(run.tokens_avoided_estimated))}</span>
                      <span class="chip">steps ${esc(fmt(run.node_count))}</span>
                      <span class="chip ${bad}">locks ${esc(fmt(run.seal_verified_fail))} failed</span>
                    </div>
                  </button>`;
                })
                .join("")
            : `<p class="muted">No runs match that search.</p>`
        }
      </div>
      <div class="help">
        <div><h3>What happened</h3><p>The story of one run, newest first, in plain words.</p></div>
        <div><h3>Locked decisions</h3><p>Seals freeze a plan before tools that change the world.</p></div>
        <div><h3>Files</h3><p>.tir packages stored on this PC. Open the folder. No MinIO.</p></div>
        <div><h3>Tokens</h3><p>How much context we trimmed. Guess from text size, not a bill.</p></div>
      </div>`;
    $("home").querySelectorAll(".run-card").forEach((btn) => {
      btn.addEventListener("click", () => selectTrajectory(btn.getAttribute("data-id")));
    });
  }

  function showHome() {
    state.id = null;
    setQueryId("");
    $("crumb").textContent = "All runs";
    $("detail-title").textContent = "Operator home";
    $("detail-sub").textContent = "Pick a run to see locks, files, and tokens saved.";
    $("back-home").classList.add("hidden");
    $("home").classList.remove("hidden");
    $("panels").classList.add("hidden");
    document.querySelectorAll(".traj-item").forEach((el) => el.removeAttribute("aria-current"));
    renderHome();
  }

  function eventDetail(e) {
    const p = e.payload || {};
    if (e.kind === "seal.verified") {
      return p.ok === false ? `Check failed. ${p.error || ""}`.trim() : "Check passed.";
    }
    if (e.kind === "seal.created") return `Step ${fmt(p.step_n)} locked.`;
    if (e.kind === "context.projected") return "Sent a smaller context to the model.";
    if (e.kind === "redaction.applied") return `Hid thoughts ${fmt(p.thought_collapses)}, secret fields ${fmt(p.secret_field_hits)}.`;
    if (e.kind === "export.completed" || e.kind === "import.completed") return p.path || p.console_path || "Package moved.";
    if (e.kind === "node.appended") return p.kind || "Node written.";
    return "";
  }

  function bindJumps(root) {
    root.querySelectorAll("[data-jump]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const tab = btn.getAttribute("data-jump");
        if (tab) selectTab(tab);
      });
    });
  }

  function renderOverview() {
    const s = state.summary || {};
    const economy = s.economy || {};
    const savings = state.savings || {};
    const ev = filteredEvents().slice().reverse();
    $("panel-overview").innerHTML = `
      <div class="stats">
        ${kpi("Tokens saved (est.)", economy.lifetime_tokens_avoided_estimated, economy.lifetime_tokens_avoided_estimated != null ? "ok" : "", "economy")}
        ${kpi("All runs saved (est.)", savings.tokens_avoided_estimated, savings.tokens_avoided_estimated != null ? "ok" : "", "")}
        ${kpi("Locked decisions", s.seal_created_count, "", "seals")}
        ${kpi("Failed checks", s.seal_verified_fail, s.seal_verified_fail ? "bad" : "", "seals")}
        ${kpi("Files sent", s.exports_ok, "", "transfers")}
        ${kpi("Files received", s.imports_ok, "", "transfers")}
        ${stat("Events in view", ev.length)}
        ${stat("Steps logged", s.node_count)}
      </div>
      <p class="muted lede">Newest first. Words are for people; the log still uses the real event names. Tokens saved is estimated_tokens (ceil of chars divided by 4), not a provider invoice.</p>
      <ol class="timeline">
        ${
          ev.length
            ? ev
                .slice(0, 40)
                .map((e) => {
                  const cls = dotClass(e.kind, e.payload);
                  return `<li>
                    <span class="dot ${cls}"></span>
                    <div>
                      <div class="what">${esc(kindLabel(e.kind))}</div>
                      <div class="when">${esc(fmtTime(e.ts))} <code>${esc(e.kind)}</code></div>
                      <div class="detail">${esc(eventDetail(e))}</div>
                    </div>
                  </li>`;
                })
                .join("")
            : `<li><span class="dot"></span><div class="muted">No events in this time range.</div></li>`
        }
      </ol>`;
    bindJumps($("panel-overview"));
  }

  function renderSeals() {
    const seals = filteredEvents().filter(
      (e) => e.kind === "seal.created" || e.kind === "seal.verified"
    );
    const s = state.summary || {};
    $("panel-seals").innerHTML = `
      <p class="lede muted">A lock freezes the plan. It does not freeze the outside world.</p>
      <div class="stats">
        ${stat("Decisions locked", s.seal_created_count)}
        ${stat("Checks passed", s.seal_verified_ok, "ok")}
        ${stat("Checks failed", s.seal_verified_fail, s.seal_verified_fail ? "bad" : "")}
      </div>
      <ol class="timeline">
        ${
          seals.length
            ? seals
                .map((e) => {
                  const p = e.payload || {};
                  const cls = e.kind === "seal.verified" && p.ok === false ? "bad" : "ok";
                  const detail =
                    e.kind === "seal.verified"
                      ? p.ok === false
                        ? `Failed. ${p.error || ""} ${p.content_hash || ""}`.trim()
                        : `Passed. ${p.content_hash || ""}`
                      : `Step ${fmt(p.step_n)}. ${p.content_hash || ""}`;
                  return `<li>
                    <span class="dot ${cls}"></span>
                    <div>
                      <div class="what">${esc(kindLabel(e.kind))}</div>
                      <div class="when">${esc(fmtTime(e.ts))}</div>
                      <div class="detail"><code>${esc(detail)}</code></div>
                    </div>
                  </li>`;
                })
                .join("")
            : `<li><span class="dot"></span><div class="muted">No lock events in this time range.</div></li>`
        }
      </ol>`;
  }

  function renderTransfers() {
    const s = state.summary || {};
    const view = s.transfers || {};
    const handoffs = Array.isArray(view.handoffs) ? view.handoffs : [];
    const rows = handoffs.filter((h) => inRange(h.export_ts || h.import_ts));
    const packs = state.packages || [];
    $("panel-transfers").innerHTML = `
      <p class="lede muted">Packages live under TRAJIR_CONSOLE_DATA/packages/. Host temp paths are not the console store.</p>
      <div class="stats">
        ${stat("Files sent", view.exports_ok)}
        ${stat("Files received", view.imports_ok)}
        ${stat("Moves", rows.length)}
        ${stat("Copies on this PC", packs.length)}
      </div>
      ${
        packs.length
          ? packs
              .map(
                (p) => `<div class="file-card">
                  <div>
                    <div class="name"><code>${esc(p.name)}</code></div>
                    <div class="muted tiny">${esc(fmt(p.bytes))} bytes on this computer</div>
                  </div>
                  <div class="pkg-actions">
                    <button type="button" class="btn primary" data-dl="${esc(p.name)}">Download</button>
                    <button type="button" class="btn" data-reveal="${esc(p.name)}">Show in folder</button>
                  </div>
                </div>`
              )
              .join("")
          : `<p class="muted">No local .tir copy yet. Export with a file sink, or run the demo script.</p>`
      }
      <table>
        <thead>
          <tr>
            <th>Status</th><th>Mode</th><th>Package</th><th>Bytes</th>
            <th>Members</th><th>Nodes</th><th>Verify</th><th>Source dest</th>
          </tr>
        </thead>
        <tbody>
          ${rows
            .map((h) => {
              const status = h.status || "";
              const cls = status === "failed" ? "bad" : status === "connected" ? "ok" : "";
              const label = h.redacted === true ? '<span class="chip gold">redacted</span>' : "";
              const path = h.console_path || h.export_path || h.import_path || "";
              const verify = h.verify_ok === true ? "ok" : h.verify_ok === false ? "fail" : "";
              const verifyText = [verify, h.error || ""].filter(Boolean).join(" ");
              const sources = [h.export_source, h.import_source].filter(Boolean);
              const runtime = h.runtime || "";
              const ends = sources.join(" to ");
              const who = !runtime || sources.indexOf(runtime) >= 0 ? ends || runtime : [ends, runtime].filter(Boolean).join(" ");
              const bad = h.verify_ok === false || status === "failed" ? "bad" : "";
              const statusWord =
                status === "connected" ? "linked" : status === "failed" ? "failed" : status === "export_only" ? "sent only" : status === "import_only" ? "received only" : status;
              return `<tr>
                <td class="${cls}">${esc(statusWord)}</td>
                <td>${esc(h.mode || "")} ${label}</td>
                <td><code>${esc(path)}</code></td>
                <td>${esc(fmt(h.bytes))}</td>
                <td>${esc(fmt(h.member_count))}</td>
                <td>${esc(fmt(h.node_count))}</td>
                <td class="${bad}">${esc(verifyText)}</td>
                <td>${esc(who)}</td>
              </tr>`;
            })
            .join("") || `<tr><td colspan="8" class="muted">No transfer handoffs in range.</td></tr>`}
        </tbody>
      </table>`;
    $("panel-transfers").querySelectorAll("[data-dl]").forEach((btn) => {
      btn.addEventListener("click", () => downloadPackage(btn.getAttribute("data-dl")));
    });
    $("panel-transfers").querySelectorAll("[data-reveal]").forEach((btn) => {
      btn.addEventListener("click", () =>
        postLocal("/v1/local/reveal", {
          trajectory_id: state.id,
          name: btn.getAttribute("data-reveal"),
        })
      );
    });
  }

  function economyCSV(steps) {
    const header = [
      "id",
      "ts",
      "kind",
      "step_n",
      "mode",
      "raw_estimated_tokens",
      "projected_estimated_tokens",
      "tokens_avoided_estimated",
      "dropped",
      "thought_collapses",
      "secret_field_hits",
    ];
    const lines = [header.join(",")];
    (steps || []).forEach((row) => {
      const cells = [
        row.id,
        row.ts,
        row.kind,
        row.step_n,
        row.mode,
        row.raw_estimated_tokens,
        row.projected_estimated_tokens,
        row.tokens_avoided_estimated,
        row.dropped,
        row.thought_collapses,
        row.secret_field_hits,
      ].map((v) => {
        if (v === null || v === undefined) return "";
        const s = String(v);
        return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
      });
      lines.push(cells.join(","));
    });
    return lines.join("\n") + "\n";
  }

  function downloadText(name, text, type) {
    const blob = new Blob([text], { type: type });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = name;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  function renderEconomy() {
    const s = state.summary || {};
    const economy = s.economy || {};
    const steps = (economy.steps || []).filter((row) => inRange(row.ts));
    const largest = (economy.largest || []).filter((row) => inRange(row.ts));
    const noProjection = !economy.projection_hits;
    const empty = steps.length === 0
      ? (noProjection ? "No projection events yet." : "No economy events in range.")
      : "";
    const projectionNote = noProjection && steps.length > 0 ? "No projection events yet." : "";
    const saved =
      economy.size_units_saved === null || economy.size_units_saved === undefined
        ? "Size unit savings unknown (raw_size_units was not emitted)."
        : `Latest size unit savings: ${economy.size_units_saved}.`;
    const raw = economy.raw_estimated_tokens;
    const proj = economy.projected_estimated_tokens;
    const whole = raw || 0;
    $("panel-economy").innerHTML = `
      <p class="lede muted">We count how much context we dropped before calling the model. estimated_tokens uses ceil of chars divided by 4 on the server. This is not a provider invoice.</p>
      <div class="stats">
        ${stat("Before trim", economy.raw_estimated_tokens)}
        ${stat("After trim", economy.projected_estimated_tokens)}
        ${stat("Saved last time", economy.tokens_avoided_estimated, "ok")}
        ${stat("Saved so far", economy.lifetime_tokens_avoided_estimated, "ok")}
        ${stat("Times trimmed", economy.projection_hits)}
        ${stat("Hidden bits", economy.redaction_collapses)}
      </div>
      <div class="meter">
        <div class="meter-row">
          <span>Before</span>
          <div class="meter-bar raw"><span style="width:${barPct(raw, whole)}%"></span></div>
          <strong>${esc(fmt(raw))}</strong>
        </div>
        <div class="meter-row">
          <span>After</span>
          <div class="meter-bar"><span style="width:${barPct(proj, whole)}%"></span></div>
          <strong>${esc(fmt(proj))}</strong>
        </div>
        <div class="meter-row">
          <span>Saved</span>
          <div class="meter-bar saved"><span style="width:${barPct(economy.tokens_avoided_estimated, whole)}%"></span></div>
          <strong>${esc(fmt(economy.tokens_avoided_estimated))}</strong>
        </div>
      </div>
      <p class="muted">${esc(saved)}</p>
      ${projectionNote ? `<p class="empty-economy" role="status">${esc(projectionNote)}</p>` : ""}
      <div class="economy-actions">
        <button type="button" class="btn" id="economy-json">Download JSON</button>
        <button type="button" class="btn" id="economy-csv">Download CSV</button>
      </div>
      ${
        empty
          ? `<p class="empty-economy" role="status">${esc(empty)}</p>`
          : `<table>
              <thead><tr><th>Time</th><th>What</th><th>Step</th><th>Before</th><th>After</th><th>Saved</th><th>Dropped</th><th>Hidden</th></tr></thead>
              <tbody>${steps
                .map((row) => {
                  const redaction =
                    row.kind === "redaction.applied"
                      ? `thoughts ${fmt(row.thought_collapses)}, fields ${fmt(row.secret_field_hits)}${row.mode ? ", " + row.mode : ""}`
                      : "none";
                  return `<tr>
                    <td>${esc(fmtTime(row.ts))}</td>
                    <td>${esc(kindLabel(row.kind))} <code>${esc(row.kind)}</code></td>
                    <td>${esc(fmt(row.step_n))}</td>
                    <td>${esc(fmt(row.raw_estimated_tokens))}</td>
                    <td>${esc(fmt(row.projected_estimated_tokens))}</td>
                    <td>${esc(fmt(row.tokens_avoided_estimated))}</td>
                    <td>${esc(fmt(row.dropped))}</td>
                    <td>${esc(redaction)}</td>
                  </tr>`;
                })
                .join("")}</tbody>
            </table>
            <h3>Largest context payloads</h3>
            <table>
              <thead><tr><th>Time</th><th>Step</th><th>Raw estimate</th><th>Size units</th><th>Dropped</th></tr></thead>
              <tbody>${
                largest
                  .map(
                    (row) => `<tr>
                      <td>${esc(fmtTime(row.ts))}</td>
                      <td>${esc(fmt(row.step_n))}</td>
                      <td>${esc(fmt(row.raw_estimated_tokens))}</td>
                      <td>${esc(fmt(row.size_units))}</td>
                      <td>${esc(fmt(row.dropped))}</td>
                    </tr>`
                  )
                  .join("") || `<tr><td colspan="5" class="muted">No projection rows in range.</td></tr>`
              }</tbody>
            </table>`
      }`;
    const id = state.id || "trajectory";
    const jsonBtn = $("economy-json");
    const csvBtn = $("economy-csv");
    if (jsonBtn) {
      jsonBtn.addEventListener("click", () => {
        downloadText(`${id}-economy.json`, JSON.stringify(economy, null, 2) + "\n", "application/json");
      });
    }
    if (csvBtn) {
      csvBtn.addEventListener("click", () => {
        downloadText(`${id}-economy.csv`, economyCSV(economy.steps), "text/csv");
      });
    }
  }

  function renderPanels() {
    renderSavings();
    renderRunList();
    renderHome();
    renderOverview();
    renderSeals();
    renderTransfers();
    renderEconomy();
  }

  function selectTab(name) {
    state.tab = name;
    document.querySelectorAll('[role="tab"]').forEach((btn) => {
      const on = btn.dataset.tab === name;
      btn.setAttribute("aria-selected", on ? "true" : "false");
    });
    ["overview", "seals", "transfers", "economy"].forEach((n) => {
      $("panel-" + n).classList.toggle("hidden", n !== name);
    });
  }

  async function loadSavings(quiet) {
    try {
      const dash = await api("/v1/dashboard");
      if (quiet && sameJSON(state.savings, dash.savings) && sameJSON(state.runs, dash.runs)) {
        stampLive(true);
        return;
      }
      state.savings = dash.savings || null;
      state.runs = dash.runs || [];
    } catch (_) {
      if (quiet) {
        stampLive(false);
        return;
      }
      try {
        state.savings = await api("/v1/savings");
      } catch (err) {
        state.savings = null;
      }
    }
    renderSavings();
    renderRunList();
    if (!state.id) renderHome();
    if (state.id) renderOverview();
    stampLive(true);
  }

  async function loadList() {
    saveToken();
    showBanner("");
    try {
      await loadSavings(false);
    } catch (err) {
      showBanner(String(err.message || err), true);
      $("traj-list").innerHTML = "";
      $("traj-empty").classList.add("hidden");
      state.savings = null;
      state.runs = [];
      renderSavings();
    }
  }

  async function loadTrajectory(id, quiet) {
    try {
      const [ev, sum, packs] = await Promise.all([
        api(`/v1/trajectories/${encodeURIComponent(id)}/events`),
        api(`/v1/trajectories/${encodeURIComponent(id)}/summary`),
        api(`/v1/trajectories/${encodeURIComponent(id)}/packages`).catch(() => ({ packages: [] })),
      ]);
      if (state.id !== id) return;
      const events = (ev.events || []).map((e) => ({
        ...e,
        payload: typeof e.payload === "string" ? safeParse(e.payload) : e.payload || {},
      }));
      const packages = packs.packages || [];
      if (quiet && sameJSON(state.events, events) && sameJSON(state.summary, sum) && sameJSON(state.packages, packages)) {
        stampLive(true);
        return;
      }
      state.events = events;
      state.summary = sum;
      state.packages = packages;
      renderPanels();
      selectTab(state.tab);
      if (!quiet) loadSavings(true);
      stampLive(true);
    } catch (err) {
      if (quiet) {
        stampLive(false);
        return;
      }
      state.events = [];
      state.summary = null;
      state.packages = [];
      ["overview", "seals", "transfers", "economy"].forEach((n) => {
        $("panel-" + n).innerHTML = `<p class="empty-seals" role="status">Could not load this trajectory.</p>`;
      });
      showBanner(String(err.message || err), true);
    }
  }

  async function selectTrajectory(id) {
    state.id = id;
    setQueryId(id);
    $("crumb").textContent = "Run";
    $("detail-title").textContent = id;
    $("detail-sub").innerHTML = `Open again later with <code>?id=${esc(id)}</code>`;
    $("back-home").classList.remove("hidden");
    $("home").classList.add("hidden");
    $("panels").classList.remove("hidden");
    document.querySelectorAll(".traj-item").forEach((el) => {
      el.setAttribute("aria-current", el.dataset.id === id ? "true" : "false");
    });
    showBanner("");
    await loadTrajectory(id, false);
  }

  async function tickLive() {
    if (!state.live || document.hidden || state.inflight) return;
    state.inflight = true;
    try {
      await loadSavings(true);
      if (state.id) await loadTrajectory(state.id, true);
    } finally {
      state.inflight = false;
    }
  }

  async function postLocal(path, body) {
    showBanner("");
    try {
      const res = await fetch(path, {
        method: "POST",
        headers: { ...headers(), "Content-Type": "application/json" },
        body: JSON.stringify(body || { root: true }),
      });
      const text = await res.text();
      let data = null;
      try {
        data = text ? JSON.parse(text) : null;
      } catch (_) {
        data = { error: text };
      }
      if (!res.ok) {
        throw new Error((data && data.error) || res.statusText);
      }
      showBanner("Opened " + ((data && data.path) || "local path"));
    } catch (err) {
      showBanner(String(err.message || err), true);
    }
  }

  async function downloadPackage(name) {
    if (!state.id || !name) return;
    try {
      const res = await fetch(
        `/v1/trajectories/${encodeURIComponent(state.id)}/packages/${encodeURIComponent(name)}`,
        { headers: headers() }
      );
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }
      const blob = await res.blob();
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = name;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (err) {
      showBanner(String(err.message || err), true);
    }
  }

  function safeParse(s) {
    try {
      return JSON.parse(s);
    } catch (_) {
      return {};
    }
  }

  function boot() {
    loadToken();
    try {
      state.live = localStorage.getItem("trajir_console_live") !== "off";
    } catch (_) {
      state.live = true;
    }
    applyTheme(currentTheme());
    applyLive(state.live);
    $("theme-toggle").addEventListener("click", () => {
      applyTheme(currentTheme() === "light" ? "dark" : "light");
    });
    $("live-toggle").addEventListener("click", () => applyLive(!state.live));
    document.addEventListener("visibilitychange", () => {
      if (!document.hidden && state.live) tickLive();
    });
    setInterval(tickLive, LIVE_MS);
    $("token").addEventListener("change", saveToken);
    $("refresh").addEventListener("click", loadList);
    $("open-folder").addEventListener("click", () => postLocal("/v1/local/reveal", { root: true }));
    $("open-shell").addEventListener("click", () => postLocal("/v1/local/open-shell", { root: true }));
    $("from").addEventListener("change", () => state.id && renderPanels());
    $("to").addEventListener("change", () => state.id && renderPanels());
    $("back-home").addEventListener("click", showHome);
    $("savings").addEventListener("click", showHome);
    $("run-search").addEventListener("input", () => {
      state.query = $("run-search").value || "";
      renderRunList();
      if (!state.id) renderHome();
    });
    document.querySelectorAll('[role="tab"]').forEach((btn) => {
      btn.addEventListener("click", () => selectTab(btn.dataset.tab));
    });
    document.addEventListener("keydown", (ev) => {
      if (ev.key === "/" && document.activeElement && document.activeElement.tagName !== "INPUT") {
        ev.preventDefault();
        $("run-search").focus();
      }
    });
    const params = new URLSearchParams(window.location.search);
    const initial = params.get("id");
    loadList().then(() => {
      if (initial) selectTrajectory(initial);
      else showHome();
    });
  }

  boot();
})();
