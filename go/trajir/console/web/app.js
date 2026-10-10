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
    about: null,
    aboutOpen: false,
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

  function savedOf(run) {
    const n = run && run.tokens_avoided_estimated;
    return n === null || n === undefined ? null : Number(n);
  }

  function hexTone(saved, maxSaved, failed) {
    if (failed) return "bad";
    if (saved === null || saved === undefined) return "l1";
    if (!maxSaved) return "l2";
    const t = saved / maxSaved;
    if (t >= 0.8) return "l5";
    if (t >= 0.5) return "l4";
    if (t >= 0.25) return "l3";
    if (t > 0) return "l2";
    return "l1";
  }

  function sparkPoints(values) {
    const w = 280;
    const h = 88;
    if (!values.length) return { w, h, pts: "" };
    const max = Math.max.apply(null, values.concat([1]));
    const pts = values.map((v, i) => {
      const x = values.length === 1 ? w / 2 : (i / (values.length - 1)) * w;
      const y = h - 10 - (Number(v) / max) * (h - 20);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    });
    return { w, h, pts: pts.join(" ") };
  }

  function hexRows(runs, maxSaved) {
    const pattern = [7, 7, 7, 7, 7];
    const total = pattern.reduce((n, x) => n + x, 0);
    const cells = [];
    for (let i = 0; i < total; i++) {
      const run = runs[i];
      if (run) {
        cells.push({
          run,
          tone: hexTone(savedOf(run), maxSaved, !!run.seal_verified_fail),
        });
      } else {
        cells.push({ run: null, tone: "empty" });
      }
    }
    const rows = [];
    let offset = 0;
    pattern.forEach((count, ri) => {
      rows.push({ odd: ri % 2 === 1, cells: cells.slice(offset, offset + count) });
      offset += count;
    });
    return { rows };
  }

  function renderHome() {
    const savings = state.savings || {};
    const about = state.about || {};
    const runs = visibleRuns();
    const all = state.runs || [];
    const failed = all.reduce((n, r) => n + (r.seal_verified_fail || 0), 0);
    const files = all.reduce((n, r) => n + (r.exports_ok || 0) + (r.imports_ok || 0), 0);
    const events = all.reduce((n, r) => n + (r.event_count || 0), 0);
    const savedNums = runs.map(savedOf).filter((n) => n !== null);
    const maxSaved = savedNums.length ? Math.max.apply(null, savedNums) : 0;
    const board = hexRows(runs, maxSaved);
    const newest = runs[0] && runs[0].last_ts ? fmtTime(runs[0].last_ts) : "none";
    const sparkRuns = runs.slice().sort((a, b) => String(a.last_ts || "").localeCompare(String(b.last_ts || "")));
    const sparkVals = sparkRuns.map((r) => savedOf(r) || 0);
    const spark = sparkPoints(sparkVals);
    const shareTotal = savedNums.reduce((n, x) => n + x, 0);
    const health = about.health || "none";
    $("home").innerHTML = `
      <p class="lede">Everything here is on this computer. Filled cells are real runs. Empty cells are empty. Token numbers are estimates, not a provider invoice.</p>
      <div class="stats">
        ${kpi("Tokens saved (est.)", savings.tokens_avoided_estimated, savings.tokens_avoided_estimated != null ? "ok" : "", "")}
        ${kpi("Runs", all.length, "", "")}
        ${kpi("Failed lock checks", failed, failed ? "bad" : "", "seals")}
        ${kpi("File moves", files, "", "transfers")}
      </div>
      <div class="home-board">
        <section class="heat-card">
          <div class="heat-head">
            <div>
              <h3>Run activity</h3>
              <p class="muted tiny"><span class="live-dot" aria-hidden="true"></span> ${esc(all.length)} runs on this machine</p>
            </div>
            <div class="heat-meta muted tiny">
              <div>Newest ${esc(newest)}</div>
              <div>${esc(fmt(events))} events</div>
            </div>
          </div>
          <div class="hex-wrap">
            <div class="hex-grid">
            ${board.rows
              .map(
                (row) => `<div class="hex-row ${row.odd ? "odd" : ""}">${row.cells
                  .map((cell) => {
                    if (!cell.run) {
                      return `<span class="hex empty" title="empty"></span>`;
                    }
                    const id = cell.run.trajectory_id;
                    return `<button type="button" class="hex ${cell.tone}" data-id="${esc(id)}" data-saved="${esc(fmt(savedOf(cell.run)))}" data-events="${esc(fmt(cell.run.event_count))}" data-failed="${esc(fmt(cell.run.seal_verified_fail))}" aria-label="${esc(id)}"></button>`;
                  })
                  .join("")}</div>`
              )
              .join("")}
            </div>
            <div id="heat-tip" class="heat-tip hidden" role="status"></div>
          </div>
          <div class="hex-legend" aria-hidden="true">
            <span>Less saved</span>
            <span class="hex l1"></span><span class="hex l2"></span><span class="hex l3"></span><span class="hex l4"></span><span class="hex l5"></span>
            <span>More saved (est.)</span>
            <span class="hex bad"></span>
            <span>Failed lock</span>
            <span class="hex empty"></span>
            <span>Empty</span>
          </div>
          <div class="run-index-wrap">
            <h3>Runs</h3>
            ${
              runs.length
                ? `<table class="run-index">
              <thead>
                <tr>
                  <th>Run</th>
                  <th>Last event</th>
                  <th>Events</th>
                  <th>Saved (est.)</th>
                  <th>Failed locks</th>
                  <th>Files</th>
                </tr>
              </thead>
              <tbody>
                ${runs
                  .map((run) => {
                    const fail = run.seal_verified_fail || 0;
                    const moved = (run.exports_ok || 0) + (run.imports_ok || 0);
                    return `<tr>
                    <td><button type="button" class="share-name" data-id="${esc(run.trajectory_id)}">${esc(run.trajectory_id)}</button></td>
                    <td>${esc(fmtTime(run.last_ts))}</td>
                    <td>${esc(fmt(run.event_count))}</td>
                    <td class="ok">${esc(fmt(savedOf(run)))}</td>
                    <td class="${fail ? "bad" : ""}">${esc(fmt(fail))}</td>
                    <td>${esc(fmt(moved))}</td>
                  </tr>`;
                  })
                  .join("")}
              </tbody>
            </table>`
                : `<p class="muted">No runs match that search.</p>`
            }
          </div>
        </section>
        <aside class="home-side">
          <section class="side-card">
            <h3>Tokens saved (est.)</h3>
            <p class="muted tiny">Guess from text size. Not a bill. Ordered by last event time.</p>
            <div class="side-hero ${savings.tokens_avoided_estimated != null ? "ok" : ""}">${esc(fmt(savings.tokens_avoided_estimated))}</div>
            ${
              spark.pts
                ? `<svg class="spark" viewBox="0 0 ${spark.w} ${spark.h}" aria-hidden="true"><polyline fill="none" stroke="currentColor" stroke-width="2.4" points="${spark.pts}"></polyline></svg>`
                : `<p class="muted tiny">No projection events yet.</p>`
            }
          </section>
          <section class="side-card">
            <h3>This machine</h3>
            <ul class="health-list">
              <li><span>Health</span><strong class="${health === "ok" ? "ok" : ""}">${esc(health)}</strong></li>
              <li><span>Packages</span><strong>${esc(fmt(about.package_count))}</strong></li>
              <li><span>Bytes on disk</span><strong>${esc(fmt(about.data_bytes))}</strong></li>
              <li><span>API token</span><strong>${about.auth_required ? "required" : "not set"}</strong></li>
              <li><span>Folder tools</span><strong>${about.loopback_tools ? "this machine only" : "none"}</strong></li>
            </ul>
          </section>
          <section class="side-card">
            <h3>Share of saved (est.)</h3>
            <p class="muted tiny">Each bar is one run. No forecast. No parade.</p>
            ${
              runs.length
                ? runs
                    .map((run) => {
                      const n = savedOf(run);
                      const pct = n === null || !shareTotal ? 0 : barPct(n, shareTotal);
                      return `<div class="share-row">
                        <button type="button" class="share-name" data-id="${esc(run.trajectory_id)}">${esc(run.trajectory_id)}</button>
                        <div class="meter-bar saved"><span style="width:${pct}%"></span></div>
                        <span class="tiny">${esc(fmt(n))}</span>
                      </div>`;
                    })
                    .join("")
                : `<p class="muted tiny">No runs yet.</p>`
            }
          </section>
        </aside>
      </div>
      <div class="help">
        <div><h3>What happened</h3><p>The story of one run, newest first, in plain words.</p></div>
        <div><h3>Locked decisions</h3><p>Seals freeze a plan before tools that change the world.</p></div>
        <div><h3>Files</h3><p>.tir packages stored on this PC. Open the folder. No MinIO.</p></div>
        <div><h3>Tokens</h3><p>How much context we trimmed. Guess from text size, not a bill.</p></div>
        <div><h3>License</h3><p>Apache License 2.0. No product key. No expiry.</p></div>
      </div>`;
    bindHome();
  }

  function bindHome() {
    const home = $("home");
    const tip = $("heat-tip");
    home.querySelectorAll(".hex[data-id], .run-card[data-id], .share-name[data-id], .run-index [data-id]").forEach((btn) => {
      btn.addEventListener("click", () => selectTrajectory(btn.getAttribute("data-id")));
    });
    home.querySelectorAll(".hex[data-id]").forEach((btn) => {
      btn.addEventListener("mouseenter", () => {
        if (!tip) return;
        tip.classList.remove("hidden");
        tip.innerHTML = `<div class="name">${esc(btn.getAttribute("data-id"))}</div>
          <div>saved ${esc(btn.getAttribute("data-saved"))} (est.)</div>
          <div>${esc(btn.getAttribute("data-events"))} events, locks failed ${esc(btn.getAttribute("data-failed"))}</div>`;
      });
      btn.addEventListener("mouseleave", () => {
        if (tip) tip.classList.add("hidden");
      });
    });
    bindJumps(home);
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
    loadAbout(true);
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

  function boolLabel(v) {
    if (v === true) return "ok";
    if (v === false) return "fail";
    return "none";
  }

  function boolClass(v) {
    if (v === true) return "ok";
    if (v === false) return "bad";
    return "";
  }

  function renderEvidence() {
    const s = state.summary || {};
    const ev = s.evidence || {};
    const tools = Array.isArray(ev.tool_calls) ? ev.tool_calls.filter((row) => inRange(row.ts || "")) : [];
    const gaps = Array.isArray(ev.seal_before_execute_gaps) ? ev.seal_before_execute_gaps : [];
    const findings = Array.isArray(ev.audit_findings) ? ev.audit_findings : [];
    const openWorld = Array.isArray(ev.open_world_tools) ? ev.open_world_tools : [];
    const empty =
      !ev.seal_count &&
      tools.length === 0 &&
      (ev.audit_ok === undefined || ev.audit_ok === null)
        ? "No evidence events yet. Seal a step, run tools, then trajir verify."
        : "";
    $("panel-evidence").innerHTML = `
      <div class="stats">
        ${stat("Seals", ev.seal_count)}
        ${stat("Seal-before-execute", boolLabel(ev.seal_before_execute_ok), boolClass(ev.seal_before_execute_ok))}
        ${stat("Gaps", gaps.length, gaps.length ? "bad" : "")}
        ${stat("trajir verify", boolLabel(ev.audit_ok), boolClass(ev.audit_ok))}
        ${stat("Open-world tools", openWorld.length || "none", openWorld.length ? "bad" : "")}
      </div>
      <p class="muted">TURNING_POINT evidence: console Chain-of-Evidence requires a DECISION before every TOOL_CALL (stricter than <code>trajir verify</code>, which exempts some pure/read-only tools). Also shows open-world flags, hashed idempotency keys, and offline audit results.</p>
      ${ev.last_audit_path ? `<p class="muted">Last audit: <code>${esc(ev.last_audit_path)}</code>${ev.last_audit_ts ? " @ " + esc(ev.last_audit_ts) : ""}</p>` : ""}
      ${
        findings.length
          ? `<h3>Audit findings</h3><ul class="findings">${findings
              .map((f) => `<li><code>${esc(f)}</code></li>`)
              .join("")}</ul>`
          : ""
      }
      ${
        openWorld.length
          ? `<p class="muted">Open-world: ${openWorld.map((n) => `<code>${esc(n)}</code>`).join(", ")}</p>`
          : ""
      }
      ${
        gaps.length
          ? `<h3>Seal-before-execute gaps</h3>
             <table>
               <thead><tr><th>Step</th><th>Seq</th><th>Tool</th></tr></thead>
               <tbody>${gaps
                 .map(
                   (g) =>
                     `<tr class="bad"><td>${esc(fmt(g.step_n))}</td><td>${esc(fmt(g.seq))}</td><td><code>${esc(g.tool_name || "")}</code></td></tr>`
                 )
                 .join("")}</tbody>
             </table>`
          : ""
      }
      ${
        empty
          ? `<p class="empty-economy" role="status">${esc(empty)}</p>`
          : `<h3>Tool calls</h3>
             <table>
               <thead><tr><th>Time</th><th>Step</th><th>Seq</th><th>Tool</th><th>Effect</th><th>Idempotency</th><th>Open-world</th></tr></thead>
               <tbody>${
                 tools
                   .map((row) => {
                     const ow = row.open_world === true;
                     return `<tr>
                       <td>${esc(row.ts || "")}</td>
                       <td>${esc(fmt(row.step_n))}</td>
                       <td>${esc(fmt(row.seq))}</td>
                       <td><code>${esc(row.tool_name || "")}</code></td>
                       <td>${esc(row.effect_class || "none")}</td>
                       <td><code>${esc(row.idempotency_key || "none")}</code></td>
                       <td class="${ow ? "bad" : ""}">${ow ? "yes" : "no"}</td>
                     </tr>`;
                   })
                   .join("") || `<tr><td colspan="7" class="muted">No TOOL_CALL observations in range.</td></tr>`
               }</tbody>
             </table>`
      }`;
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

  function showAbout(on) {
    state.aboutOpen = !!on;
    const el = $("about");
    if (!el) return;
    el.classList.toggle("hidden", !state.aboutOpen);
    if (state.aboutOpen) loadAbout(false);
  }

  function renderAbout() {
    const el = $("about-body");
    if (!el) return;
    const a = state.about || {};
    const lic = a.license || {};
    const auth = a.auth_required ? "API token is required" : "API token is not set (open on this machine)";
    const tools = a.loopback_tools ? "Folder and PowerShell only work from this machine" : "none";
    el.innerHTML = `
      <h3>License</h3>
      <p>${esc(lic.name || "Apache License 2.0")}. SPDX ${esc(lic.spdx || "Apache-2.0")}.</p>
      <p>No product key. No expiry. This is not a paid support contract.</p>
      <p><a href="https://www.apache.org/licenses/LICENSE-2.0" target="_blank" rel="noopener noreferrer">Read Apache License 2.0</a></p>
      <p><a href="https://github.com/Coder-s-OG-s/Trajectory-IR" target="_blank" rel="noopener noreferrer">Source on GitHub</a></p>
      <h3>This computer</h3>
      <div class="about-facts">
        ${stat("Health", a.health || "none", a.health === "ok" ? "ok" : "")}
        ${stat("Runs", a.run_count)}
        ${stat("Events", a.event_count)}
        ${stat("Packages", a.package_count)}
        ${stat("Bytes on disk", a.data_bytes)}
      </div>
      <p>Data folder: <code>${esc(a.data_dir || "none")}</code></p>
      <p>${esc(auth)}</p>
      <p>${esc(tools)}</p>
      <p class="tiny">This page is local observation. It is not object storage, IAM, or a remote control plane.</p>`;
  }

  async function loadAbout(quiet) {
    try {
      const about = await api("/v1/about");
      if (quiet && sameJSON(state.about, about)) return;
      state.about = about;
      renderAbout();
      if (!state.id) renderHome();
    } catch (err) {
      if (quiet) return;
      state.about = null;
      renderAbout();
      showBanner(String(err.message || err), true);
    }
  }

  function renderPanels() {
    renderSavings();
    renderRunList();
    renderHome();
    renderOverview();
    renderSeals();
    renderEvidence();
    renderTransfers();
    renderEconomy();
  }

  function selectTab(name) {
    state.tab = name;
    document.querySelectorAll('[role="tab"]').forEach((btn) => {
      const on = btn.dataset.tab === name;
      btn.setAttribute("aria-selected", on ? "true" : "false");
    });
    ["overview", "seals", "evidence", "transfers", "economy"].forEach((n) => {
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
      ["overview", "seals", "evidence", "transfers", "economy"].forEach((n) => {
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
      if (state.aboutOpen) await loadAbout(true);
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
    $("open-about").addEventListener("click", () => showAbout(true));
    $("about-close").addEventListener("click", () => showAbout(false));
    $("about").addEventListener("click", (ev) => {
      if (ev.target === $("about")) showAbout(false);
    });
    document.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape" && state.aboutOpen) showAbout(false);
    });
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
