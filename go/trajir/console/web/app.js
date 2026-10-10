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
    pageSize: 10,
    page: {},
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

  function kindCode(kind) {
    return esc(kind || "").replaceAll(".", ".<wbr>");
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

  function pageSizeNow() {
    return state.pageSize === 20 ? 20 : 10;
  }

  function takePage(key, items) {
    const list = items || [];
    const total = list.length;
    const size = pageSizeNow();
    const pages = total > 0 ? ((total + size - 1) / size) | 0 : 1;
    let page = Number(state.page[key]);
    if (!Number.isFinite(page) || page < 1) page = 1;
    if (page > pages) page = pages;
    state.page[key] = page;
    const start = (page - 1) * size;
    return { rows: list.slice(start, start + size), total, page, pages, size, start };
  }

  function pagerHTML(key, total) {
    if (!(total > 10)) return "";
    const size = pageSizeNow();
    const pages = ((total + size - 1) / size) | 0;
    let page = Number(state.page[key]);
    if (!Number.isFinite(page) || page < 1) page = 1;
    if (page > pages) page = pages;
    const from = (page - 1) * size + 1;
    const to = page * size > total ? total : page * size;
    const prevOff = page <= 1 ? " disabled" : "";
    const nextOff = page >= pages ? " disabled" : "";
    const on10 = size === 10 ? ' aria-pressed="true"' : "";
    const on20 = size === 20 ? ' aria-pressed="true"' : "";
    return `<nav class="pager" data-pager="${esc(key)}" aria-label="Pages">
      <button type="button" class="btn" data-page="prev"${prevOff}>Previous</button>
      <span class="pager-status">Page ${page} of ${pages}</span>
      <button type="button" class="btn" data-page="next"${nextOff}>Next</button>
      <span class="pager-size">Per page</span>
      <button type="button" class="btn" data-size="10"${on10}>10</button>
      <button type="button" class="btn" data-size="20"${on20}>20</button>
      <span class="pager-count">${from} to ${to} of ${total}</span>
    </nav>`;
  }

  function resetPages(keys) {
    keys.forEach((key) => {
      state.page[key] = 1;
    });
  }

  function refreshAfterPager(key, sizeChanged) {
    if (sizeChanged || key === "sidebar" || String(key).indexOf("home-") === 0) {
      renderRunList();
    }
    if (sizeChanged) {
      if (!state.id) renderHome();
      else {
        renderOverview();
        renderSeals();
        renderTransfers();
        renderEconomy();
      }
      return;
    }
    if (key === "home-runs" || key === "home-share" || key === "home-tokens") {
      if (!state.id) renderHome();
      return;
    }
    if (key === "overview") renderOverview();
    else if (key === "seals") renderSeals();
    else if (key === "transfers-packs" || key === "transfers-rows") renderTransfers();
    else if (key === "economy-steps" || key === "economy-largest") renderEconomy();
  }

  function bindPager(root) {
    if (!root) return;
    root.querySelectorAll("nav.pager").forEach((nav) => {
      const key = nav.getAttribute("data-pager") || "";
      nav.querySelectorAll("[data-page]").forEach((btn) => {
        btn.addEventListener("click", () => {
          if (btn.disabled) return;
          const cur = Number(state.page[key]) || 1;
          state.page[key] = btn.getAttribute("data-page") === "next" ? cur + 1 : cur - 1;
          refreshAfterPager(key, false);
        });
      });
      nav.querySelectorAll("[data-size]").forEach((btn) => {
        btn.addEventListener("click", () => {
          const next = btn.getAttribute("data-size") === "20" ? 20 : 10;
          if (state.pageSize === next) return;
          state.pageSize = next;
          state.page = {};
          refreshAfterPager(key, true);
        });
      });
    });
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
    const view = takePage("sidebar", runs);
    list.innerHTML = "";
    $("traj-empty").classList.toggle("hidden", (state.runs || []).length > 0);
    view.rows.forEach((run) => {
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
    const pager = pagerHTML("sidebar", runs.length);
    if (pager) {
      const holder = document.createElement("div");
      holder.innerHTML = pager;
      if (holder.firstElementChild) list.appendChild(holder.firstElementChild);
      bindPager(list);
    }
  }

  function savedOf(run) {
    const n = run && run.tokens_avoided_estimated;
    return n === null || n === undefined ? null : Number(n);
  }

  function fmtText(s) {
    if (s === null || s === undefined || s === "") return "none";
    return String(s);
  }

  function formatUsd(n) {
    return Number(n).toFixed(6);
  }

  function loadPrices() {
    try {
      const raw = localStorage.getItem("trajir_console_prices");
      if (!raw) return {};
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
      return parsed;
    } catch (_) {
      return {};
    }
  }

  function priceComplete(price) {
    if (!price || typeof price !== "object") return false;
    const model = String(price.model || "").trim();
    if (!model || model.length > 200) return false;
    const input = Number(price.input_usd_per_million);
    const output = Number(price.output_usd_per_million);
    if (!Number.isFinite(input) || !Number.isFinite(output) || input < 0 || output < 0) return false;
    if (!/^\d{4}-\d{2}-\d{2}$/.test(String(price.priced_on || ""))) return false;
    const source = String(price.source || "").trim();
    if (!source || source.length > 300) return false;
    return true;
  }

  function listIllustration(run, prices) {
    if (!run) return null;
    const model = String(run.model || "").trim();
    if (!model || run.prompt_tokens === null || run.prompt_tokens === undefined) return null;
    if (run.completion_tokens === null || run.completion_tokens === undefined) return null;
    const price = prices[model];
    if (!priceComplete(price) || String(price.model || "").trim() !== model) return null;
    const prompt = Number(run.prompt_tokens);
    const completion = Number(run.completion_tokens);
    if (!Number.isFinite(prompt) || !Number.isFinite(completion) || prompt < 0 || completion < 0) return null;
    const input = (prompt / 1000000) * Number(price.input_usd_per_million);
    const output = (completion / 1000000) * Number(price.output_usd_per_million);
    if (!Number.isFinite(input) || !Number.isFinite(output)) return null;
    return { input: input, output: output };
  }

  function listPriceLabel(model, prices) {
    const key = String(model || "").trim();
    if (!key) return "none";
    const price = prices[key];
    if (!priceComplete(price)) return "none";
    return `in ${price.input_usd_per_million} USD per million, out ${price.output_usd_per_million} USD per million, dated ${price.priced_on}, source ${price.source}`;
  }

  function illustrationText(run, prices) {
    const pic = listIllustration(run, prices);
    if (!pic) return "none";
    return `in ${formatUsd(pic.input)} USD, out ${formatUsd(pic.output)} USD`;
  }

  function illustrationHTML(run, prices) {
    const pic = listIllustration(run, prices);
    if (!pic) return "none";
    return `<div>in ${esc(formatUsd(pic.input))} USD</div><div>out ${esc(formatUsd(pic.output))} USD</div>`;
  }

  function renderPriceList() {
    const el = $("price-saved");
    if (!el) return;
    const prices = loadPrices();
    const rows = Object.keys(prices)
      .filter((key) => priceComplete(prices[key]) && String(prices[key].model || "").trim() === key)
      .sort();
    el.innerHTML = rows.length
      ? rows
          .map((key) => {
            const price = prices[key];
            return `<div class="price-row">
              <span><strong>${esc(key)}</strong> ${esc(listPriceLabel(key, prices))}</span>
              <button type="button" class="btn ghost" data-forget-price="${esc(key)}">Remove</button>
            </div>`;
          })
          .join("")
      : `<p class="muted tiny">No list price on this browser yet.</p>`;
    el.querySelectorAll("[data-forget-price]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const all = loadPrices();
        delete all[btn.getAttribute("data-forget-price")];
        try {
          localStorage.setItem("trajir_console_prices", JSON.stringify(all));
        } catch (_) {
          showBanner("This browser blocked local storage. The price was not removed.", true);
          return;
        }
        renderPriceList();
        if (!state.id) renderHome();
      });
    });
  }

  function shown(v) {
    const s = v === null || v === undefined ? "" : String(v).trim();
    return s ? s : "none";
  }

  function tokenFact(label, valueHTML) {
    return `<div class="token-fact"><div class="label">${esc(label)}</div><div class="value">${valueHTML}</div></div>`;
  }

  function tokenCheckHTML(runs, page) {
    const prices = loadPrices();
    const shownRuns = page && page.rows ? page.rows : runs;
    const cards = shownRuns.length
      ? shownRuns
          .map((run) => {
            const facts = [
              tokenFact("Latest trim (est.)", esc(fmt(run.latest_tokens_avoided_estimated))),
              tokenFact("Lifetime sum (est.)", esc(fmt(savedOf(run)))),
              tokenFact("Times trimmed", esc(fmt(run.projection_hits))),
              tokenFact("Raw chars", esc(fmt(run.raw_char_len))),
              tokenFact("Projected chars", esc(fmt(run.projected_char_len))),
              tokenFact("Provider input", esc(fmt(run.prompt_tokens))),
              tokenFact("Provider output", esc(fmt(run.completion_tokens))),
              tokenFact("Model", esc(fmtText(run.model))),
              tokenFact("List price used", esc(listPriceLabel(run.model, prices))),
              tokenFact("List-price illustration", illustrationHTML(run, prices)),
            ].join("");
            return `<article class="token-run">
              <h4><button type="button" class="share-name" data-id="${esc(run.trajectory_id)}">${esc(run.trajectory_id)}</button></h4>
              <div class="token-facts">${facts}</div>
            </article>`;
          })
          .join("")
      : `<p class="muted">No runs match that search.</p>`;
    return `<section class="token-check" id="token-check">
      <h3>Token check</h3>
      <p class="muted tiny">Every fact sits on the run card. none means that field was not sent. A zero is a real zero. The sidebar total adds Lifetime sum (est.) across runs.</p>
      <div class="token-runs">${cards}</div>
      ${pagerHTML("home-tokens", runs.length)}
      <p class="muted tiny">Formula: ceil of characters divided by 4, on the server. Not a provider invoice. The list-price illustration is separate. It is input and output, and it is not folded into the estimate.</p>
      <div class="read-guide">
        <div><h3>Latest trim</h3><p>The newest projection only. Re-trimming the same step does not change this cell.</p></div>
        <div><h3>Lifetime sum</h3><p>Every projection on that run, added. The home total uses this column.</p></div>
        <div><h3>Character counts</h3><p>Raw chars and projected chars from the newest projection. Check them against the estimates.</p></div>
        <div><h3>Provider counts</h3><p>Flat prompt_tokens, completion_tokens, and model on the event. A nested usage object is not read.</p></div>
      </div>
    </section>`;
  }

  function hexTone(saved, maxSaved, failed) {
    if (failed) return "bad";
    if (saved === null || saved === undefined || !Number.isFinite(saved)) return "unknown";
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
    const runPage = takePage("home-runs", runs);
    const sharePage = takePage("home-share", runs);
    const tokenPage = takePage("home-tokens", runs);
    const all = state.runs || [];
    const failed = all.reduce((n, r) => n + (r.seal_verified_fail || 0), 0);
    const files = all.reduce((n, r) => n + (r.exports_ok || 0) + (r.imports_ok || 0), 0);
    const events = all.reduce((n, r) => n + (r.event_count || 0), 0);
    const savedNums = runs.map(savedOf).filter((n) => n !== null);
    const maxSaved = savedNums.length ? Math.max.apply(null, savedNums) : 0;
    const board = hexRows(runs, maxSaved);
    const newest = runs[0] && runs[0].last_ts ? fmtTime(runs[0].last_ts) : "none";
    const sparkRuns = runs.slice().sort((a, b) => String(a.last_ts || "").localeCompare(String(b.last_ts || "")));
    const sparkVals = [];
    let sparkSkipped = 0;
    sparkRuns.forEach((r) => {
      const n = savedOf(r);
      if (n === null) {
        sparkSkipped += 1;
        return;
      }
      sparkVals.push(n);
    });
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
                    const savedLabel = savedOf(cell.run) === null ? "no trim yet" : `saved ${fmt(savedOf(cell.run))} (est.)`;
                    return `<button type="button" class="hex ${cell.tone}" data-id="${esc(id)}" data-saved="${esc(savedLabel)}" data-events="${esc(fmt(cell.run.event_count))}" data-failed="${esc(fmt(cell.run.seal_verified_fail))}" aria-label="${esc(id)}"></button>`;
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
            <span class="hex unknown"></span>
            <span>No trim yet</span>
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
                ${runPage.rows
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
            </table>
            ${pagerHTML("home-runs", runs.length)}`
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
                ? `<svg class="spark" viewBox="0 0 ${spark.w} ${spark.h}" aria-hidden="true"><polyline fill="none" stroke="currentColor" stroke-width="2.4" points="${spark.pts}"></polyline></svg>${sparkSkipped ? `<p class="muted tiny">${esc(sparkSkipped === 1 ? "1 run with no trim is left off this line." : String(sparkSkipped) + " runs with no trim are left off this line.")}</p>` : ""}`
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
                ? sharePage.rows
                    .map((run) => {
                      const n = savedOf(run);
                      const pct = n === null || !shareTotal ? 0 : barPct(n, shareTotal);
                      return `<div class="share-row">
                        <button type="button" class="share-name" data-id="${esc(run.trajectory_id)}">${esc(run.trajectory_id)}</button>
                        <div class="meter-bar saved"><span style="width:${pct}%"></span></div>
                        <span class="tiny">${esc(fmt(n))}</span>
                      </div>`;
                    })
                    .join("") + pagerHTML("home-share", runs.length)
                : `<p class="muted tiny">No runs yet.</p>`
            }
          </section>
        </aside>
      </div>
      ${tokenCheckHTML(runs, tokenPage)}
      <div class="help">
        <div><h3>What happened</h3><p>The story of one run, newest first, in plain words.</p></div>
        <div><h3>Locked decisions</h3><p>Seals freeze a plan before tools that change the world.</p></div>
        <div><h3>Files</h3><p>.tir packages stored on this PC. Open the folder. No MinIO.</p></div>
        <div><h3>Tokens</h3><p>Latest trim is one projection. Lifetime sum adds every trim. Guess from text size, not a bill.</p></div>
        <div><h3>Token check</h3><p>Character counts, provider counts, and a list-price illustration stay in separate columns.</p></div>
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
          <div>${esc(btn.getAttribute("data-saved"))}</div>
          <div>${esc(btn.getAttribute("data-events"))} events, locks failed ${esc(btn.getAttribute("data-failed"))}</div>`;
      });
      btn.addEventListener("mouseleave", () => {
        if (tip) tip.classList.add("hidden");
      });
    });
    bindJumps(home);
    bindPager(home);
    renderPriceList();
  }

  function showHome() {
    state.id = null;
    setQueryId("");
    $("crumb").textContent = "All runs";
    $("detail-title").textContent = "Operator home";
    $("detail-sub").textContent = "Pick a run to see locks, files, and tokens saved.";
    $("back-home").classList.add("hidden");
    $("home").classList.remove("hidden");
    const priceCard = $("price-card");
    if (priceCard) priceCard.classList.remove("hidden");
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
    const evPage = takePage("overview", ev);
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
            ? evPage.rows
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
      </ol>
      ${pagerHTML("overview", ev.length)}`;
    bindJumps($("panel-overview"));
    bindPager($("panel-overview"));
  }

  function renderSeals() {
    const seals = filteredEvents().filter(
      (e) => e.kind === "seal.created" || e.kind === "seal.verified"
    );
    const sealPage = takePage("seals", seals);
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
            ? sealPage.rows
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
      </ol>
      ${pagerHTML("seals", seals.length)}`;
    bindPager($("panel-seals"));
  }

  function renderTransfers() {
    const s = state.summary || {};
    const view = s.transfers || {};
    const handoffs = Array.isArray(view.handoffs) ? view.handoffs : [];
    const rows = handoffs.filter((h) => inRange(h.export_ts || h.import_ts));
    const packs = state.packages || [];
    const packPage = takePage("transfers-packs", packs);
    const rowPage = takePage("transfers-rows", rows);
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
          ? packPage.rows
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
              .join("") + pagerHTML("transfers-packs", packs.length)
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
          ${rowPage.rows
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
                <td class="${cls}">${esc(shown(statusWord))}</td>
                <td>${esc(shown(h.mode))} ${label}</td>
                <td><code>${esc(shown(path))}</code></td>
                <td>${esc(fmt(h.bytes))}</td>
                <td>${esc(fmt(h.member_count))}</td>
                <td>${esc(fmt(h.node_count))}</td>
                <td class="${bad}">${esc(shown(verifyText))}</td>
                <td>${esc(shown(who))}</td>
              </tr>`;
            })
            .join("") || `<tr><td colspan="8" class="muted">No transfer handoffs in range.</td></tr>`}
        </tbody>
      </table>
      ${pagerHTML("transfers-rows", rows.length)}`;
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
    bindPager($("panel-transfers"));
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
      "raw_char_len",
      "projected_char_len",
      "prompt_tokens",
      "completion_tokens",
      "model",
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
        row.raw_char_len,
        row.projected_char_len,
        row.prompt_tokens,
        row.completion_tokens,
        row.model,
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
    const stepPage = takePage("economy-steps", steps);
    const bigPage = takePage("economy-largest", largest);
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
    const repeatNote = economy.projection_hits > 1
      ? `Trimmed ${fmt(economy.projection_hits)} times. Saved so far adds every trim. Saved last time is only the newest trim.`
      : "";
    const charNote = `Latest raw characters ${fmt(economy.raw_char_len)}. Latest projected characters ${fmt(economy.projected_char_len)}. The estimates above are the server figures for those counts.`;
    const prices = loadPrices();
    const providerNote = `Provider input ${fmt(economy.prompt_tokens)}. Provider output ${fmt(economy.completion_tokens)}. Model ${fmtText(economy.model)}. A nested usage object is not read. List-price illustration: ${illustrationText(economy, prices)}. Edit the list price on All runs. It stays in this browser.`;
    $("panel-economy").innerHTML = `
      <p class="lede muted">We count how much context we dropped before calling the model. estimated_tokens uses ceil of chars divided by 4 on the server. This is not a provider invoice.</p>
      <p class="muted">${esc(charNote)}</p>
      <p class="muted">${esc(providerNote)}</p>
      ${repeatNote ? `<p class="muted">${esc(repeatNote)}</p>` : ""}
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
          : `<div class="token-scroll token-fit"><table class="token-table">
              <colgroup><col style="width:13%"><col style="width:18%"><col style="width:7%"><col style="width:8%"><col style="width:8%"><col style="width:8%"><col style="width:10%"><col style="width:12%"><col style="width:8%"><col style="width:8%"></colgroup>
              <thead><tr><th>Time</th><th>What</th><th>Step</th><th>Before</th><th>After</th><th>Saved</th><th>Raw chars</th><th>Projected chars</th><th>Dropped</th><th>Hidden</th></tr></thead>
              <tbody>${stepPage.rows
                .map((row) => {
                  const redaction =
                    row.kind === "redaction.applied"
                      ? `thoughts ${fmt(row.thought_collapses)}, fields ${fmt(row.secret_field_hits)}${row.mode ? ", " + row.mode : ""}`
                      : "none";
                  return `<tr>
                    <td>${esc(fmtTime(row.ts))}</td>
                    <td>${esc(kindLabel(row.kind))} <code>${kindCode(row.kind)}</code></td>
                    <td>${esc(fmt(row.step_n))}</td>
                    <td>${esc(fmt(row.raw_estimated_tokens))}</td>
                    <td>${esc(fmt(row.projected_estimated_tokens))}</td>
                    <td>${esc(fmt(row.tokens_avoided_estimated))}</td>
                    <td>${esc(fmt(row.raw_char_len))}</td>
                    <td>${esc(fmt(row.projected_char_len))}</td>
                    <td>${esc(fmt(row.dropped))}</td>
                    <td>${esc(redaction)}</td>
                  </tr>`;
                })
                .join("")}</tbody>
            </table></div>
            ${pagerHTML("economy-steps", steps.length)}
            <h3>Largest context payloads</h3>
            <div class="token-scroll token-fit"><table class="token-table">
              <thead><tr><th>Time</th><th>Step</th><th>Raw estimate</th><th>Raw chars</th><th>Size units</th><th>Dropped</th></tr></thead>
              <tbody>${
                bigPage.rows
                  .map(
                    (row) => `<tr>
                      <td>${esc(fmtTime(row.ts))}</td>
                      <td>${esc(fmt(row.step_n))}</td>
                      <td>${esc(fmt(row.raw_estimated_tokens))}</td>
                      <td>${esc(fmt(row.raw_char_len))}</td>
                      <td>${esc(fmt(row.size_units))}</td>
                      <td>${esc(fmt(row.dropped))}</td>
                    </tr>`
                  )
                  .join("") || `<tr><td colspan="6" class="muted">No projection rows in range.</td></tr>`
              }</tbody>
            </table></div>
            ${pagerHTML("economy-largest", largest.length)}
            <div class="read-guide">
              <div><h3>Before trim</h3><p>Server estimate of the raw character count on the newest projection.</p></div>
              <div><h3>After trim</h3><p>Server estimate of the projected character count on that same event.</p></div>
              <div><h3>Saved last time</h3><p>Newest projection only. This is not the sidebar total.</p></div>
              <div><h3>Saved so far</h3><p>Sum of every projection on this run. Trimming again counts again.</p></div>
            </div>`
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
    bindPager($("panel-economy"));
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
    if (state.id !== id) {
      resetPages(["overview", "seals", "transfers-packs", "transfers-rows", "economy-steps", "economy-largest"]);
    }
    state.id = id;
    setQueryId(id);
    $("crumb").textContent = "Run";
    $("detail-title").textContent = id;
    $("detail-sub").innerHTML = `Open again later with <code>?id=${esc(id)}</code>`;
    $("back-home").classList.remove("hidden");
    $("home").classList.add("hidden");
    const priceCard = $("price-card");
    if (priceCard) priceCard.classList.add("hidden");
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

  async function runLangGraphDemo() {
    const btn = $("run-langgraph-demo");
    showBanner("Running LangGraph demo into this console...");
    if (btn) btn.disabled = true;
    try {
      const res = await fetch("/v1/local/run-langgraph-demo", {
        method: "POST",
        headers: { ...headers(), "Content-Type": "application/json" },
        body: "{}",
      });
      const text = await res.text();
      let data = null;
      try {
        data = text ? JSON.parse(text) : null;
      } catch (_) {
        data = { error: text };
      }
      if (!res.ok) {
        const detail = (data && (data.error || data.output)) || res.statusText;
        throw new Error(detail);
      }
      showBanner("LangGraph demo OK - open run langgraph-demo");
      await loadList();
      if (data && data.trajectory_id) {
        selectTrajectory(data.trajectory_id);
      }
    } catch (err) {
      showBanner(String(err.message || err), true);
    } finally {
      if (btn) btn.disabled = false;
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
    $("run-langgraph-demo").addEventListener("click", () => runLangGraphDemo());
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
    const onRange = () => {
      resetPages(["overview", "seals", "transfers-packs", "transfers-rows", "economy-steps", "economy-largest"]);
      if (state.id) renderPanels();
    };
    $("from").addEventListener("change", onRange);
    $("to").addEventListener("change", onRange);
    $("back-home").addEventListener("click", showHome);
    $("savings").addEventListener("click", showHome);
    $("run-search").addEventListener("input", () => {
      state.query = $("run-search").value || "";
      resetPages(["sidebar", "home-runs", "home-share", "home-tokens"]);
      renderRunList();
      if (!state.id) renderHome();
    });
    const priceForm = $("price-form");
    if (priceForm) {
      priceForm.addEventListener("submit", (ev) => {
        ev.preventDefault();
        const entry = {
          model: priceForm.model.value.trim(),
          input_usd_per_million: priceForm.input_usd.value.trim(),
          output_usd_per_million: priceForm.output_usd.value.trim(),
          priced_on: priceForm.priced_on.value.trim(),
          source: priceForm.source.value.trim(),
        };
        if (!priceComplete(entry)) {
          showBanner("Price needs a model, both USD rates, a date, and a source. Nothing was saved.", true);
          return;
        }
        const all = loadPrices();
        all[entry.model] = entry;
        try {
          localStorage.setItem("trajir_console_prices", JSON.stringify(all));
        } catch (_) {
          showBanner("This browser blocked local storage. The price was not saved.", true);
          return;
        }
        priceForm.reset();
        showBanner("List price saved in this browser only.", false);
        renderPriceList();
        if (!state.id) renderHome();
      });
    }
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
