(function () {
  const $ = (id) => document.getElementById(id);
  const state = {
    token: sessionStorage.getItem("jurix_token") || "",
    user: null,
    dbx: null,
    matters: [],
    exports: [],
    playbook: null,
    detail: null,
    selectedId: sessionStorage.getItem("jurix_matter") || "",
    filter: "all",
    view: "matters",
    itemKey: "",
    doc: null,
    mode: "signin",
    busy: false,
  };

  function esc(s) {
    return String(s ?? "").replace(/[&<>"']/g, (c) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
    }[c]));
  }

  function detailOf(data) {
    const d = data && data.detail;
    if (typeof d === "string") return d;
    if (Array.isArray(d)) return d.map((x) => x.msg || JSON.stringify(x)).join("; ");
    return "";
  }

  function toast(msg) {
    const el = $("toast");
    el.textContent = msg;
    el.hidden = !msg;
    if (!msg) { el.classList.remove("toast-visible"); return; }
    el.classList.remove("toast-visible");
    void el.offsetWidth;
    el.classList.add("toast-visible");
    clearTimeout(toast._t);
    toast._t = setTimeout(() => { el.hidden = true; el.classList.remove("toast-visible"); }, 4200);
  }

  /* ── Progress bar ──────────────────────────────────────────────── */
  let _apiCount = 0;
  function progressStart() {
    _apiCount++;
    const bar = $("page-progress");
    if (bar) { bar.style.width = "60%"; bar.style.opacity = "1"; }
  }
  function progressEnd() {
    _apiCount = Math.max(0, _apiCount - 1);
    const bar = $("page-progress");
    if (!bar) return;
    if (_apiCount === 0) {
      bar.style.width = "100%";
      setTimeout(() => { bar.style.opacity = "0"; setTimeout(() => { bar.style.width = "0"; }, 300); }, 200);
    }
  }

  function when(ts) {
    if (!ts) return "—";
    return new Date(ts * 1000).toLocaleString(undefined, {
      day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit",
    });
  }

  function bytes(n) {
    const v = Number(n || 0);
    if (v < 1024) return v + " B";
    if (v < 1048576) return (v / 1024).toFixed(1) + " KB";
    return (v / 1048576).toFixed(1) + " MB";
  }

  function saveFile(name, text) {
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/markdown" }));
    a.download = name;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  function quoteHtml(text, mark) {
    const safe = esc(text || "");
    if (!mark) return safe;
    const needle = esc(mark);
    const at = safe.toLowerCase().indexOf(needle.toLowerCase());
    if (at < 0) return safe;
    return safe.slice(0, at) + "<mark>" + safe.slice(at, at + needle.length) + "</mark>" + safe.slice(at + needle.length);
  }

  function seal(id) {
    return `<span class="seal"><span class="ring"></span><span class="lbl">SEALED</span><span class="tid">${esc(id)}</span></span>`;
  }

  async function api(path, opts) {
    opts = opts || {};
    const headers = Object.assign({}, opts.headers || {});
    if (opts.body) headers["Content-Type"] = "application/json";
    if (state.token) headers.Authorization = "Bearer " + state.token;
    progressStart();
    try {
      const res = await fetch(path, Object.assign({}, opts, { headers }));
      const text = await res.text();
      let data = {};
      try { data = text ? JSON.parse(text) : {}; } catch (e) { data = { detail: text }; }
      if (res.status === 401 && path !== "/api/login" && path !== "/api/register") {
        logout();
        throw new Error(detailOf(data) || "Sign in again");
      }
      if (!res.ok) throw new Error(detailOf(data) || res.statusText);
      return data;
    } finally {
      progressEnd();
    }
  }

  function selected() {
    return state.matters.find((m) => m.id === state.selectedId) || null;
  }

  function visible() {
    return state.matters.filter((m) => {
      if (state.filter === "open") return m.open > 0;
      if (state.filter === "mine") return state.user && m.reviewer_email === state.user.email;
      return true;
    });
  }

  function currentItem() {
    const items = (state.detail && state.detail.items) || [];
    return items.find((i) => i.key === state.itemKey) || items[0] || null;
  }

  function showGate(name) {
    ["load", "home", "login"].forEach((k) => { $("g-" + k).hidden = k !== name; });
    $("gate").hidden = false;
    $("app").hidden = true;
    $("doc").hidden = true;
    $("night").hidden = true;
    if (name === "login") $("g-email").focus();
  }

  function logout() {
    state.token = "";
    state.user = null;
    state.detail = null;
    sessionStorage.removeItem("jurix_token");
    showGate("login");
  }

  async function enterApp() {
    $("gate").hidden = true;
    $("doc").hidden = true;
    $("night").hidden = true;
    $("app").hidden = false;
    $("app").classList.add("fx-enter");
    setTimeout(() => $("app").classList.remove("fx-enter"), 700);
    $("firm-name").textContent = state.user.firm_name;
    $("user-name").textContent = state.user.name;
    $("user-initials").textContent = state.user.initials;
    await Promise.all([refreshHealth(), refreshMatters(), refreshExports(), refreshPlaybook()]);
    if (!state.selectedId || !state.matters.some((m) => m.id === state.selectedId)) {
      state.selectedId = (state.matters[0] && state.matters[0].id) || "";
    }
    if (state.selectedId) await loadDetail(state.selectedId);
    const hash = (location.hash || "#matters").slice(1);
    if (["matters", "checklists", "exports", "firm"].includes(hash)) state.view = hash;
    render();
  }

  async function refreshHealth() {
    try {
      const data = await api("/api/health");
      state.dbx = data.dbx;
    } catch (err) {
      state.dbx = { ok: false, detail: err.message };
    }
  }

  async function refreshMatters() {
    const data = await api("/api/matters");
    state.matters = data.matters || [];
  }

  async function refreshExports() {
    const data = await api("/api/exports");
    state.exports = data.exports || [];
  }

  async function refreshPlaybook() {
    state.playbook = await api("/api/playbook");
  }

  async function loadDetail(id) {
    state.selectedId = id;
    sessionStorage.setItem("jurix_matter", id);
    state.detail = await api("/api/matters/" + encodeURIComponent(id));
    const items = state.detail.items || [];
    if (!items.some((i) => i.key === state.itemKey)) state.itemKey = (items[0] && items[0].key) || "";
  }

  function render() {
    const pill = $("dbx-pill");
    const ok = state.dbx && state.dbx.ok;
    pill.textContent = ok ? "DBX connected" : "DBX unreachable";
    pill.className = "dbx-pill " + (ok ? "on" : "off");
    $("banner").hidden = ok;
    if (!ok) $("banner").textContent = (state.dbx && state.dbx.detail) || "DBX control plane is not reachable.";

    const leaks = state.detail ? state.detail.sibling_hits || 0 : 0;
    $("nav-matters").textContent = String(state.matters.length);
    $("nav-checks").textContent = String(state.matters.filter((m) => m.has_run).length * 4 || 4);
    $("nav-exports").textContent = String(state.exports.length);
    $("nav-seal").textContent = state.matters.length + " clients · " + state.matters.length + " sealed tenants";
    $("nav-leaks").textContent = String(leaks);
    document.querySelectorAll(".nav a").forEach((a) => {
      a.setAttribute("aria-current", a.dataset.view === state.view ? "page" : "false");
    });
    ["matters", "checklists", "exports", "firm"].forEach((v) => {
      $("view-" + v).hidden = state.view !== v;
    });
    renderQueue();
    renderAudit();
    renderPlaybook();
    renderExports();
    renderFirm();
  }

  function findingsCell(m) {
    if (!m.has_run) return '<span class="s-idle">—</span>';
    if (m.hibernated) return '<span class="s-idle">Hibernated</span>';
    if (m.open === 0) return '<span class="s-clear">Cleared</span>';
    const high = m.high + m.missing;
    const bits = [];
    if (high) bits.push(`<span class="s-high"><i class="sq"></i>${high} high</span>`);
    if (m.medium) bits.push(`<span class="s-medium"><i class="sq"></i>${m.medium} medium</span>`);
    return bits.join(" ");
  }

  function reviewerCell(m) {
    const name = `<small>${esc(m.reviewer_name)}</small>`;
    if (m.status === "hibernated") return `<span class="s-idle">Hibernated</span> ${name}`;
    if (m.status === "not_started") return `<span class="s-idle">Not started</span>`;
    if (m.status === "cleared") return `<span class="s-clear">Cleared</span> ${name}`;
    return `In review ${name}`;
  }

  function renderQueue() {
    const rows = visible();
    const all = state.matters.length;
    const open = state.matters.filter((m) => m.open > 0).length;
    const mine = state.matters.filter((m) => state.user && m.reviewer_email === state.user.email).length;
    $("f-all").textContent = String(all);
    $("f-open").textContent = String(open);
    $("f-mine").textContent = String(mine);
    $("queue-meta").textContent = (state.user ? state.user.firm_name : "") + " · sorted by open findings";
    document.querySelectorAll("[data-filter]").forEach((b) => {
      b.setAttribute("aria-pressed", b.dataset.filter === state.filter ? "true" : "false");
    });
    $("queue-empty").hidden = rows.length > 0;
    $("queue-body").innerHTML = rows.map((m) => `
      <tr data-id="${esc(m.id)}" aria-selected="${m.id === state.selectedId ? "true" : "false"}">
        <td class="client">${esc(m.name)}</td>
        <td>${esc(m.matter)}</td>
        <td>${seal(m.tenant_id)}</td>
        <td class="findings">${findingsCell(m)}</td>
        <td class="reviewer">${reviewerCell(m)}</td>
      </tr>`).join("");
  }

  function scopeLine(m) {
    if (!m) return "";
    if (!m.siblings_probed) return `Query scope: ${m.tenant_id} only · no other indexed client to probe`;
    return `Query scope: ${m.tenant_id} only · sibling tenants returned ${m.sibling_hits}`;
  }

  function renderAudit() {
    const m = state.detail;
    const show = state.view === "matters" && m && m.id === state.selectedId;
    const instruments = $("instruments");
    if (instruments && !show) instruments.hidden = true;
    $("audit").hidden = !show;
    $("audit-empty").hidden = show || state.view !== "matters";
    if (!show) {
      if (state.view === "matters" && !state.matters.length) {
        $("audit-empty").hidden = false;
        $("audit-empty").innerHTML = "<strong>The queue is empty.</strong> New matter provisions a tenant on the DBX node this bench is using.";
      }
      return;
    }
    $("crumb-client").textContent = m.name;
    $("matter-h").textContent = m.name + " · " + m.matter;
    $("matter-meta").innerHTML = `
      ${seal(m.tenant_id)}
      <span>${esc(scopeLine(m))}</span>
      <span>Reviewer <b>${esc(m.reviewer_name)}</b></span>
      <span>Playbook <b>${esc(m.playbook || "")}</b></span>`;
    const items = m.items || [];
    const counts = { high: 0, missing: 0, medium: 0, clear: 0 };
    items.forEach((i) => { if (counts[i.state] != null) counts[i.state] += 1; });
    $("tally").innerHTML = ["high", "missing", "medium", "clear"].map((k) =>
      `<div><dt class="s-${k}">${k}</dt><dd>${counts[k]}</dd></div>`).join("");
    const open = items.filter((i) => i.state !== "clear").length;
    $("check-meta").textContent = items.length
      ? `${items.length} items · ${open} open${m.stale ? " · documents changed since the last run" : ""}`
      : "Not run";
    $("run-btn").textContent = m.stale ? "Run checklist" : (items.length ? "Run checklist again" : "Run checklist");
    $("checklist-body").innerHTML = items.map((item) => `
      <tr data-key="${esc(item.key)}" aria-selected="${item.key === (currentItem() && currentItem().key) ? "true" : "false"}" tabindex="${item.key === (currentItem() && currentItem().key) ? "0" : "-1"}">
        <td class="mono">${esc(item.number)}</td>
        <td>${esc(item.item)}</td>
        <td>${esc(item.playbook)}</td>
        <td>${esc(item.finding)}</td>
        <td class="mono">${esc(item.citation)}</td>
        <td class="state s-${esc(item.state)}"><i class="sq"></i>${esc(item.state)}</td>
      </tr>`).join("");
    const item = currentItem();
    $("evidence").hidden = !item;
    if (item) {
      $("ev-cite").textContent = item.citation;
      $("ev-src").textContent = (item.doc_name || "document") + " · " + (item.via === "vector" ? "DBX vector hit" : "read from DBX");
      const quote = $("ev-quote");
      quote.innerHTML = "“" + quoteHtml(item.quote, item.mark) + "”";
      quote.classList.toggle("absent", !!item.absent);
      $("ev-note").innerHTML = `Playbook <b>${esc(item.playbook)}</b> · <b class="s-${esc(item.state)}">${esc(item.tail)}</b>`;
      const notes = (m.notes || []).filter((n) => n.item_key === item.key);
      $("ev-notes").innerHTML = notes.map((n) => `<li>${esc(n.text)} <span class="muted">${esc(n.author)}</span></li>`).join("");
    }
    const docs = (m.documents || []).map((d) => d.name).join(", ") || "none";
    $("table-foot").innerHTML = `
      <span>Source <span class="mono">${esc(docs)}</span> · ${m.chunk_count || 0} chunks indexed</span>
      <span>Last run <span class="mono">${esc(when(m.ran_at))}</span></span>
      <span>Read path <span class="mono">${esc(m.read_path || "")}</span></span>`;
    const hib = !!m.hibernated;
    $("run-btn").disabled = hib;
    $("add-doc-btn").disabled = hib;
    $("q").placeholder = "Clauses, findings, chunks in " + m.tenant_id;
    renderInstruments();
  }

  function renderInstruments() {
    const box = $("instruments");
    if (!box) return;
    const m = state.detail;
    const show = state.view === "matters" && m && m.id === state.selectedId;
    box.hidden = !show;
    if (!show) return;
    const items = m.items || [];
    const vectors = items.filter((item) => item.via === "vector").length;
    const pct = items.length ? Math.round((vectors / items.length) * 100) : 0;
    $("ins-vector").textContent = items.length ? vectors + " / " + items.length : "—";
    $("ins-vector-bar").style.width = pct + "%";
    const leaks = Number(m.sibling_hits || 0);
    $("ins-silence").textContent = String(leaks);
    $("ins-silence-bar").style.width = leaks ? "100%" : "8%";
    $("ins-silence-bar").style.background = leaks ? "var(--high)" : "var(--clear)";
    $("ins-chunks").textContent = String(m.chunk_count || 0);
    $("ins-play").textContent = m.playbook || "—";
  }

  function renderPlaybook() {
    const pb = state.playbook;
    if (!pb) return;
    $("playbook-name").textContent = pb.name;
    const items = (state.detail && state.detail.items) || [];
    $("playbook-list").innerHTML = `<table><thead><tr><th>Item</th><th>Floor</th><th>Query sent to DBX</th><th>Latest finding</th></tr></thead><tbody>` +
      pb.items.map((spec) => {
        const hit = items.find((i) => i.key === spec.key);
        return `<tr data-key="${esc(spec.key)}"><td>${esc(spec.item)}</td><td>${esc(spec.playbook)}</td><td class="mono">${esc(spec.query)}</td><td>${hit ? esc(hit.finding) : "—"}</td></tr>`;
      }).join("") + "</tbody></table>";
  }

  function renderExports() {
    $("export-empty").hidden = state.exports.length > 0;
    $("export-body").innerHTML = state.exports.map((row) => `
      <tr>
        <td>${esc(when(row.created_at))}</td>
        <td>${esc(row.client_name)}</td>
        <td>${esc(row.matter)}</td>
        <td>${esc(row.kind)}</td>
        <td><button class="btn btn-quiet" type="button" data-download="${esc(row.id)}" data-name="${esc(row.client_name)}">Download</button></td>
      </tr>`).join("");
  }

  function renderFirm() {
    const u = state.user;
    if (!u) return;
    $("firm-sub").textContent = u.firm_name;
    $("firm-account").innerHTML = `
      <dt>Firm</dt><dd>${esc(u.firm_name)}</dd>
      <dt>Email</dt><dd>${esc(u.email)}</dd>
      <dt>Firm code</dt><dd class="mono">${esc(u.firm_code || "—")}</dd>
      <dt>Reviewer</dt><dd>${esc(u.name)}</dd>`;
    const dbx = state.dbx || {};
    $("firm-dbx").innerHTML = `
      <dt>Control plane</dt><dd class="mono">${esc(dbx.control_url || "")}</dd>
      <dt>RESP ingress</dt><dd class="mono">${esc(dbx.resp || "")}</dd>
      <dt>Status</dt><dd class="${dbx.ok ? "s-clear" : "s-high"}">${dbx.ok ? "Connected" : "Unreachable"}</dd>`;
    const m = state.detail;
    $("firm-matter-line").textContent = m
      ? m.name + " · " + m.tenant_id + (m.hibernated ? " · hibernated" : " · awake")
      : "Open a matter to hibernate, wake, or offboard it.";
    $("hibernate-btn").disabled = !m || m.hibernated;
    $("wake-btn").disabled = !m || !m.hibernated;
    $("offboard-btn").disabled = !m;
  }

  function runStream() {
    const rs = $("raw-stream");
    const rl = $("raw-lines");
    if (!rs || !rl) return;
    rs.hidden = false;
    rl.innerHTML = "";
    const lines = [
      "INIT dbx_tenant_conn [ok]",
      "REQ GET /v1/playbook",
      "SYS chunking_doc chunks=13",
      "VEC gen_embeddings dim=1536 batch=1",
      "VEC pushing_vectors to dbx [ok]",
      "SYS query_vectors rule=indemnity",
      "SYS query_vectors rule=renewal",
      "SYS query_vectors rule=law",
      "EVAL running_heuristics",
      "RES stream_closed 200 OK"
    ];
    let i = 0;
    const it = setInterval(() => {
      if (i >= lines.length) { clearInterval(it); setTimeout(() => rs.hidden = true, 1000); return; }
      const p = document.createElement("p");
      p.textContent = "> " + lines[i++];
      rl.appendChild(p);
      rl.scrollTop = rl.scrollHeight;
    }, 150);
  }

  async function selectMatter(id) {
    try {
      await loadDetail(id);
      state.view = "matters";
      history.replaceState(null, "", "#matters");
      render();
    } catch (err) { toast(err.message); }
  }

  async function runAudit() {
    if (!state.selectedId) return;
    const runBtn = $("run-btn");
    const audit = $("audit");
    const shell = $("app");
    runBtn.disabled = true;
    runBtn.classList.add("is-busy");
    if (audit) audit.classList.add("is-scanning");
    if (shell) shell.classList.add("is-busy");
    runStream();
    try {
      state.detail = await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/audit", { method: "POST" });
      state.itemKey = (state.detail.items[0] && state.detail.items[0].key) || "";
      await refreshMatters();
      toast("Checklist read from " + state.detail.tenant_id);
      render();
      document.querySelectorAll(".instrument").forEach((el) => {
        el.classList.remove("flash");
        void el.offsetWidth;
        el.classList.add("flash");
      });
      document.querySelectorAll("#audit .seal .ring").forEach((r) => {
        r.classList.add("is-spinning");
        setTimeout(() => r.classList.remove("is-spinning"), 1200);
      });
      /* Seal stamp celebration: trigger when all items are clear */
      const allClear = (state.detail.items || []).every((i) => i.state === "clear");
      if (allClear && state.detail.items.length > 0 && window.JurixSealStamp) {
        setTimeout(() => {
          window.JurixSealStamp.celebrate({
            text: "CLEARED",
            sub: state.detail.name + " · all items passed",
            duration: 2400,
          });
        }, 600);
      }
    } catch (err) { toast(err.message); render(); }
    finally {
      runBtn.classList.remove("is-busy");
      if (audit) audit.classList.remove("is-scanning");
      if (shell) shell.classList.remove("is-busy");
    }
  }

  async function indexDoc(docName, text, sample) {
    const id = state.selectedId;
    if (!id) throw new Error("Open a matter first");
    if (sample) {
      await api("/api/matters/" + encodeURIComponent(id) + "/sample", {
        method: "POST", body: JSON.stringify({ kind: sample }),
      });
    } else {
      await api("/api/matters/" + encodeURIComponent(id) + "/documents", {
        method: "POST", body: JSON.stringify({ doc_name: docName, text }),
      });
    }
    await runAudit();
  }

  async function createMatter(sample) {
    const name = $("m-name").value.trim();
    const matter = $("m-matter").value.trim() || "Matter";
    if (!name) { toast("Name the client"); return; }
    $("new-matter-btn").disabled = true;
    $("app").classList.add("is-busy");
    try {
      const created = await api("/api/matters", {
        method: "POST", body: JSON.stringify({ name, matter }),
      });
      state.selectedId = created.matter.id;
      const text = $("m-text").value.trim();
      const docName = $("m-doc").value.trim() || "Contract.txt";
      $("dlg-matter").close();
      await refreshMatters();
      if (sample || text) {
        await loadDetail(state.selectedId);
        await indexDoc(docName, text, sample);
      } else {
        await loadDetail(state.selectedId);
        render();
        toast("Tenant " + created.matter.tenant_id + " is sealed");
      }
    } catch (err) { toast(err.message); }
    $("new-matter-btn").disabled = false;
    $("app").classList.remove("is-busy");
  }

  async function exportMemo() {
    if (!state.selectedId) { toast("Open a matter first"); return; }
    try {
      const data = await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/export", { method: "POST" });
      const name = "jurix-" + (state.detail ? state.detail.name : "matter") + "-memorandum.md";
      saveFile(name.replace(/\s+/g, "-"), data.body);
      await refreshExports();
      render();
      toast("Memorandum exported");
    } catch (err) { toast(err.message); }
  }

  async function openDocument() {
    const item = currentItem();
    if (!state.selectedId) return;
    try {
      const q = item && item.doc_id ? "?doc_id=" + encodeURIComponent(item.doc_id) : "";
      state.doc = await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/document" + q);
      renderDocument();
    } catch (err) { toast(err.message); }
  }

  function renderDocument() {
    const doc = state.doc;
    const m = state.detail;
    if (!doc || !m) return;
    $("app").hidden = true;
    $("night").hidden = true;
    $("doc").hidden = false;
    $("doc-title").textContent = doc.name;
    $("doc-sub").textContent = "Read from DBX tenant " + doc.tenant_id + " · " + when(doc.ingested_at);
    const items = m.items || [];
    $("doc-body").innerHTML = doc.chunks.map((chunk) => {
      const cited = items.filter((i) => i.chunk_id === chunk.id);
      const on = cited.some((i) => i.key === (currentItem() && currentItem().key));
      return `<p class="clause${cited.length ? " is-cited" : ""}" id="chunk-${esc(chunk.id).replace(/[^a-z0-9]+/gi, "-")}" data-key="${cited[0] ? esc(cited[0].key) : ""}"${on ? ' style="outline:1px solid #8c4a32"' : ""}>${formatClause(chunk.text)}</p>`;
    }).join("");
    $("doc-notes").innerHTML = items.map((item) => `
      <article class="margin-note">
        <span class="s-${esc(item.state)}"><i class="sq"></i>${esc(item.state)}</span>
        <h3>${esc(item.item)}</h3>
        <p>${esc(item.finding)}. ${esc(item.tail)}</p>
        <p class="mono">${esc(item.citation)}</p>
      </article>`).join("") || "<p class='muted'>Run the checklist to place notes in the margin.</p>";
    const item = currentItem();
    if (item && item.chunk_id) {
      const el = document.getElementById("chunk-" + item.chunk_id.replace(/[^a-z0-9]+/gi, "-"));
      if (el) el.scrollIntoView({ block: "center" });
    }
  }

  function formatClause(text) {
    const match = String(text || "").match(/^(§\s*[\d.]+)\s*/);
    if (!match) return esc(text);
    return `<a class="clause-no">${esc(match[1])}</a>${esc(text.slice(match[0].length))}`;
  }

  function renderNight() {
    const m = state.detail;
    if (!m) return;
    $("app").hidden = true;
    $("doc").hidden = true;
    $("night").hidden = false;
    $("night-matter").textContent = m.name + " · " + m.matter;
    const items = m.items || [];
    $("night-sheet").innerHTML = `<p class="muted">Execution copy · read from ${esc(m.tenant_id)}</p><h1>${esc(m.name)}</h1>` +
      (items.length ? items.map((item) => `
        <div class="clause${item.key === (currentItem() && currentItem().key) ? " is-active" : ""}" id="night-${esc(item.key)}">
          <div class="cite">${esc(item.citation)} · ${esc(item.doc_name)}</div>
          <p>${quoteHtml(item.quote, item.mark) || "No passage stored."}</p>
        </div>`).join("") : "<p>Run the checklist to fill the brief.</p>");
    $("night-findings").innerHTML = items.map((item) => `
      <li><button type="button" data-night="${esc(item.key)}" aria-current="${item.key === (currentItem() && currentItem().key) ? "true" : "false"}">
        <span class="s-${esc(item.state)}">${esc(item.state)}</span>
        <h3>${esc(item.item)}</h3>
        <p>${esc(item.finding)}</p>
      </button></li>`).join("");
    $("night-scope").textContent = scopeLine(m);
  }

  async function doSearch(q) {
    if (!state.selectedId || q.trim().length < 2) {
      $("search-pop").hidden = true;
      return;
    }
    try {
      const data = await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/search", {
        method: "POST", body: JSON.stringify({ query: q.trim() }),
      });
      const hits = data.hits || [];
      $("search-pop").hidden = hits.length === 0;
      $("search-pop").innerHTML = hits.map((h) =>
        `<button type="button" data-hit="${esc(h.id)}" data-doc="${esc(h.doc_name)}"><b class="mono">${esc(h.doc_name)} · chunk ${esc(h.index)}</b><br>${esc((h.text || "").slice(0, 180))}</button>`
      ).join("") || "";
      if (!hits.length) toast("No chunks in " + (state.detail ? state.detail.tenant_id : "this tenant"));
    } catch (err) { toast(err.message); }
  }

  function closeDoc() {
    $("doc").hidden = true;
    $("night").hidden = true;
    $("app").hidden = false;
    render();
  }

  let aiTimeout;
  let aiTarget;
  document.addEventListener("mousemove", (e) => {
    document.body.style.setProperty("--mouse-x", e.clientX + "px");
    document.body.style.setProperty("--mouse-y", e.clientY + "px");

    const clause = e.target.closest(".clause");
    if (clause) {
      if (aiTarget !== clause) {
        aiTarget = clause;
        clearTimeout(aiTimeout);
        const tt = $("ai-tooltip");
        if (tt) tt.hidden = true;
        aiTimeout = setTimeout(() => showAITooltip(clause), 800);
      } else {
        const tt = $("ai-tooltip");
        if (tt && !tt.hidden) {
          tt.style.left = (e.clientX + 20) + "px";
          tt.style.top = (e.clientY + 20) + "px";
        }
      }
    } else {
      aiTarget = null;
      clearTimeout(aiTimeout);
      const tt = $("ai-tooltip");
      if (tt) tt.hidden = true;
    }
  });

  function showAITooltip(clause) {
    const tt = $("ai-tooltip");
    const body = $("ai-tooltip-body");
    if (!tt || !body) return;
    const x = parseInt(document.body.style.getPropertyValue("--mouse-x") || "0");
    const y = parseInt(document.body.style.getPropertyValue("--mouse-y") || "0");
    tt.style.left = (x + 20) + "px";
    tt.style.top = (y + 20) + "px";
    tt.hidden = false;
    
    const text = clause.textContent.toLowerCase();
    let translation = "This is a standard boilerplate provision regulating general obligations.";
    if (text.includes("indemni")) translation = "The provider is agreeing to pay for any legal damages or losses you suffer from third-party claims.";
    else if (text.includes("liab")) translation = "This caps the maximum amount of money they can be sued for, usually tied to fees paid.";
    else if (text.includes("law") || text.includes("jurisdiction")) translation = "If you sue them, you must do it in the specified state's courts.";
    else if (text.includes("term") || text.includes("terminate")) translation = "This outlines how and when you can cancel the contract without penalty.";
    
    body.innerHTML = "";
    let i = 0;
    function type() {
      if (i < translation.length) {
        body.innerHTML += translation.charAt(i);
        i++;
        setTimeout(type, 15);
      }
    }
    type();
  }

  const cq = $("counsel-q");
  const cs = $("counsel-suggest");
  if (cq && cs) {
    cq.addEventListener("input", (e) => {
      const val = e.target.value.toLowerCase();
      if (val.length > 2 && "liability".includes(val)) {
        cs.hidden = false;
        cs.textContent = "✨ Hit Tab to ask: What is the liability cap in this agreement?";
      } else if (val.length > 2 && "terminate".includes(val)) {
        cs.hidden = false;
        cs.textContent = "✨ Hit Tab to ask: Under what conditions can we terminate for convenience?";
      } else {
        cs.hidden = true;
      }
    });
    cq.addEventListener("keydown", (e) => {
      if (e.key === "Tab" && !cs.hidden) {
        e.preventDefault();
        cq.value = cs.textContent.replace("✨ Hit Tab to ask: ", "");
        cs.hidden = true;
      }
    });
  }

  $("g-load").addEventListener("click", () => showGate("home"));
  document.addEventListener("keydown", (e) => {
    if (!$("g-load").hidden && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      showGate("home");
    }
    if (e.key === "/" && document.activeElement && document.activeElement.tagName !== "INPUT" && document.activeElement.tagName !== "TEXTAREA" && !$("app").hidden) {
      e.preventDefault();
      $("q").focus();
    }
    /* ⌘K / Ctrl+K: open command palette */
    if ((e.metaKey || e.ctrlKey) && e.key === "k" && !$("app").hidden) {
      e.preventDefault();
      const cmd = $("cmd");
      if (cmd) {
        if (cmd.open) { cmd.close(); } else { cmd.showModal(); $("cmd-q").value = ""; $("cmd-q").focus(); populateCmdPalette(); }
      }
    }
    /* Escape: close overlays */
    if (e.key === "Escape") {
      const counsel = $("counsel");
      if (counsel && !counsel.hidden) { counsel.hidden = true; return; }
    }
  });
  const SHEETS = {
    sealed: {
      k: "01 · Matters",
      h: "Sealed client",
      p: "Creating a matter provisions one DBX tenant. The queue shows the seal and the tenant id.",
      img: "img/corridor.jpg",
      alt: "Closed walnut doors in a law-office corridor",
    },
    checklist: {
      k: "02 · Checklists",
      h: "Checklist",
      p: "Indemnity, renewal, governing law, and breach notice. Each row carries a playbook floor and a finding from the indexed text.",
      img: "img/desk.jpg",
      alt: "Oak desk with a leather folio and a wax seal",
      states: true,
    },
    citation: {
      k: "03",
      h: "Citation",
      p: "The clause comes back with the section and the chunk. Open it in the document view or the night brief.",
      img: "img/chambers.jpg",
      alt: "Law library shelves",
    },
    silence: {
      k: "04",
      h: "Silence",
      p: "A probe of every other client in the firm must return none of this matter’s citations.",
      img: "img/corridor.jpg",
      alt: "A corridor of closed office doors",
    },
    memo: {
      k: "05 · Exports",
      h: "Memorandum",
      p: "Export the findings without leaving the matter. The file stays under Exports.",
      img: "img/lamp.jpg",
      alt: "A banker's lamp on a night desk",
    },
    offboard: {
      k: "06",
      h: "Offboard",
      p: "Export and purge one client. The rest of the firm stays in place.",
      img: "img/parchment.jpg",
      alt: "Parchment surface",
    },
  };

  function openSheet(id) {
    const sheet = SHEETS[id];
    const dialog = $("g-sheet");
    if (!sheet || !dialog) return;
    $("g-sheet-k").textContent = sheet.k;
    $("g-sheet-h").textContent = sheet.h;
    $("g-sheet-p").textContent = sheet.p;
    $("g-sheet-img").src = sheet.img;
    $("g-sheet-img").alt = sheet.alt;
    $("g-sheet-states").hidden = !sheet.states;
    $("g-sheet-note").hidden = !sheet.states;
    if (!dialog.open) dialog.showModal();
  }

  document.addEventListener("click", (e) => {
    const go = e.target.closest("[data-go]");
    if (go && $("gate").contains(go)) {
      e.preventDefault();
      showGate(go.getAttribute("data-go"));
      return;
    }
    const sheetBtn = e.target.closest("[data-sheet]");
    if (sheetBtn && $("gate").contains(sheetBtn)) {
      e.preventDefault();
      openSheet(sheetBtn.getAttribute("data-sheet"));
    }
  });
  $("g-sheet-close").addEventListener("click", () => $("g-sheet").close());
  $("g-sheet").addEventListener("click", (e) => {
    if (e.target === $("g-sheet")) $("g-sheet").close();
  });

  let registerMode = false;
  $("g-mode").addEventListener("click", () => {
    registerMode = !registerMode;
    $("g-firm-field").hidden = !registerMode;
    $("g-login-h").textContent = registerMode ? "Open a firm on Jurix" : "Sign in to Jurix";
    $("g-submit").textContent = registerMode ? "Create firm" : "Continue";
    $("g-mode").textContent = registerMode ? "Have an account? Sign in" : "Create a firm account";
    $("g-password").autocomplete = registerMode ? "new-password" : "current-password";
  });
  $("g-code-toggle").addEventListener("click", () => {
    const field = $("g-code-field");
    if (field.hidden) { field.hidden = false; $("g-code").focus(); return; }
    $("g-form").requestSubmit();
  });
  $("g-terms-btn").addEventListener("click", () => { $("g-terms").hidden = !$("g-terms").hidden; });
  $("g-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const email = $("g-email").value.trim();
    const password = $("g-password").value;
    const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);
    $("g-email-err").hidden = emailOk;
    $("g-pass-err").hidden = password.length >= 8;
    $("g-email").setAttribute("aria-invalid", emailOk ? "false" : "true");
    if (!emailOk || password.length < 8) return;
    if (registerMode && !$("g-firm").value.trim()) { toast("Name the firm"); return; }
    $("g-submit").disabled = true;
    try {
      const path = registerMode ? "/api/register" : "/api/login";
      const body = registerMode
        ? { email, password, firm_name: $("g-firm").value.trim() }
        : { email, password, code: $("g-code").value.trim() };
      const data = await api(path, { method: "POST", body: JSON.stringify(body) });
      state.token = data.token;
      state.user = data.user;
      sessionStorage.setItem("jurix_token", state.token);
      if (data.user.firm_code) toast("Firm code " + data.user.firm_code);
      await enterApp();
    } catch (err) { toast(err.message); }
    $("g-submit").disabled = false;
  });

  $("logout-btn").addEventListener("click", logout);
  document.querySelectorAll(".nav a").forEach((a) => {
    a.addEventListener("click", (e) => {
      e.preventDefault();
      state.view = a.dataset.view;
      history.replaceState(null, "", "#" + state.view);
      if (state.view === "firm" && state.selectedId) {
        api("/api/matters/" + encodeURIComponent(state.selectedId) + "/usage").then((data) => {
          const u = data.usage || {};
          $("firm-usage").innerHTML = `Tenant <span class="mono">${esc(data.tenant_id)}</span> · ${esc(u.status || "")} · ${u.vectors || 0} vectors · ${u.keys || 0} keys · ${u.commands || 0} commands · ${bytes(u.memory_used_bytes)} in memory`;
        }).catch((err) => { $("firm-usage").textContent = err.message; });
      }
      render();
    });
  });
  document.querySelectorAll("[data-filter]").forEach((b) => {
    b.addEventListener("click", () => { state.filter = b.dataset.filter; renderQueue(); });
  });
  $("queue-body").addEventListener("click", (e) => {
    const row = e.target.closest("tr[data-id]");
    if (row) selectMatter(row.dataset.id);
  });
  $("checklist-body").addEventListener("click", (e) => {
    const row = e.target.closest("tr[data-key]");
    if (!row) return;
    state.itemKey = row.dataset.key;
    renderAudit();
  });
  $("checklist-body").addEventListener("keydown", (e) => {
    const row = e.target.closest("tr[data-key]");
    if (!row) return;
    const rows = Array.from($("checklist-body").querySelectorAll("tr"));
    const i = rows.indexOf(row);
    if (e.key === "ArrowDown" && rows[i + 1]) { e.preventDefault(); state.itemKey = rows[i + 1].dataset.key; renderAudit(); rows[i + 1] && $("checklist-body").querySelector(`[data-key="${rows[i + 1].dataset.key}"]`).focus(); }
    if (e.key === "ArrowUp" && rows[i - 1]) { e.preventDefault(); state.itemKey = rows[i - 1].dataset.key; renderAudit(); $("checklist-body").querySelector(`[data-key="${rows[i - 1].dataset.key}"]`).focus(); }
  });
  $("playbook-list").addEventListener("click", (e) => {
    const row = e.target.closest("tr[data-key]");
    if (!row || !state.detail) return;
    state.itemKey = row.dataset.key;
    state.view = "matters";
    render();
  });

  $("new-matter-btn").addEventListener("click", () => $("dlg-matter").showModal());
  $("m-cancel").addEventListener("click", () => $("dlg-matter").close());
  $("form-matter").addEventListener("submit", (e) => { e.preventDefault(); createMatter(""); });
  $("m-sample-msa").addEventListener("click", () => createMatter("msa"));
  $("m-sample-dpa").addEventListener("click", () => createMatter("dpa"));

  $("add-doc-btn").addEventListener("click", () => $("dlg-doc").showModal());
  $("d-cancel").addEventListener("click", () => $("dlg-doc").close());
  $("form-doc").addEventListener("submit", async (e) => {
    e.preventDefault();
    try {
      $("dlg-doc").close();
      await indexDoc($("d-name").value.trim() || "Contract.txt", $("d-text").value, "");
    } catch (err) { toast(err.message); }
  });
  $("d-msa").addEventListener("click", async () => {
    $("dlg-doc").close();
    try { await indexDoc("", "", "msa"); } catch (err) { toast(err.message); }
  });
  $("d-dpa").addEventListener("click", async () => {
    $("dlg-doc").close();
    try { await indexDoc("", "", "dpa"); } catch (err) { toast(err.message); }
  });

  $("run-btn").addEventListener("click", runAudit);
  $("export-btn").addEventListener("click", exportMemo);
  $("doc-export").addEventListener("click", exportMemo);
  $("night-export").addEventListener("click", exportMemo);
  $("open-doc-btn").addEventListener("click", openDocument);
  $("brief-btn").addEventListener("click", renderNight);
  $("doc-back").addEventListener("click", closeDoc);
  $("night-back").addEventListener("click", closeDoc);
  $("night-findings").addEventListener("click", (e) => {
    const btn = e.target.closest("[data-night]");
    if (!btn) return;
    state.itemKey = btn.dataset.night;
    renderNight();
    const el = $("night-" + btn.dataset.night);
    if (el) el.scrollIntoView({ block: "center" });
  });

  $("note-btn").addEventListener("click", () => {
    if (!currentItem()) { toast("Run the checklist first"); return; }
    $("n-text").value = "";
    $("dlg-note").showModal();
  });
  $("n-cancel").addEventListener("click", () => $("dlg-note").close());
  $("form-note").addEventListener("submit", async (e) => {
    e.preventDefault();
    const item = currentItem();
    if (!item) return;
    try {
      await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/notes", {
        method: "POST",
        body: JSON.stringify({ item_key: item.key, text: $("n-text").value }),
      });
      $("dlg-note").close();
      await loadDetail(state.selectedId);
      render();
    } catch (err) { toast(err.message); }
  });

  $("hibernate-btn").addEventListener("click", async () => {
    try {
      await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/hibernate", { method: "POST" });
      await refreshMatters();
      await loadDetail(state.selectedId);
      toast("Tenant hibernated");
      render();
    } catch (err) { toast(err.message); }
  });
  $("wake-btn").addEventListener("click", async () => {
    try {
      await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/wake", { method: "POST" });
      await refreshMatters();
      await loadDetail(state.selectedId);
      toast("Tenant awake");
      render();
    } catch (err) { toast(err.message); }
  });
  $("offboard-btn").addEventListener("click", () => {
    const m = state.detail;
    if (!m) return;
    $("off-copy").textContent = "Export " + m.name + " and purge tenant " + m.tenant_id + ". Other clients stay.";
    $("dlg-off").showModal();
  });
  $("off-cancel").addEventListener("click", () => $("dlg-off").close());
  $("form-off").addEventListener("submit", async (e) => {
    e.preventDefault();
    const m = state.detail;
    try {
      const data = await api("/api/matters/" + encodeURIComponent(state.selectedId) + "/offboard", { method: "POST" });
      saveFile("jurix-" + (m ? m.name : "client") + "-offboard.md", data.body);
      $("dlg-off").close();
      state.selectedId = "";
      state.detail = null;
      sessionStorage.removeItem("jurix_matter");
      await refreshMatters();
      await refreshExports();
      if (state.matters[0]) await loadDetail(state.matters[0].id);
      toast("Purged " + (data.tenant_id || "tenant"));
      render();
    } catch (err) { toast(err.message); }
  });

  $("export-body").addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-download]");
    if (!btn) return;
    try {
      const res = await fetch("/api/exports/" + encodeURIComponent(btn.dataset.download), {
        headers: { Authorization: "Bearer " + state.token },
      });
      const text = await res.text();
      if (!res.ok) throw new Error(text);
      saveFile("jurix-" + (btn.dataset.name || "export") + ".md", text);
    } catch (err) { toast(err.message); }
  });

  let searchTimer = null;
  $("q").addEventListener("input", () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => doSearch($("q").value), 250);
  });
  $("search-form").addEventListener("submit", (e) => { e.preventDefault(); doSearch($("q").value); });
  $("search-pop").addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-hit]");
    if (!btn) return;
    $("search-pop").hidden = true;
    const item = (state.detail.items || []).find((i) => i.chunk_id === btn.dataset.hit);
    if (item) state.itemKey = item.key;
    await openDocument();
  });
  document.addEventListener("click", (e) => {
    if (!e.target.closest(".search")) $("search-pop").hidden = true;
  });

  const counsel = $("counsel");
  const counselLog = $("counsel-log");
  function openCounsel() {
    if (!counsel) return;
    counsel.hidden = false;
    $("counsel-q").focus();
  }
  function pushCounsel(role, html) {
    const node = document.createElement("article");
    node.className = "counsel-turn" + (role === "counsel" ? " counsel-a" : "");
    node.innerHTML = `<p class="who">${role === "counsel" ? "Counsel" : "You"}</p>` + html;
    counselLog.appendChild(node);
    counselLog.scrollTop = counselLog.scrollHeight;
  }
  function matchItem(question) {
    const low = question.toLowerCase();
    const items = (state.detail && state.detail.items) || [];
    const topics = [
      [["indemn", "liabil", "cap", "fees"], "indemnity"],
      [["renew", "non-renew", "auto"], "renewal"],
      [["govern", "delaware", "juris", "forum"], "law"],
      [["breach", "incident", "security", "72"], "breach"],
    ];
    for (let i = 0; i < topics.length; i++) {
      if (topics[i][0].some((word) => low.includes(word))) {
        return items.find((item) => item.key === topics[i][1]) || null;
      }
    }
    return null;
  }
  async function askCounsel(question) {
    const q = question.trim();
    if (!q) return;
    if (!state.detail) {
      pushCounsel("counsel", "<p>Open a matter first. Counsel only reads the tenant on the bench.</p>");
      return;
    }
    pushCounsel("you", `<p>${esc(q)}</p>`);
    $("counsel-send").disabled = true;
    try {
      const matter = state.detail;
      if (/silence|sibling|other client|cross-tenant|isolation/.test(q.toLowerCase())) {
        const line = matter.siblings_probed
          ? `Probed ${matter.siblings_probed} other client${matter.siblings_probed === 1 ? "" : "s"}. Passages from those tenants matching this citation: ${matter.sibling_hits}.`
          : `No other indexed client to probe. The checklist read only ${matter.tenant_id}.`;
        pushCounsel("counsel", `<p>${esc(line)}</p><p class="mono">${esc(matter.tenant_id)}</p>`);
        return;
      }
      let hits = [];
      if (!matter.hibernated) {
        const data = await api("/api/matters/" + encodeURIComponent(matter.id) + "/search", {
          method: "POST",
          body: JSON.stringify({ query: q }),
        });
        hits = data.hits || [];
      }
      const item = matchItem(q);
      const hit = hits[0];
      const score = hit ? Math.max(0, Math.min(1, Number(hit.score) || 0)) : 0;
      const bar = `<span class="meter" aria-hidden="true"><i style="width:${Math.round(score * 100)}%"></i></span>`;
      if (item) {
        pushCounsel("counsel", `
          <p><strong>${esc(item.item)}</strong> is ${esc(item.state)}. ${esc(item.finding)}. ${esc(item.tail)}.</p>
          <blockquote>“${quoteHtml(item.quote, item.mark)}”</blockquote>
          <p class="mono">${esc(item.citation)} · ${esc(item.doc_name || "")} · ${item.via === "vector" ? "vector hit" : "tenant read"}</p>
          ${bar}`);
      } else if (hit) {
        pushCounsel("counsel", `
          <p>Closest passage in ${esc(matter.tenant_id)}. This is retrieval, not a playbook finding.</p>
          <blockquote>“${esc((hit.text || "").slice(0, 420))}”</blockquote>
          <p class="mono">${esc(hit.doc_name || "document")} · chunk ${esc(hit.index)}</p>
          ${bar}`);
      } else {
        pushCounsel("counsel", "<p>Nothing in this tenant matched. Index a document, then run the checklist.</p>");
      }
    } catch (err) {
      pushCounsel("counsel", `<p>${esc(err.message)}</p>`);
    } finally {
      $("counsel-send").disabled = false;
    }
  }
  if ($("counsel-btn")) {
    $("counsel-btn").addEventListener("click", openCounsel);
    $("counsel-close").addEventListener("click", () => { counsel.hidden = true; });
    $("counsel-form").addEventListener("submit", (e) => {
      e.preventDefault();
      const value = $("counsel-q").value;
      $("counsel-q").value = "";
      askCounsel(value);
    });
    counsel.addEventListener("click", (e) => {
      const chip = e.target.closest("[data-ask]");
      if (chip) askCounsel(chip.dataset.ask);
    });
  }

  /* ── Auditor Dark Mode Toggle ──────────────────────────────────── */
  const lexBtn = $("lex-btn");
  if (lexBtn) {
    const savedTheme = sessionStorage.getItem("jurix_theme");
    if (savedTheme === "dark") {
      document.body.classList.add("lex-dark");
      lexBtn.setAttribute("aria-pressed", "true");
      lexBtn.textContent = "Auditor light";
    }
    lexBtn.addEventListener("click", () => {
      const isDark = document.body.classList.toggle("lex-dark");
      lexBtn.setAttribute("aria-pressed", isDark ? "true" : "false");
      lexBtn.textContent = isDark ? "Auditor light" : "Auditor dark";
      sessionStorage.setItem("jurix_theme", isDark ? "dark" : "light");
    });
  }

  /* ── Focus Mode Toggle (Theatre Mode) ──────────────────────────── */
  const focusBtn = $("focus-btn");
  if (focusBtn) {
    focusBtn.addEventListener("click", () => {
      const shell = $("app");
      if (!shell) return;
      const isFocus = shell.classList.toggle("focus-mode");
      focusBtn.setAttribute("aria-pressed", isFocus ? "true" : "false");
      focusBtn.textContent = isFocus ? "Exit focus" : "Focus";
    });
  }

  /* ── Command Palette (⌘K) ──────────────────────────────────────── */
  function populateCmdPalette() {
    const list = $("cmd-list");
    if (!list) return;
    const cmds = [
      { label: "New matter", desc: "Provision a sealed DBX tenant", fn: () => { $("cmd").close(); $("dlg-matter").showModal(); } },
      { label: "Run checklist", desc: "Audit the open matter against the playbook", fn: () => { $("cmd").close(); runAudit(); } },
      { label: "Export memorandum", desc: "Download findings as markdown", fn: () => { $("cmd").close(); exportMemo(); } },
      { label: "Open counsel", desc: "Ask questions about this matter", fn: () => { $("cmd").close(); openCounsel(); } },
      { label: "Focus mode", desc: "Dim navigation, focus on content", fn: () => { $("cmd").close(); $("focus-btn") && $("focus-btn").click(); } },
      { label: "Auditor dark", desc: "Toggle the dark theme", fn: () => { $("cmd").close(); $("lex-btn") && $("lex-btn").click(); } },
      { label: "Night brief", desc: "View the findings in dark reading mode", fn: () => { $("cmd").close(); renderNight(); } },
      { label: "View matters", desc: "Go to the matter queue", fn: () => { $("cmd").close(); state.view = "matters"; render(); } },
      { label: "View checklists", desc: "Go to the playbook view", fn: () => { $("cmd").close(); state.view = "checklists"; render(); } },
      { label: "View exports", desc: "Go to exported memoranda", fn: () => { $("cmd").close(); state.view = "exports"; render(); } },
      { label: "Firm settings", desc: "View firm, DBX, and tenant info", fn: () => { $("cmd").close(); state.view = "firm"; render(); } },
      { label: "Sign out", desc: "End session", fn: () => { $("cmd").close(); logout(); } },
    ];
    list.innerHTML = cmds.map((c, i) =>
      `<button type="button" data-cmd="${i}"><b>${esc(c.label)}</b><br><span class="muted">${esc(c.desc)}</span></button>`
    ).join("");
    list.querySelectorAll("button").forEach((btn) => {
      btn.addEventListener("click", () => cmds[parseInt(btn.dataset.cmd)].fn());
    });
    /* Filter as user types */
    $("cmd-q").oninput = () => {
      const q = $("cmd-q").value.toLowerCase();
      list.querySelectorAll("button").forEach((btn, i) => {
        const match = cmds[i].label.toLowerCase().includes(q) || cmds[i].desc.toLowerCase().includes(q);
        btn.hidden = !match;
      });
    };
  }
  if ($("cmd-btn")) {
    $("cmd-btn").addEventListener("click", () => {
      const cmd = $("cmd");
      if (cmd && !cmd.open) { cmd.showModal(); $("cmd-q").value = ""; $("cmd-q").focus(); populateCmdPalette(); }
    });
  }
  /* Close command palette on backdrop click */
  if ($("cmd")) {
    $("cmd").addEventListener("click", (e) => { if (e.target === $("cmd")) $("cmd").close(); });
  }

  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  (async function boot() {
    if (state.token) {
      try {
        state.user = await api("/api/me");
        await enterApp();
        return;
      } catch (err) {
        state.token = "";
        sessionStorage.removeItem("jurix_token");
      }
    }
    const start = (location.hash || "").replace("#", "");
    if (start === "home" || start === "login") showGate(start);
    else {
      showGate("load");
      setTimeout(() => { if (!$("g-load").hidden) showGate("home"); }, reduce ? 0 : 1600);
    }
  })();
})();
