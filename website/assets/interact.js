/* Interactive site widgets. Loaded after site.js. */
(function () {
  "use strict";

  var POSTS_HEAD = "recall-2026-09-10";
  var POSTS_KEY = "dbx-posts-seen";

  function $(sel, root) {
    return (root || document).querySelector(sel);
  }
  function $all(sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  }
  function on(el, ev, fn) {
    if (el) el.addEventListener(ev, fn);
  }

  function cosine(a, b) {
    var dot = 0, na = 0, nb = 0, i;
    for (i = 0; i < a.length; i++) {
      dot += a[i] * b[i];
      na += a[i] * a[i];
      nb += b[i] * b[i];
    }
    return dot / (Math.sqrt(na) * Math.sqrt(nb) + 1e-12);
  }
  function mix(a, b, wa, wb) {
    var out = [], i, n;
    for (i = 0; i < a.length; i++) out.push(wa * a[i] + wb * b[i]);
    n = Math.sqrt(out.reduce(function (s, x) { return s + x * x; }, 0)) || 1;
    return out.map(function (x) { return x / n; });
  }
  function fmt(x) {
    return (x * 100).toFixed(1);
  }

  function initRecall() {
    var root = $("#recall-bench");
    if (!root) return;
    var catalog = {
      alpha:   { id: "alpha",   text: [0.98, 0.12, 0.04], image: [0.18, 0.92, 0.08] },
      bravo:   { id: "bravo",   text: [0.91, 0.28, 0.10], image: [0.22, 0.88, 0.14] },
      charlie: { id: "charlie", text: [0.20, 0.18, 0.96], image: [0.12, 0.22, 0.94] },
      delta:   { id: "delta",   text: [0.55, 0.78, 0.12], image: [0.82, 0.48, 0.10] },
      echo:    { id: "echo",    text: [0.14, 0.90, 0.22], image: [0.10, 0.86, 0.28] },
      foxtrot: { id: "foxtrot", text: [0.40, 0.40, 0.82], image: [0.30, 0.35, 0.88] }
    };
    var query = { text: [0.96, 0.18, 0.06], image: [0.20, 0.90, 0.10] };
    var modeEl = $("#recall-mode");
    var wText = $("#recall-w-text");
    var wImg = $("#recall-w-img");
    var minEl = $("#recall-min");
    var out = $("#recall-out");
    var meta = $("#recall-meta");
    var wrow = $("#recall-weights");
    var minVal = $("#recall-min-val");
    if (!modeEl || !minEl || !out) return;

    function run() {
      var mode = modeEl.value;
      var min = parseFloat(minEl.value) || 0;
      var wt = parseFloat((wText && wText.value) || "0.6") || 0.6;
      var wi = parseFloat((wImg && wImg.value) || "0.4") || 0.4;
      if (wrow) wrow.hidden = mode !== "vfuse";
      var qVec, spaceNote, scored;
      if (mode === "vsim") {
        qVec = mix(catalog.alpha.text, catalog.alpha.image, 0.5, 0.5);
        spaceNote = "VSIM alpha — stored embedding, self excluded. Toy cosine, not a live node.";
        scored = Object.keys(catalog).filter(function (k) { return k !== "alpha"; }).map(function (k) {
          var row = catalog[k];
          var v = mix(row.text, row.image, 0.5, 0.5);
          return { id: k, score: cosine(qVec, v), note: "default space" };
        });
      } else if (mode === "vfuse") {
        qVec = mix(query.text, query.image, wt, wi);
        spaceNote = "VFUSE WEIGHTS " + wt.toFixed(2) + "," + wi.toFixed(2) + " — late fusion of SPACE text + SPACE image.";
        scored = Object.keys(catalog).map(function (k) {
          var row = catalog[k];
          var fused = mix(row.text, row.image, wt, wi);
          return { id: k, score: cosine(qVec, fused), note: "text+image" };
        });
      } else if (mode === "space-image") {
        qVec = query.image;
        spaceNote = "VSEARCH SPACE image — image vectors only. Text neighbors stay out of this ANN.";
        scored = Object.keys(catalog).map(function (k) {
          return { id: k, score: cosine(qVec, catalog[k].image), note: "SPACE image" };
        });
      } else {
        qVec = query.text;
        spaceNote = "VSEARCH SPACE text MIN_SCORE " + min.toFixed(2) + " — toy cosine, not a live node.";
        scored = Object.keys(catalog).map(function (k) {
          return { id: k, score: cosine(qVec, catalog[k].text), note: "SPACE text" };
        });
      }
      scored.sort(function (a, b) { return b.score - a.score; });
      var kept = scored.filter(function (r) { return r.score >= min; });
      if (meta) meta.textContent = spaceNote + " Showing " + kept.length + "/" + scored.length + " above MIN_SCORE.";
      out.innerHTML = kept.map(function (r, i) {
        var w = Math.max(4, Math.round(r.score * 100));
        return '<li><span class="rank">' + (i + 1) + '</span><span class="id">' + r.id +
          '</span><span class="bar"><i style="width:' + w + '%"></i></span><span class="score">' +
          fmt(r.score) + '</span><span class="note">' + r.note + '</span></li>';
      }).join("") || '<li class="empty">No neighbors above MIN_SCORE. Lower the floor.</li>';
    }
    on(modeEl, "change", run);
    on(wText, "input", run);
    on(wImg, "input", run);
    on(minEl, "input", function () {
      if (minVal) minVal.textContent = parseFloat(minEl.value).toFixed(2);
      run();
    });
    run();
  }

  function initLeak() {
    var root = $("#leak-demo");
    if (!root) return;
    var shared = $("#leak-shared");
    var dbx = $("#leak-dbx");
    var btn = $("#leak-run");
    var sharedLog = [
      { cls: "ok", t: "AUTH default" },
      { cls: "ok", t: "SET user:alice:session tok-a" },
      { cls: "ok", t: "SELECT 1" },
      { cls: "ok", t: "SET user:bob:session tok-b" },
      { cls: "warn", t: "KEYS user:*" },
      { cls: "bad", t: "1) user:alice:session" },
      { cls: "bad", t: "2) user:bob:session   ← bob's key leaked across logical DB" }
    ];
    var dbxLog = [
      { cls: "ok", t: "AUTH tenant-alice" },
      { cls: "ok", t: "SET session tok-a" },
      { cls: "ok", t: "KEYS *" },
      { cls: "ok", t: "1) session" },
      { cls: "ok", t: "AUTH tenant-bob" },
      { cls: "ok", t: "SET session tok-b" },
      { cls: "ok", t: "KEYS *" },
      { cls: "ok", t: "1) session" },
      { cls: "ok", t: "GET user:alice:session" },
      { cls: "ok", t: "(nil)  ← alice's worker never held bob's heap" }
    ];
    function typeLog(el, lines, i) {
      if (i >= lines.length) return;
      var row = document.createElement("div");
      row.className = "term-line " + lines[i].cls;
      row.textContent = lines[i].t;
      el.appendChild(row);
      el.scrollTop = el.scrollHeight;
      setTimeout(function () { typeLog(el, lines, i + 1); }, 220);
    }
    on(btn, "click", function () {
      shared.innerHTML = "";
      dbx.innerHTML = "";
      typeLog(shared, sharedLog, 0);
      setTimeout(function () { typeLog(dbx, dbxLog, 0); }, 80);
    });
  }

  function initDensity() {
    var root = $("#density-demo");
    if (!root) return;
    var slider = $("#density-n");
    var val = $("#density-n-val");
    var rss = $("#density-rss");
    var note = $("#density-note");
    function run() {
      var n = parseInt(slider.value, 10);
      val.textContent = String(n);
      var mib = n * 15.5;
      var gib = mib / 1024;
      rss.textContent = gib >= 1 ? gib.toFixed(2) + " GiB" : Math.round(mib) + " MiB";
      note.textContent = n + " idle Isolation Kernel workers × ~15.5 MiB (mid of the 14–17 MiB strict band). No dollar figure — price the box you already run.";
    }
    on(slider, "input", run);
    run();
  }

  function initQuiz() {
    var form = $("#fit-quiz");
    if (!form) return;
    var out = $("#quiz-out");
    on(form, "submit", function (e) {
      e.preventDefault();
      var keys = ["q1", "q2", "q3", "q4"];
      var answers = { q1: "tenant", q2: "caller", q3: "kernel", q4: "kv" };
      var score = 0;
      var missing = false;
      keys.forEach(function (k) {
        var picked = form.querySelector('input[name="' + k + '"]:checked');
        if (!picked) missing = true;
        else if (picked.value === answers[k]) score++;
      });
      if (missing) {
        out.hidden = false;
        out.className = "quiz-out warn";
        out.innerHTML = "<p>Answer all four. This is a fit check, not a lead form.</p>";
        return;
      }
      out.hidden = false;
      if (score <= 1) {
        out.className = "quiz-out bad";
        out.innerHTML = "<p><strong>Not a fit.</strong> You want shared-process Redis, a model host, or SQL. DBX is none of those. Read <a href='docs/positioning.html'>positioning</a> and stop here.</p>";
      } else if (score === 2) {
        out.className = "quiz-out warn";
        out.innerHTML = "<p><strong>Maybe.</strong> Isolation is real; recall is ANN on vectors you send. If that still matches, <a href='start.html'>get started</a>. If you need CLIP-in-process or joins, it does not.</p>";
      } else {
        out.className = "quiz-out ok";
        out.innerHTML = "<p><strong>" + score + "/4 — this is the product.</strong> One tenant, one worker, KV + SQ8 HNSW, Isolation Kernel when you turn it on. <a href='start.html'>Get started</a> or watch the <a href='demo.html'>walkthrough</a>.</p>";
      }
    });
  }

  function initShred() {
    var root = $("#shred-demo");
    if (!root) return;
    var btn = $("#shred-run");
    var reset = $("#shred-reset");
    var cards = $all("[data-shred]", root);
    function setState(dead) {
      cards.forEach(function (c) {
        var kind = c.getAttribute("data-shred");
        var body = c.querySelector(".shred-body");
        if (!dead) {
          c.classList.remove("dead", "plain");
          body.textContent = c.getAttribute("data-live");
          return;
        }
        if (kind === "vec") {
          c.classList.add("plain");
          body.textContent = "SQ8 mmap still plaintext on disk. Isolation Kernel does not encrypt .vec rows. Put the data dir on LUKS or fscrypt.";
        } else {
          c.classList.add("dead");
          body.textContent = "DEK revoked. Ciphertext without a key. Unreadable.";
        }
      });
    }
    on(btn, "click", function () { setState(true); });
    on(reset, "click", function () { setState(false); });
  }

  function initChapters() {
    $all("[data-jump]").forEach(function (a) {
      on(a, "click", function (e) {
        var sel = a.getAttribute("data-video");
        var t = parseFloat(a.getAttribute("data-jump"));
        var v = sel ? $(sel) : a.closest(".film, .demo-block, main") && a.closest("main").querySelector("video");
        if (!v) v = document.querySelector("video");
        if (!v || isNaN(t)) return;
        e.preventDefault();
        v.currentTime = t;
        v.play();
      });
    });
  }

  function initHotspots() {
    $all(".hotspot").forEach(function (btn) {
      on(btn, "click", function () {
        var id = btn.getAttribute("data-spot");
        $all(".hotspot").forEach(function (b) { b.classList.toggle("on", b === btn); });
        $all(".spot-note").forEach(function (n) { n.hidden = n.getAttribute("data-spot") !== id; });
      });
    });
  }

  function initPostsBadge() {
    var onPosts = document.body.getAttribute("data-page") === "posts" || /posts\.html/.test(location.pathname);
    if (onPosts) {
      localStorage.setItem(POSTS_KEY, POSTS_HEAD);
    }
    var seen = localStorage.getItem(POSTS_KEY);
    var unread = seen !== POSTS_HEAD;
    $all("[data-posts-badge]").forEach(function (el) {
      el.hidden = !unread;
      el.textContent = unread ? "1" : "";
    });
  }

  function patchCommandPalette() {
    /* site.js already bound ⌘K with keyword field k. */
  }

  initRecall();
  initLeak();
  initDensity();
  initQuiz();
  initShred();
  initChapters();
  initHotspots();
  initPostsBadge();
  patchCommandPalette();
})();
