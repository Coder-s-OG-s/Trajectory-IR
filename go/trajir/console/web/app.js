(() => {
  const $ = (id) => document.getElementById(id);
  const state = {
    id: null,
    events: [],
    summary: null,
    tab: "overview",
  };

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
    const d = new Date(dt);
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
    if (n === null || n === undefined) return "—";
    return String(n);
  }

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function stat(label, value, cls) {
    return `<div class="stat"><div class="label">${esc(label)}</div><div class="value ${cls || ""}">${esc(fmt(value))}</div></div>`;
  }

  function renderOverview() {
    const s = state.summary || {};
    const ev = filteredEvents();
    $("panel-overview").innerHTML = `
      <div class="stats">
        ${stat("Events (filtered)", ev.length)}
        ${stat("Nodes", s.node_count)}
        ${stat("Last ts", s.last_ts || "—")}
        ${stat("Seals created", s.seal_created_count)}
        ${stat("Verify fail", s.seal_verified_fail, s.seal_verified_fail ? "bad" : "")}
        ${stat("Exports OK", s.exports_ok)}
        ${stat("Imports OK", s.imports_ok)}
      </div>
      <p class="muted">Overview uses server summary plus the time filter on the event list below.</p>
      <table>
        <thead><tr><th>Time</th><th>Kind</th><th>Source</th><th>Id</th></tr></thead>
        <tbody>
          ${ev
            .slice()
            .reverse()
            .slice(0, 50)
            .map(
              (e) =>
                `<tr><td>${esc(e.ts)}</td><td><code>${esc(e.kind)}</code></td><td>${esc(e.source)}</td><td><code>${esc(e.id)}</code></td></tr>`
            )
            .join("") || `<tr><td colspan="4" class="muted">No events in range.</td></tr>`}
        </tbody>
      </table>`;
  }

  function renderSeals() {
    const seals = filteredEvents().filter(
      (e) => e.kind === "seal.created" || e.kind === "seal.verified"
    );
    const s = state.summary || {};
    $("panel-seals").innerHTML = `
      <div class="stats">
        ${stat("Created", s.seal_created_count)}
        ${stat("Verified OK", s.seal_verified_ok, "ok")}
        ${stat("Verified fail", s.seal_verified_fail, s.seal_verified_fail ? "bad" : "")}
      </div>
      <table>
        <thead><tr><th>Time</th><th>Kind</th><th>Payload</th></tr></thead>
        <tbody>
          ${seals
            .map((e) => {
              const p = e.payload || {};
              const detail =
                e.kind === "seal.verified"
                  ? `ok=${p.ok} hash=${p.content_hash || ""}${p.error ? " err=" + p.error : ""}`
                  : `step=${p.step_n} node=${p.node_id || ""} hash=${p.content_hash || ""}`;
              const cls = e.kind === "seal.verified" && p.ok === false ? "bad" : "";
              return `<tr><td>${esc(e.ts)}</td><td class="${cls}"><code>${esc(e.kind)}</code></td><td><code>${esc(detail)}</code></td></tr>`;
            })
            .join("") || `<tr><td colspan="3" class="muted">No seal events in range.</td></tr>`}
        </tbody>
      </table>`;
  }

  function renderTransfers() {
    const xfer = filteredEvents().filter(
      (e) =>
        e.kind === "export.started" ||
        e.kind === "export.completed" ||
        e.kind === "import.completed"
    );
    const s = state.summary || {};
    const verify =
      s.transfer_verify_ok === true ? "ok" : s.transfer_verify_ok === false ? "fail" : "—";
    $("panel-transfers").innerHTML = `
      <div class="stats">
        ${stat("Last mode", s.last_package_mode || "—")}
        ${stat("Last bytes", s.last_package_bytes || "—")}
        ${stat("Members", s.last_package_members || "—")}
        ${stat("Verify", verify, verify === "ok" ? "ok" : verify === "fail" ? "bad" : "")}
      </div>
      <table>
        <thead><tr><th>Time</th><th>Kind</th><th>Detail</th></tr></thead>
        <tbody>
          ${xfer
            .map((e) => {
              const p = e.payload || {};
              const detail = [
                p.mode && `mode=${p.mode}`,
                p.redacted !== undefined && `redacted=${p.redacted}`,
                p.bytes !== undefined && `bytes=${p.bytes}`,
                p.path && `path=${p.path}`,
                p.ok !== undefined && `ok=${p.ok}`,
                p.verify_ok !== undefined && `verify_ok=${p.verify_ok}`,
                p.error && `error=${p.error}`,
              ]
                .filter(Boolean)
                .join(" ");
              return `<tr><td>${esc(e.ts)}</td><td><code>${esc(e.kind)}</code></td><td><code>${esc(detail)}</code></td></tr>`;
            })
            .join("") || `<tr><td colspan="3" class="muted">No transfer events in range.</td></tr>`}
        </tbody>
      </table>`;
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
        ? "Size-unit savings unknown (raw_size_units was not emitted)."
        : `Latest size-unit savings: ${economy.size_units_saved}.`;
    $("panel-economy").innerHTML = `
      <div class="stats">
        ${stat("Latest raw est. tokens", economy.raw_estimated_tokens)}
        ${stat("Latest projected est. tokens", economy.projected_estimated_tokens)}
        ${stat("Latest tokens avoided (est.)", economy.tokens_avoided_estimated)}
        ${stat("Lifetime tokens avoided (est.)", economy.lifetime_tokens_avoided_estimated)}
        ${stat("Projection hits", economy.projection_hits)}
        ${stat("Redaction collapses", economy.redaction_collapses)}
      </div>
      <p class="muted">estimated_tokens uses ceil(chars/4) on the server. This is not a provider invoice.</p>
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
              <thead><tr><th>Time</th><th>Kind</th><th>Step</th><th>Raw est.</th><th>Projected est.</th><th>Avoided est.</th><th>Dropped</th><th>Redaction</th></tr></thead>
              <tbody>${steps
                .map((row) => {
                  const redaction =
                    row.kind === "redaction.applied"
                      ? `thoughts ${fmt(row.thought_collapses)}, fields ${fmt(row.secret_field_hits)}${row.mode ? ", " + row.mode : ""}`
                      : "—";
                  return `<tr>
                    <td>${esc(row.ts)}</td>
                    <td><code>${esc(row.kind)}</code></td>
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
              <thead><tr><th>Time</th><th>Step</th><th>Raw est.</th><th>Size units</th><th>Dropped</th></tr></thead>
              <tbody>${
                largest
                  .map(
                    (row) => `<tr>
                      <td>${esc(row.ts)}</td>
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

  async function loadList() {
    saveToken();
    showBanner("");
    try {
      const data = await api("/v1/trajectories");
      const ids = data.trajectories || [];
      const list = $("traj-list");
      list.innerHTML = "";
      $("traj-empty").classList.toggle("hidden", ids.length > 0);
      ids.forEach((id) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "traj-item";
        btn.textContent = id;
        btn.dataset.id = id;
        if (id === state.id) btn.setAttribute("aria-current", "true");
        btn.addEventListener("click", () => selectTrajectory(id));
        list.appendChild(btn);
      });
    } catch (err) {
      showBanner(String(err.message || err), true);
      $("traj-empty").classList.remove("hidden");
    }
  }

  async function selectTrajectory(id) {
    state.id = id;
    setQueryId(id);
    $("detail-title").textContent = id;
    $("detail-sub").innerHTML = `Deep link: <code>?id=${esc(id)}</code>`;
    $("placeholder").classList.add("hidden");
    $("panels").classList.remove("hidden");
    document.querySelectorAll(".traj-item").forEach((el) => {
      el.setAttribute("aria-current", el.dataset.id === id ? "true" : "false");
    });
    showBanner("");
    try {
      const [ev, sum] = await Promise.all([
        api(`/v1/trajectories/${encodeURIComponent(id)}/events`),
        api(`/v1/trajectories/${encodeURIComponent(id)}/summary`),
      ]);
      state.events = (ev.events || []).map((e) => ({
        ...e,
        payload: typeof e.payload === "string" ? safeParse(e.payload) : e.payload || {},
      }));
      state.summary = sum;
      renderPanels();
      selectTab(state.tab);
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
    $("token").addEventListener("change", saveToken);
    $("refresh").addEventListener("click", loadList);
    $("from").addEventListener("change", () => state.id && renderPanels());
    $("to").addEventListener("change", () => state.id && renderPanels());
    document.querySelectorAll('[role="tab"]').forEach((btn) => {
      btn.addEventListener("click", () => selectTab(btn.dataset.tab));
    });
    const params = new URLSearchParams(window.location.search);
    const initial = params.get("id");
    loadList().then(() => {
      if (initial) selectTrajectory(initial);
    });
  }

  boot();
})();
