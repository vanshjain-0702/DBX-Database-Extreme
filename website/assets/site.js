(function () {
  document.documentElement.classList.add("js");

  function joinRoot(root, file) {
    if (!root || root === ".") return file;
    return root.replace(/\/$/, "") + "/" + file;
  }

  function injectChrome() {
    var mast = document.getElementById("masthead");
    var foot = document.getElementById("colophon");
    var rail = document.getElementById("docs-rail");
    var root = document.body.getAttribute("data-root") || ".";
    var page = document.body.getAttribute("data-page") || "";

    function href(file) {
      return joinRoot(root, file);
    }
    function current(id) {
      return page === id ? ' aria-current="page"' : "";
    }

    var productPages = { features: 1, architecture: 1, start: 1, performance: 1 };
    var docsPages = {
      docs: 1,
      "docs-quickstart": 1,
      "docs-architecture": 1,
      "docs-api": 1,
      "docs-positioning": 1,
    };
    var productMark = productPages[page] ? ' aria-current="true"' : "";
    var docsMark = docsPages[page] ? ' aria-current="page"' : "";

    if (mast) {
      mast.outerHTML =
        '<header class="site-header">' +
        '<div class="header-inner">' +
        '<a class="brand" href="' + href("index.html") + '"><img src="' + href("assets/logo.jpg") + '" width="48" height="48" alt=""><span>DBX</span></a>' +
        '<span class="live-meta"><span data-clock>00:00:00</span><span class="live-port">:6380</span></span>' +
        '<button type="button" class="cmdk-launch cmdk-launch--bar" data-cmdk-open aria-label="Search the site">' +
        '<span>Search</span><kbd data-cmdk-kbd>⌘K</kbd></button>' +
        '<button class="nav-toggle" type="button" aria-expanded="false" aria-controls="site-nav">Menu</button>' +
        '<nav id="site-nav" class="site-nav" aria-label="Primary">' +
        "<ul>" +
        '<li class="has-sub">' +
        '<a href="' + href("features.html") + '"' + productMark + ">Product</a>" +
        '<ul class="sub">' +
        "<li><a href=\"" + href("features.html") + '"' + current("features") + ">Features</a></li>" +
        "<li><a href=\"" + href("architecture.html") + '"' + current("architecture") + ">Architecture</a></li>" +
        "<li><a href=\"" + href("start.html") + '"' + current("start") + ">Get started</a></li>" +
        "<li><a href=\"" + href("performance.html") + '"' + current("performance") + ">Performance</a></li>" +
        "</ul></li>" +
        "<li><a href=\"" + href("docs/index.html") + '"' + docsMark + ">Docs</a></li>" +
        "<li><a href=\"" + href("pricing.html") + '"' + current("pricing") + ">Pricing</a></li>" +
        "<li><a href=\"" + href("security.html") + '"' + current("security") + ">Security</a></li>" +
        "<li><a href=\"" + href("contact.html") + '"' + current("contact") + ">Contact</a></li>" +
        "</ul>" +
        '<div class="nav-actions">' +
        '<button type="button" class="cmdk-launch cmdk-launch--nav" data-cmdk-open aria-label="Search the site">' +
        '<span>Search</span><kbd data-cmdk-kbd>⌘K</kbd></button>' +
        '<a href="https://github.com/vanshjain-0702/DBX-Database-Extreme">GitHub</a>' +
        '<a class="btn" href="' + href("start.html") + '">Get started</a>' +
        "</div></nav></div></header>";
    }

    if (foot) {
      foot.outerHTML =
        '<footer class="site-footer">' +
        '<div class="footer-grid">' +
        "<div>" +
        '<p class="footer-brand"><img src="' + href("assets/logo.jpg") + '" width="56" height="56" alt=""><span>DBX</span></p>' +
        '<p class="muted">Per-tenant memory for AI products. One engine per customer.</p>' +
        '<p><a href="mailto:hello@dbxdb.io">hello@dbxdb.io</a></p>' +
        "</div>" +
        "<div><h2>Product</h2><ul>" +
        "<li><a href=\"" + href("features.html") + '">Features</a></li>' +
        "<li><a href=\"" + href("architecture.html") + '">Architecture</a></li>' +
        "<li><a href=\"" + href("start.html") + '">Get started</a></li>' +
        "<li><a href=\"" + href("performance.html") + '">Performance</a></li>' +
        "<li><a href=\"" + href("pricing.html") + '">Pricing</a></li>' +
        "</ul></div>" +
        "<div><h2>Docs &amp; legal</h2><ul>" +
        "<li><a href=\"" + href("docs/index.html") + '">Documentation</a></li>' +
        "<li><a href=\"" + href("security.html") + '">Security</a></li>' +
        "<li><a href=\"" + href("changelog.html") + '">Changelog</a></li>' +
        "<li><a href=\"" + href("license.html") + '">License</a></li>' +
        "<li><a href=\"" + href("privacy.html") + '">Privacy</a></li>' +
        "<li><a href=\"" + href("terms.html") + '">Terms</a></li>' +
        "</ul></div>" +
        "<div><h2>Company</h2><ul>" +
        "<li><a href=\"" + href("contact.html") + '">Contact</a></li>' +
        '<li><a href="https://github.com/vanshjain-0702/DBX-Database-Extreme">GitHub</a></li>' +
        '<li><a href="https://github.com/vanshjain-0702/DBX-Database-Extreme/releases">Releases</a></li>' +
        "</ul></div>" +
        "</div>" +
        '<div class="legal">' +
        "<span>BSL 1.1 · Apache 2.0 after four years.</span>" +
        "<span>© 2026 DBX</span>" +
        "</div></footer>";
    }

    if (rail) {
      var docs = [
        ["docs/index.html", "docs", "Overview"],
        ["docs/quickstart.html", "docs-quickstart", "Quickstart"],
        ["docs/architecture.html", "docs-architecture", "Architecture"],
        ["docs/api.html", "docs-api", "API"],
        ["docs/positioning.html", "docs-positioning", "Positioning"],
      ];
      rail.outerHTML =
        '<nav class="docs-nav" aria-label="Docs">' +
        '<p class="docs-nav-label">Documentation</p>' +
        docs
          .map(function (d) {
            return "<a href=\"" + href(d[0]) + '"' + current(d[1]) + ">" + d[2] + "</a>";
          })
          .join("") +
        "</nav>";
    }
  }

  injectChrome();
  injectSurface();

  var header = document.querySelector(".site-header");
  var toggle = document.querySelector(".nav-toggle");
  var nav = document.querySelector("#site-nav");
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var page = document.body.getAttribute("data-page") || "";
  var root = document.body.getAttribute("data-root") || ".";

  function injectSurface() {
    if (document.querySelector(".saas-bg")) return;
    var bg = document.createElement("div");
    bg.className = "saas-bg";
    bg.setAttribute("aria-hidden", "true");
    bg.innerHTML =
      '<div class="saas-bg__orb saas-bg__orb--a"></div>' +
      '<div class="saas-bg__orb saas-bg__orb--b"></div>' +
      '<div class="saas-bg__orb saas-bg__orb--c"></div>' +
      '<div class="saas-bg__grid"></div>' +
      '<div class="saas-bg__vignette"></div>' +
      '<div class="saas-bg__noise"></div>';
    document.body.insertBefore(bg, document.body.firstChild);

    var bar = document.createElement("div");
    bar.className = "scroll-progress";
    bar.setAttribute("aria-hidden", "true");
    document.body.appendChild(bar);

    var light = document.createElement("dialog");
    light.className = "lightbox";
    light.setAttribute("data-lightbox", "");
    light.innerHTML =
      '<div class="lightbox-head"><span>Operator UI</span><button class="lightbox-close" type="button" aria-label="Close">×</button></div>' +
      '<figure class="lightbox-inner"><img alt=""><figcaption></figcaption></figure>';
    document.body.appendChild(light);

    var pop = document.createElement("dialog");
    pop.className = "pop";
    pop.setAttribute("data-pop", "");
    pop.innerHTML =
      '<div class="pop-head"><span data-pop-title>Tenant</span><button class="pop-close" type="button" aria-label="Close">×</button></div>' +
      '<div class="pop-body" data-pop-body></div>';
    document.body.appendChild(pop);

    var tip = document.createElement("div");
    tip.className = "tip";
    tip.hidden = true;
    tip.setAttribute("role", "tooltip");
    document.body.appendChild(tip);

    var toast = document.createElement("div");
    toast.className = "toast";
    toast.setAttribute("data-toast", "");
    toast.innerHTML =
      "<p>Each cabinet is a different engine. GET on harbor cannot see acme’s session.</p>" +
      '<div class="btn-row"><button type="button" class="btn" data-toast-dismiss>Got it</button></div>';
    document.body.appendChild(toast);

    var topBtn = document.createElement("button");
    topBtn.className = "to-top";
    topBtn.type = "button";
    topBtn.setAttribute("aria-label", "Back to top");
    topBtn.textContent = "↑";
    document.body.appendChild(topBtn);

    var cmdk = document.createElement("dialog");
    cmdk.className = "cmdk";
    cmdk.setAttribute("data-cmdk", "");
    cmdk.innerHTML =
      '<form class="cmdk-head" data-cmdk-form>' +
      '<input type="search" data-cmdk-q placeholder="Jump to a page or section…" autocomplete="off" spellcheck="false" aria-label="Search the site">' +
      "<kbd>esc</kbd></form>" +
      '<ul class="cmdk-list" data-cmdk-list role="listbox"></ul>' +
      '<p class="cmdk-foot">Browser map. Does not query a live node.</p>';
    document.body.appendChild(cmdk);
  }

  if (!reduce) {
    window.addEventListener(
      "pointermove",
      function (e) {
        document.documentElement.style.setProperty("--mx", e.clientX + "px");
        document.documentElement.style.setProperty("--my", e.clientY + "px");
      },
      { passive: true }
    );
  }

  var topBtn = document.querySelector(".to-top");
  function onScroll() {
    if (header) header.classList.toggle("is-compact", window.scrollY > 12);
    var max = document.documentElement.scrollHeight - window.innerHeight;
    var pct = max > 0 ? (window.scrollY / max) * 100 : 0;
    document.documentElement.style.setProperty("--scroll", pct.toFixed(2) + "%");
    document.documentElement.style.setProperty("--sy", String(window.scrollY));
    if (topBtn) topBtn.classList.toggle("is-on", window.scrollY > 420);
  }
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });
  if (topBtn) {
    topBtn.addEventListener("click", function () {
      window.scrollTo({ top: 0, behavior: reduce ? "auto" : "smooth" });
    });
  }

  if (toggle && nav) {
    toggle.addEventListener("click", function () {
      var open = nav.classList.toggle("is-open");
      toggle.setAttribute("aria-expanded", String(open));
      toggle.textContent = open ? "Close" : "Menu";
    });
    nav.querySelectorAll("a").forEach(function (link) {
      link.addEventListener("click", function () {
        if (window.matchMedia("(max-width: 880px)").matches) {
          nav.classList.remove("is-open");
          toggle.setAttribute("aria-expanded", "false");
          toggle.textContent = "Menu";
        }
      });
    });
  }

  document.querySelectorAll("[data-clock]").forEach(function (el) {
    function tick() {
      var now = new Date();
      el.textContent = now.toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      });
    }
    tick();
    window.setInterval(tick, 1000);
  });

  document.querySelectorAll(".code-panel").forEach(function (panel) {
    var btn = panel.querySelector(".copy-btn");
    var code = panel.querySelector("pre");
    if (!btn || !code) return;
    btn.addEventListener("click", function () {
      var text = code.textContent || "";
      var done = function () {
        var prev = btn.textContent;
        btn.textContent = "Copied";
        window.setTimeout(function () {
          btn.textContent = prev;
        }, 1400);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(function () {
          fallbackCopy(text, done);
        });
      } else {
        fallbackCopy(text, done);
      }
    });
  });

  function fallbackCopy(text, done) {
    var area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "absolute";
    area.style.left = "-9999px";
    document.body.appendChild(area);
    area.select();
    try {
      document.execCommand("copy");
      done();
    } finally {
      document.body.removeChild(area);
    }
  }

  /* ——— Isolation bench (in-browser demo, not a live server) ——— */

  var stores = {
    acme: { session: null, vectors: 0, wal: 0 },
    harbor: { session: null, vectors: 0, wal: 0 },
    lumen: { session: null, vectors: 0, wal: 0 },
  };
  var authed = "acme";
  var scriptTimer = 0;
  var scriptIndex = 0;
  var paused = false;
  var typing = false;

  var script = [
    { cmd: "AUTH acme:writer ***", wait: 420, run: function () { authed = "acme"; } },
    { cmd: 'SET session:42 {"thread":"onboarding","step":3}', wait: 520, run: function () { write("acme", "session", "onboarding · step 3"); } },
    { cmd: "VADD memories doc:1 [0.12, 0.81, 0.44]", wait: 560, run: function () { write("acme", "vector"); } },
    { cmd: "GET session:42", wait: 480, run: function () { reply(get("acme")); } },
    { cmd: "AUTH harbor:writer ***", wait: 640, run: function () { authed = "harbor"; log("switched identity — this is a different engine"); } },
    { cmd: "GET session:42", wait: 520, run: function () { reply("(nil)  — harbor has no such key"); } },
    { cmd: "SET session:42 harbor-intake", wait: 500, run: function () { write("harbor", "session", "harbor-intake"); } },
    { cmd: "AUTH acme:writer ***", wait: 640, run: function () { authed = "acme"; } },
    { cmd: "GET session:42", wait: 500, run: function () { reply(get("acme") + "  — still here. prefixes never entered it."); } },
  ];

  function write(id, kind, value) {
    var row = stores[id];
    if (kind === "session") row.session = value;
    if (kind === "vector") row.vectors += 1;
    row.wal += 1;
    paintCabinets();
    flashCabinet(id);
  }

  function flashCabinet(id) {
    var cab = document.querySelector('[data-cabinet="' + id + '"]');
    if (!cab) return;
    cab.classList.remove("is-flash");
    void cab.offsetWidth;
    cab.classList.add("is-flash");
  }

  function get(id) {
    return stores[id].session || "(nil)";
  }

  function log(line) {
    var out = document.querySelector("[data-term]");
    if (!out) return;
    var p = document.createElement("div");
    p.className = "term-line term-line--meta";
    p.textContent = "# " + line;
    out.appendChild(p);
    out.scrollTop = out.scrollHeight;
  }

  function reply(text) {
    var out = document.querySelector("[data-term]");
    if (!out) return;
    var p = document.createElement("div");
    p.className = "term-line term-line--out";
    p.textContent = text;
    out.appendChild(p);
    out.scrollTop = out.scrollHeight;
  }

  function typeCommand(text, done) {
    var out = document.querySelector("[data-term]");
    if (!out) {
      done();
      return;
    }
    var line = document.createElement("div");
    line.className = "term-line";
    var prompt = document.createElement("span");
    prompt.className = "term-prompt";
    prompt.textContent = authed + "> ";
    var typed = document.createElement("span");
    line.appendChild(prompt);
    line.appendChild(typed);
    out.appendChild(line);
    if (reduce) {
      typed.textContent = text;
      out.scrollTop = out.scrollHeight;
      done();
      return;
    }
    var i = 0;
    function step() {
      typed.textContent = text.slice(0, i);
      out.scrollTop = out.scrollHeight;
      i += 1;
      if (i <= text.length) {
        window.setTimeout(step, 12 + Math.random() * 22);
      } else {
        done();
      }
    }
    step();
  }

  function paintCabinets() {
    Object.keys(stores).forEach(function (id) {
      var cab = document.querySelector('[data-cabinet="' + id + '"]');
      if (!cab) return;
      cab.classList.toggle("is-live", authed === id);
      var kv = cab.querySelector("[data-kv]");
      var vec = cab.querySelector("[data-vec]");
      var wal = cab.querySelector("[data-wal]");
      if (kv) kv.textContent = stores[id].session ? stores[id].session : "empty";
      if (vec) vec.textContent = String(stores[id].vectors);
      if (wal) wal.textContent = String(stores[id].wal);
    });
    document.querySelectorAll("[data-pick]").forEach(function (btn) {
      btn.classList.toggle("is-on", btn.getAttribute("data-pick") === authed);
    });
    var badge = document.querySelector("[data-auth-badge]");
    if (badge) badge.textContent = "AUTH " + authed;
    var prompt = document.querySelector("[data-term-prompt]");
    if (prompt) prompt.textContent = authed + "> ";
    var root = document.querySelector("[data-bench]");
    if (root) root.style.setProperty("--tenant", "var(--" + authed + ")");
  }

  function setPaused(on) {
    paused = !!on;
    var btn = document.querySelector("[data-pause]");
    if (btn) btn.textContent = paused ? "Play" : "Pause";
    if (paused) window.clearTimeout(scriptTimer);
    else if (bench && !typing) {
      scriptTimer = window.setTimeout(runScript, reduce ? 80 : 420);
    }
  }

  function runUserCommand(raw) {
    var text = (raw || "").trim();
    if (!text) return;
    setPaused(true);
    typeCommand(text, function () {
      var parts = text.split(/\s+/);
      var verb = (parts[0] || "").toUpperCase();
      if (verb === "AUTH") {
        var id = ((parts[1] || "").split(":")[0] || "").toLowerCase();
        if (stores[id]) {
          authed = id;
          log("AUTH bound this connection to " + id + " — a different engine");
        } else {
          reply("(error) unknown tenant. Sketch has acme, harbor, lumen.");
        }
      } else if (verb === "SET") {
        var val = text.replace(/^SET\s+\S+\s*/i, "");
        if (!val) val = parts.slice(1).join(" ") || "set";
        write(authed, "session", val);
        reply("OK");
      } else if (verb === "GET") {
        reply(get(authed));
      } else if (verb === "VADD") {
        write(authed, "vector");
        reply("(integer) 1");
      } else {
        reply("(error) sketch understands AUTH, SET, GET, VADD");
      }
      paintCabinets();
    });
  }

  function runScript() {
    if (paused || typing || document.hidden) return;
    if (scriptIndex >= script.length) {
      scriptIndex = 0;
      var out = document.querySelector("[data-term]");
      if (out) {
        var p = document.createElement("div");
        p.className = "term-line term-line--meta";
        p.textContent = "# loop — isolation does not depend on remembering a prefix";
        out.appendChild(p);
      }
    }
    var step = script[scriptIndex];
    typing = true;
    typeCommand(step.cmd, function () {
      typing = false;
      if (paused) return;
      step.run();
      paintCabinets();
      scriptIndex += 1;
      scriptTimer = window.setTimeout(runScript, reduce ? 80 : step.wait);
    });
  }

  var bench = document.querySelector("[data-bench]");
  if (bench) {
    paintCabinets();
    document.querySelectorAll("[data-pick]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        authed = btn.getAttribute("data-pick");
        paintCabinets();
        log("operator selected " + authed + " (no shared map)");
      });
    });
    document.querySelectorAll("[data-cabinet]").forEach(function (cab) {
      cab.addEventListener("click", function () {
        inspectTenant(cab.getAttribute("data-cabinet"));
      });
      cab.addEventListener("keydown", function (e) {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          inspectTenant(cab.getAttribute("data-cabinet"));
        }
      });
    });
    var replay = document.querySelector("[data-replay]");
    if (replay) {
      replay.addEventListener("click", function () {
        window.clearTimeout(scriptTimer);
        stores.acme = { session: null, vectors: 0, wal: 0 };
        stores.harbor = { session: null, vectors: 0, wal: 0 };
        stores.lumen = { session: null, vectors: 0, wal: 0 };
        authed = "acme";
        scriptIndex = 0;
        typing = false;
        paused = false;
        var pauseBtn = document.querySelector("[data-pause]");
        if (pauseBtn) pauseBtn.textContent = "Pause";
        var out = document.querySelector("[data-term]");
        if (out) out.innerHTML = "";
        paintCabinets();
        runScript();
      });
    }
    var pauseBtn = document.querySelector("[data-pause]");
    if (pauseBtn) {
      pauseBtn.addEventListener("click", function () {
        setPaused(!paused);
      });
    }
    var termForm = document.querySelector("[data-term-form]");
    if (termForm) {
      termForm.addEventListener("submit", function (e) {
        e.preventDefault();
        var input = termForm.querySelector("input");
        if (!input) return;
        runUserCommand(input.value);
        input.value = "";
      });
    }
    document.addEventListener("visibilitychange", function () {
      if (document.hidden) {
        window.clearTimeout(scriptTimer);
      } else if (!paused && !typing) {
        scriptTimer = window.setTimeout(runScript, 400);
      }
    });
    runScript();
  }

  /* ——— Tabs ——— */

  document.querySelectorAll("[data-tabs]").forEach(function (root) {
    var tabs = root.querySelectorAll("[data-tab]");
    var panels = root.querySelectorAll("[data-panel]");
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var id = tab.getAttribute("data-tab");
        tabs.forEach(function (t) {
          var on = t === tab;
          t.classList.toggle("is-on", on);
          t.setAttribute("aria-selected", String(on));
        });
        panels.forEach(function (p) {
          p.hidden = p.getAttribute("data-panel") !== id;
        });
      });
    });
  });

  /* ——— Lifecycle stepper ——— */

  document.querySelectorAll("[data-steps]").forEach(function (root) {
    var buttons = root.querySelectorAll("[data-step]");
    var views = root.querySelectorAll("[data-step-view]");
    buttons.forEach(function (btn) {
      btn.addEventListener("click", function () {
        var id = btn.getAttribute("data-step");
        buttons.forEach(function (b) {
          b.classList.toggle("is-on", b === btn);
        });
        views.forEach(function (v) {
          v.hidden = v.getAttribute("data-step-view") !== id;
        });
      });
    });
  });

  /* ——— Packet hop ——— */

  var hop = document.querySelector("[data-hop]");
  if (hop) {
    var stages = hop.querySelectorAll("[data-hop-stage]");
    var i = 0;
    function pulse() {
      stages.forEach(function (s, n) {
        s.classList.toggle("is-hot", n === i);
      });
      i = (i + 1) % stages.length;
    }
    pulse();
    window.setInterval(pulse, reduce ? 1600 : 1100);
  }

  /* ——— Reveal on scroll ——— */

  document.querySelectorAll(".section > .wrap, .section-tight > .wrap, .price-card, .change, .docs-toc a").forEach(function (el) {
    if (!el.hasAttribute("data-in")) el.setAttribute("data-in", "");
  });

  if (!reduce && "IntersectionObserver" in window) {
    var io = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            entry.target.classList.add("is-in");
            io.unobserve(entry.target);
          }
        });
      },
      { threshold: 0.12, rootMargin: "0px 0px -8% 0px" }
    );
    document.querySelectorAll("[data-in]").forEach(function (el, i) {
      el.style.transitionDelay = Math.min(i, 8) * 70 + "ms";
      io.observe(el);
    });
  } else {
    document.querySelectorAll("[data-in]").forEach(function (el) {
      el.classList.add("is-in");
    });
  }

  /* ——— Dashboard filmstrip ——— */

  var film = document.querySelector("[data-film]");
  if (film) {
    var root = document.body.getAttribute("data-root") || ".";
    var frames = [
      { src: "assets/product/tenants.png", cap: "Tenants — each row is an isolated engine" },
      { src: "assets/product/overview.png", cap: "Overview — one customer’s WAL, KV, and vectors" },
      { src: "assets/product/explorer.png", cap: "Explorer — inspect keys without leaving the binary" },
      { src: "assets/product/console.png", cap: "Console — AUTH, then RESP against that tenant" },
      { src: "assets/product/dark.png", cap: "Same operator UI, dark" },
    ];
    var img = film.querySelector("[data-film-frame]");
    var cap = film.querySelector("[data-film-cap]");
    var thumbs = film.querySelector("[data-film-thumbs]");
    var i = 0;
    var timer = 0;

    function show(n) {
      if (!img) return;
      i = (n + frames.length) % frames.length;
      var f = frames[i];
      img.src = joinRoot(root, f.src);
      img.alt = f.cap;
      if (cap) cap.textContent = f.cap;
      film.querySelectorAll("[data-film-goto]").forEach(function (btn) {
        btn.classList.toggle("is-on", Number(btn.getAttribute("data-film-goto")) === i);
      });
    }

    if (thumbs) {
      frames.forEach(function (f, n) {
        var b = document.createElement("button");
        b.type = "button";
        b.setAttribute("data-film-goto", String(n));
        b.setAttribute("aria-label", f.cap);
        var thumb = document.createElement("img");
        thumb.src = joinRoot(root, f.src);
        thumb.alt = "";
        b.appendChild(thumb);
        thumbs.appendChild(b);
        b.addEventListener("click", function () {
          window.clearInterval(timer);
          show(n);
        });
      });
    }

    var prev = film.querySelector("[data-film-prev]");
    var next = film.querySelector("[data-film-next]");
    if (prev) {
      prev.addEventListener("click", function () {
        window.clearInterval(timer);
        show(i - 1);
      });
    }
    if (next) {
      next.addEventListener("click", function () {
        window.clearInterval(timer);
        show(i + 1);
      });
    }
    show(0);
    if (!reduce) {
      timer = window.setInterval(function () {
        show(i + 1);
      }, 4200);
    }
  }

  /* ——— Contact: compose mailto / copy / GitHub issue ——— */

  function contactPayload(form) {
    var name = (form.querySelector('[name="name"]') || {}).value || "";
    var email = (form.querySelector('[name="email"]') || {}).value || "";
    var topic = (form.querySelector('[name="topic"]') || {}).value || "Contact";
    var message = (form.querySelector('[name="message"]') || {}).value || "";
    var text =
      "Name: " + name + "\nEmail: " + email + "\nTopic: " + topic + "\n\n" + message;
    return {
      name: name,
      email: email,
      topic: topic,
      message: message,
      text: text,
      subject: "DBX — " + topic,
      mailto:
        "mailto:hello@dbxdb.io?subject=" +
        encodeURIComponent("DBX — " + topic) +
        "&body=" +
        encodeURIComponent(text),
    };
  }

  function setContactStatus(form, text) {
    var note = form.querySelector("[data-contact-status]");
    if (!note) return;
    note.hidden = false;
    note.textContent = text;
  }

  function copyText(text, done, fail) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done).catch(function () {
        fallbackCopy(text, done);
      });
    } else {
      fallbackCopy(text, done);
    }
  }

  document.querySelectorAll("[data-contact]").forEach(function (form) {
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (!form.reportValidity()) return;
      var payload = contactPayload(form);
      var link = document.createElement("a");
      link.href = payload.mailto;
      link.style.display = "none";
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      setContactStatus(
        form,
        "Tried to open your mail app to hello@dbxdb.io. That domain has no MX yet, so a send will bounce. Copy the message or file a GitHub issue if you need it to arrive."
      );
    });

    var copyBtn = form.querySelector("[data-contact-copy]");
    if (copyBtn) {
      copyBtn.addEventListener("click", function () {
        if (!form.reportValidity()) return;
        var payload = contactPayload(form);
        copyText(payload.subject + "\n\n" + payload.text, function () {
          setContactStatus(form, "Copied. Paste into your mail client, or into a GitHub issue.");
        });
      });
    }

    var issueBtn = form.querySelector("[data-contact-issue]");
    if (issueBtn) {
      issueBtn.addEventListener("click", function () {
        if (!form.reportValidity()) return;
        var payload = contactPayload(form);
        if (/security/i.test(payload.topic)) {
          setContactStatus(
            form,
            "Do not file a security report as a public issue. Copy the message and keep it private until security@dbxdb.io has MX."
          );
          return;
        }
        var url =
          "https://github.com/vanshjain-0702/DBX-Database-Extreme/issues/new?title=" +
          encodeURIComponent(payload.subject) +
          "&body=" +
          encodeURIComponent(payload.text);
        window.open(url, "_blank", "noopener");
        setContactStatus(form, "Opened a GitHub issue draft. That is the inbox that delivers today.");
      });
    }
  });

  document.querySelectorAll("[data-copy-email]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var addr = btn.getAttribute("data-copy-email") || "";
      copyText(addr, function () {
        var prev = btn.textContent;
        btn.textContent = "Copied";
        window.setTimeout(function () {
          btn.textContent = prev;
        }, 1400);
      });
    });
  });

  function inspectTenant(id) {
    var pop = document.querySelector("[data-pop]");
    if (!pop || !stores[id]) return;
    var row = stores[id];
    pop.querySelector("[data-pop-title]").textContent = id;
    pop.querySelector("[data-pop-body]").innerHTML =
      '<dl class="pop-kv">' +
      "<div><dt>session</dt><dd>" + (row.session || "empty") + "</dd></div>" +
      "<div><dt>vectors</dt><dd>" + row.vectors + "</dd></div>" +
      "<div><dt>WAL seq</dt><dd>" + row.wal + "</dd></div>" +
      "<div><dt>AUTH</dt><dd>" + (authed === id ? "this engine" : "another engine") + "</dd></div>" +
      "</dl>" +
      "<p>Browser sketch. GET on a neighbour cannot see this directory.</p>";
    if (typeof pop.showModal === "function") pop.showModal();
  }

  var lightbox = document.querySelector("[data-lightbox]");
  if (lightbox) {
    var lightImg = lightbox.querySelector("img");
    var lightCap = lightbox.querySelector("figcaption");
    function openLight(src, cap) {
      if (!lightImg) return;
      lightImg.src = src;
      lightImg.alt = cap || "";
      if (lightCap) lightCap.textContent = cap || "";
      if (typeof lightbox.showModal === "function") lightbox.showModal();
    }
    document.querySelectorAll(".product-shots img, .shot-stack img, .film-stage img").forEach(function (img) {
      img.addEventListener("click", function () {
        var fig = img.closest("figure");
        var cap = fig && fig.querySelector("figcaption") ? fig.querySelector("figcaption").textContent : img.alt;
        openLight(img.currentSrc || img.src, cap);
      });
    });
    lightbox.querySelector(".lightbox-close").addEventListener("click", function () {
      lightbox.close();
    });
    lightbox.addEventListener("click", function (e) {
      if (e.target === lightbox) lightbox.close();
    });
  }

  var pop = document.querySelector("[data-pop]");
  if (pop) {
    pop.querySelector(".pop-close").addEventListener("click", function () {
      pop.close();
    });
    pop.addEventListener("click", function (e) {
      if (e.target === pop) pop.close();
    });
  }

  var tipEl = document.querySelector(".tip");
  function placeTip(el) {
    if (!tipEl) return;
    var text = el.getAttribute("data-tip");
    if (!text) return;
    tipEl.textContent = text;
    tipEl.hidden = false;
    var r = el.getBoundingClientRect();
    var left = Math.min(window.innerWidth - 16, Math.max(16, r.left + r.width / 2));
    tipEl.style.left = left + "px";
    tipEl.style.top = Math.max(12, r.top) + "px";
  }
  function hideTip() {
    if (tipEl) tipEl.hidden = true;
  }
  document.querySelectorAll("[data-tip]").forEach(function (el) {
    el.setAttribute("tabindex", el.getAttribute("tabindex") || "0");
    el.addEventListener("mouseenter", function () { placeTip(el); });
    el.addEventListener("focus", function () { placeTip(el); });
    el.addEventListener("mouseleave", hideTip);
    el.addEventListener("blur", hideTip);
  });

  var live = document.querySelector(".live-meta");
  if (live && !live.getAttribute("data-tip")) {
    live.setAttribute("data-tip", "Operator clock. Public RESP ingress is :6380.");
    live.setAttribute("tabindex", "0");
    live.addEventListener("mouseenter", function () { placeTip(live); });
    live.addEventListener("focus", function () { placeTip(live); });
    live.addEventListener("mouseleave", hideTip);
    live.addEventListener("blur", hideTip);
  }

  var toast = document.querySelector("[data-toast]");
  if (toast && page === "home") {
    var key = "dbx-isolation-toast";
    toast.querySelector("[data-toast-dismiss]").addEventListener("click", function () {
      toast.classList.remove("is-on");
      try { sessionStorage.setItem(key, "1"); } catch (err) { /* private mode */ }
    });
    try {
      if (!sessionStorage.getItem(key)) {
        window.setTimeout(function () {
          toast.classList.add("is-on");
        }, reduce ? 0 : 1600);
      }
    } catch (err) { /* ignore */ }
  }

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") {
      hideTip();
      if (lightbox && lightbox.open) lightbox.close();
      if (pop && pop.open) pop.close();
      closeCmdk();
      return;
    }
    var inField = /^(INPUT|TEXTAREA|SELECT)$/.test((e.target && e.target.tagName) || "");
    if ((e.key === "k" || e.key === "K") && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      openCmdk();
      return;
    }
    if (e.key === "/" && !inField && !e.metaKey && !e.ctrlKey && !e.altKey) {
      e.preventDefault();
      openCmdk();
    }
  });

  /* ——— Command palette ——— */

  var cmdk = document.querySelector("[data-cmdk]");
  var cmdkQ = cmdk ? cmdk.querySelector("[data-cmdk-q]") : null;
  var cmdkList = cmdk ? cmdk.querySelector("[data-cmdk-list]") : null;
  var cmdkIndex = 0;
  var cmdkHits = [];
  var mac = /Mac|iPhone|iPad/.test(navigator.platform || "");
  document.querySelectorAll("[data-cmdk-kbd]").forEach(function (el) {
    el.textContent = mac ? "⌘K" : "Ctrl+K";
  });

  var catalog = [
    { t: "Home", s: "Isolation demo", href: "index.html" },
    { t: "Features", s: "Lifecycle and isolation", href: "features.html" },
    { t: "Architecture", s: "How a request lands", href: "architecture.html" },
    { t: "Get started", s: "Docker, source, Compose", href: "start.html" },
    { t: "Performance", s: "Certified single-node profile", href: "performance.html" },
    { t: "Docs", s: "Thesis, then the ports", href: "docs/index.html" },
    { t: "Quickstart", s: "AUTH and first write", href: "docs/quickstart.html" },
    { t: "Docs · Architecture", s: "Orchestrator and engines", href: "docs/architecture.html" },
    { t: "API", s: "HTTP lifecycle and RESP", href: "docs/api.html" },
    { t: "Positioning", s: "What we sell, what we refuse", href: "docs/positioning.html" },
    { t: "Pricing", s: "BSL 1.1, then talk", href: "pricing.html" },
    { t: "Security", s: "Isolation Kernel", href: "security.html" },
    { t: "Contact", s: "hello@dbxdb.io", href: "contact.html" },
    { t: "Changelog", s: "What shipped", href: "changelog.html" },
    { t: "License", s: "BSL 1.1", href: "license.html" },
    { t: "Isolation demo", s: "On this page", href: "index.html#demo", home: 1 },
    { t: "Why it exists", s: "Shared-cluster pain", href: "index.html#why", home: 1 },
    { t: "Claims", s: "Tenant is the unit", href: "index.html#claims", home: 1 },
    { t: "Operator console", s: "Screens from the binary", href: "index.html#console", home: 1 },
    { t: "Fit", s: "Build this / walk away", href: "index.html#fit", home: 1 },
  ];

  function renderCmdk(q) {
    if (!cmdkList) return;
    var needle = (q || "").trim().toLowerCase();
    cmdkHits = catalog.filter(function (row) {
      if (!needle) return true;
      return (row.t + " " + row.s).toLowerCase().indexOf(needle) !== -1;
    });
    cmdkIndex = 0;
    if (!cmdkHits.length) {
      cmdkList.innerHTML = '<li class="cmdk-empty">No page matches.</li>';
      return;
    }
    cmdkList.innerHTML = cmdkHits
      .map(function (row, n) {
        return (
          '<li><button type="button" class="cmdk-item' +
          (n === 0 ? " is-on" : "") +
          '" data-cmdk-go="' +
          n +
          '"><span>' +
          row.t +
          "</span><em>" +
          row.s +
          "</em></button></li>"
        );
      })
      .join("");
  }

  function paintCmdk() {
    if (!cmdkList) return;
    cmdkList.querySelectorAll(".cmdk-item").forEach(function (btn, n) {
      btn.classList.toggle("is-on", n === cmdkIndex);
    });
    var on = cmdkList.querySelector(".cmdk-item.is-on");
    if (on && on.scrollIntoView) on.scrollIntoView({ block: "nearest" });
  }

  function goCmdk(n) {
    var row = cmdkHits[n];
    if (!row) return;
    closeCmdk();
    window.location.href = joinRoot(root, row.href);
  }

  function openCmdk() {
    if (!cmdk || typeof cmdk.showModal !== "function") return;
    renderCmdk("");
    cmdk.showModal();
    if (cmdkQ) {
      cmdkQ.value = "";
      cmdkQ.focus();
    }
  }

  function closeCmdk() {
    if (cmdk && cmdk.open) cmdk.close();
  }

  document.querySelectorAll("[data-cmdk-open]").forEach(function (btn) {
    btn.addEventListener("click", openCmdk);
  });
  if (cmdk && cmdkQ && cmdkList) {
    cmdkQ.addEventListener("input", function () {
      renderCmdk(cmdkQ.value);
    });
    cmdk.addEventListener("click", function (e) {
      if (e.target === cmdk) closeCmdk();
    });
    cmdkList.addEventListener("click", function (e) {
      var btn = e.target.closest("[data-cmdk-go]");
      if (!btn) return;
      goCmdk(Number(btn.getAttribute("data-cmdk-go")));
    });
    cmdk.addEventListener("keydown", function (e) {
      if (!cmdkHits.length) return;
      if (e.key === "ArrowDown") {
        e.preventDefault();
        cmdkIndex = (cmdkIndex + 1) % cmdkHits.length;
        paintCmdk();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        cmdkIndex = (cmdkIndex - 1 + cmdkHits.length) % cmdkHits.length;
        paintCmdk();
      } else if (e.key === "Enter") {
        e.preventDefault();
        goCmdk(cmdkIndex);
      }
    });
  }

  /* ——— Breadcrumbs ——— */

  var crumbsFor = {
    features: ["Features"],
    architecture: ["Architecture"],
    start: ["Get started"],
    performance: ["Performance"],
    pricing: ["Pricing"],
    security: ["Security"],
    contact: ["Contact"],
    changelog: ["Changelog"],
    license: ["License"],
    privacy: ["Privacy"],
    terms: ["Terms"],
    docs: ["Docs"],
    "docs-quickstart": ["Docs", "Quickstart"],
    "docs-architecture": ["Docs", "Architecture"],
    "docs-api": ["Docs", "API"],
    "docs-positioning": ["Docs", "Positioning"],
  };
  if (crumbsFor[page]) {
    var band = document.querySelector(".page-band .wrap, .docs-body");
    if (band && !band.querySelector(".crumbs")) {
      var trail = document.createElement("nav");
      trail.className = "crumbs";
      trail.setAttribute("aria-label", "Breadcrumb");
      var html = '<a href="' + joinRoot(root, "index.html") + '">Home</a>';
      crumbsFor[page].forEach(function (label, i) {
        html += "<span>/</span>";
        if (i === crumbsFor[page].length - 1) {
          html += "<span>" + label + "</span>";
        } else {
          html += '<a href="' + joinRoot(root, "docs/index.html") + '">' + label + "</a>";
        }
      });
      trail.innerHTML = html;
      band.insertBefore(trail, band.firstChild);
    }
  }

  /* ——— Homepage section dots ——— */

  if (page === "home") {
    var dots = document.createElement("nav");
    dots.className = "sec-dots";
    dots.setAttribute("aria-label", "On this page");
    var secs = [
      ["demo", "Demo"],
      ["why", "Why"],
      ["claims", "Claims"],
      ["console", "Console"],
      ["fit", "Fit"],
      ["walkthrough", "Walkthrough"],
      ["install", "Install"],
    ];
    dots.innerHTML = secs
      .map(function (s) {
        return '<a href="#' + s[0] + '" data-dot="' + s[0] + '" aria-label="' + s[1] + '"></a>';
      })
      .join("");
    document.body.appendChild(dots);
    if ("IntersectionObserver" in window) {
      var spy = new IntersectionObserver(
        function (entries) {
          entries.forEach(function (entry) {
            if (!entry.isIntersecting) return;
            dots.querySelectorAll("a").forEach(function (a) {
              a.classList.toggle("is-on", a.getAttribute("data-dot") === entry.target.id);
            });
          });
        },
        { rootMargin: "-40% 0px -50% 0px", threshold: 0.01 }
      );
      secs.forEach(function (s) {
        var el = document.getElementById(s[0]);
        if (el) spy.observe(el);
      });
    }
  }

  /* ——— Count-up certified numbers ——— */

  function countUp(el) {
    var end = Number(el.getAttribute("data-count") || "0");
    var suffix = el.getAttribute("data-suffix") || "";
    if (reduce) {
      el.textContent = end + suffix;
      return;
    }
    var start = 0;
    var t0 = performance.now();
    function frame(now) {
      var p = Math.min(1, (now - t0) / 900);
      var eased = 1 - Math.pow(1 - p, 3);
      el.textContent = Math.round(start + (end - start) * eased) + suffix;
      if (p < 1) requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
  }
  if ("IntersectionObserver" in window) {
    var cio = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          countUp(entry.target);
          cio.unobserve(entry.target);
        });
      },
      { threshold: 0.4 }
    );
    document.querySelectorAll("[data-count]").forEach(function (el) {
      cio.observe(el);
    });
  } else {
    document.querySelectorAll("[data-count]").forEach(countUp);
  }

  /* ——— Pain list + hop click ——— */

  document.querySelectorAll(".pain-list li").forEach(function (li) {
    li.addEventListener("pointerenter", function () {
      document.querySelectorAll(".pain-list li").forEach(function (n) {
        n.classList.toggle("is-hot", n === li);
      });
    });
  });
  document.querySelectorAll("[data-hop]").forEach(function (rootHop) {
    rootHop.querySelectorAll("[data-hop-stage]").forEach(function (stage, n) {
      stage.style.cursor = "pointer";
      stage.addEventListener("click", function () {
        rootHop.querySelectorAll("[data-hop-stage]").forEach(function (s, i) {
          s.classList.toggle("is-hot", i === n);
        });
      });
    });
  });

  /* ——— Magnetic CTAs + card tilt ——— */

  if (!reduce && window.matchMedia("(pointer:fine)").matches) {
    document.querySelectorAll(".hero .btn, .cta-band .btn").forEach(function (btn) {
      btn.addEventListener("pointermove", function (e) {
        var r = btn.getBoundingClientRect();
        var x = (e.clientX - r.left) / r.width - 0.5;
        var y = (e.clientY - r.top) / r.height - 0.5;
        btn.style.transform = "translate(" + (x * 6).toFixed(1) + "px," + (y * 4).toFixed(1) + "px)";
      });
      btn.addEventListener("pointerleave", function () {
        btn.style.transform = "";
      });
    });
    document.querySelectorAll(".price-card, .usp, .product-shots figure").forEach(function (card) {
      card.addEventListener("pointermove", function (e) {
        var r = card.getBoundingClientRect();
        var x = (e.clientX - r.left) / r.width - 0.5;
        var y = (e.clientY - r.top) / r.height - 0.5;
        card.style.transform =
          "perspective(900px) rotateY(" + (x * 6).toFixed(2) + "deg) rotateX(" + (-y * 5).toFixed(2) + "deg)";
      });
      card.addEventListener("pointerleave", function () {
        card.style.transform = "";
      });
    });
  }

  /* ——— Tabs: arrow keys ——— */

  document.querySelectorAll("[data-tabs]").forEach(function (rootTabs) {
    var tabs = Array.prototype.slice.call(rootTabs.querySelectorAll("[data-tab]"));
    tabs.forEach(function (tab, n) {
      tab.addEventListener("keydown", function (e) {
        if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
        e.preventDefault();
        var next = e.key === "ArrowRight" ? (n + 1) % tabs.length : (n - 1 + tabs.length) % tabs.length;
        tabs[next].click();
        tabs[next].focus();
      });
    });
  });

  /* ——— Film: keys + swipe ——— */

  if (film) {
    var touchX = 0;
    film.addEventListener("keydown", function (e) {
      if (e.key === "ArrowRight") {
        e.preventDefault();
        var nx = film.querySelector("[data-film-next]");
        if (nx) nx.click();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        var pv = film.querySelector("[data-film-prev]");
        if (pv) pv.click();
      }
    });
    film.setAttribute("tabindex", "0");
    film.addEventListener(
      "touchstart",
      function (e) {
        if (e.changedTouches && e.changedTouches[0]) touchX = e.changedTouches[0].clientX;
      },
      { passive: true }
    );
    film.addEventListener("touchend", function (e) {
      if (!e.changedTouches || !e.changedTouches[0]) return;
      var dx = e.changedTouches[0].clientX - touchX;
      if (Math.abs(dx) < 40) return;
      var btn = film.querySelector(dx < 0 ? "[data-film-next]" : "[data-film-prev]");
      if (btn) btn.click();
    });
  }

  /* ——— Docs on-page headings ——— */

  var docsBody = document.querySelector(".docs-body");
  if (docsBody) {
    docsBody.querySelectorAll("h2").forEach(function (h, i) {
      if (!h.id) {
        h.id = (h.textContent || "section")
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, "-")
          .replace(/(^-|-$)/g, "") || "s" + i;
      }
      h.classList.add("has-anchor");
    });
  }

  /* ——— Contact character count ——— */

  document.querySelectorAll("[data-contact]").forEach(function (form) {
    var area = form.querySelector("textarea");
    var chars = form.querySelector("[data-chars]");
    if (!area || !chars) return;
    function tickChars() {
      chars.textContent = area.value.length + " / " + (area.maxLength > 0 ? area.maxLength : 4000);
    }
    area.addEventListener("input", tickChars);
    tickChars();
  });

  /* ——— Prefetch on hover ——— */

  document.querySelectorAll('a[href$=".html"], a[href*=".html#"]').forEach(function (a) {
    var href = a.getAttribute("href");
    if (!href || href.indexOf("http") === 0) return;
    a.addEventListener(
      "pointerenter",
      function () {
        if (document.querySelector('link[rel="prefetch"][href="' + href + '"]')) return;
        var link = document.createElement("link");
        link.rel = "prefetch";
        link.href = href;
        document.head.appendChild(link);
      },
      { once: true }
    );
  });

  /* ——— Hash offset after load ——— */

  if (location.hash) {
    window.setTimeout(function () {
      var el = document.getElementById(location.hash.slice(1));
      if (el) el.scrollIntoView({ block: "start" });
    }, 60);
  }

  document.documentElement.classList.add("is-ready");
})();
