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

  function sealStatusClass(status) {
    if (status === "created" || status === "verified" || status === "failed") return status;
    return "created";
  }

  function sealRange(row) {
    const bits = [];
    if (row.from_seq != null && row.to_seq != null) bits.push(`${row.from_seq}–${row.to_seq}`);
    if (row.from_node_id && row.to_node_id) {
      bits.push(
        row.from_node_id === row.to_node_id
          ? row.from_node_id
          : `${row.from_node_id} → ${row.to_node_id}`
      );
    }
    if (row.covered_nodes) bits.push(`${row.covered_nodes} nodes`);
    if (row.content_hash) bits.push(row.content_hash);
    if (row.tool_names && row.tool_names.length) bits.push(`tools: ${row.tool_names.join(", ")}`);
    return bits.join(" · ") || "—";
  }

  function renderSeals() {
    const s = state.summary || {};
    const all = s.seals || [];
    const rows = all.filter((row) => inRange(row.ts));
    let empty = "";
    if (rows.length === 0) {
      if (all.length === 0 && s.node_count > 0) {
        const noun = s.node_count === 1 ? "node" : "nodes";
        empty = `This trajectory has ${s.node_count} ${noun} and no seals yet.`;
      } else if (all.length === 0) {
        empty = "No seal events yet.";
      } else {
        empty = "No seal events in range.";
      }
    }
    const body = rows
      .map((row) => {
        const failed = row.status === "failed";
        const reason = row.error ? `<span class="reason">${esc(row.error)}</span>` : "—";
        return `<tr class="${failed ? "seal-failed" : ""}">
          <td>${esc(row.ts)}</td>
          <td><span class="badge ${sealStatusClass(row.status)}">${esc(row.status)}</span></td>
          <td><code>${esc(row.kind)}</code></td>
          <td class="${row.chain === "break" ? "bad" : "ok"}">${esc(row.chain || "—")}</td>
          <td><code>${esc(row.node_id || "—")}</code>${row.step_n != null ? `<div class="muted">step ${esc(row.step_n)}</div>` : ""}</td>
          <td class="hash"><code>${esc(sealRange(row))}</code></td>
          <td>${reason}</td>
        </tr>`;
      })
      .join("");
    $("panel-seals").innerHTML = `
      <div class="stats">
        ${stat("Created", s.seal_created_count)}
        ${stat("Verified OK", s.seal_verified_ok, "ok")}
        ${stat("Verified fail", s.seal_verified_fail, s.seal_verified_fail ? "bad" : "")}
      </div>
      ${
        empty
          ? `<p class="empty-seals" role="status">${esc(empty)}</p>`
          : `<table>
              <thead><tr><th>Time</th><th>Status</th><th>Event</th><th>Chain</th><th>Node</th><th>Range / hash</th><th>Reason</th></tr></thead>
              <tbody>${body}</tbody>
            </table>`
      }`;
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

  function renderEconomy() {
    const s = state.summary || {};
    const note =
      s.tokens_avoided_estimated === null || s.tokens_avoided_estimated === undefined
        ? "Token avoidance is null when raw/projected char lengths were not emitted."
        : "estimated_tokens = ceil(chars/4). Not provider billing.";
    $("panel-economy").innerHTML = `
      <div class="stats">
        ${stat("Projection units", s.projection_size_units)}
        ${stat("Budget", s.projection_budget)}
        ${stat("Nodes dropped", s.nodes_dropped)}
        ${stat("Raw est. tokens", s.raw_estimated_tokens)}
        ${stat("Projected est. tokens", s.projected_estimated_tokens)}
        ${stat("Tokens avoided (est.)", s.tokens_avoided_estimated)}
        ${stat("Redaction collapses", s.redaction_collapses)}
      </div>
      <p class="muted">${esc(note)}</p>
      <table>
        <thead><tr><th>Time</th><th>Kind</th><th>Detail</th></tr></thead>
        <tbody>
          ${filteredEvents()
            .filter((e) => e.kind === "context.projected" || e.kind === "redaction.applied")
            .map((e) => {
              const p = e.payload || {};
              const detail = JSON.stringify(p);
              return `<tr><td>${esc(e.ts)}</td><td><code>${esc(e.kind)}</code></td><td><code>${esc(detail)}</code></td></tr>`;
            })
            .join("") || `<tr><td colspan="3" class="muted">No economy events in range.</td></tr>`}
        </tbody>
      </table>`;
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
