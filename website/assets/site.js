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
    var root = document.querySelector("[data-bench]");
    if (root) root.style.setProperty("--tenant", "var(--" + authed + ")");
  }

  function runScript() {
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
    typeCommand(step.cmd, function () {
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
        var out = document.querySelector("[data-term]");
        if (out) out.innerHTML = "";
        paintCabinets();
        runScript();
      });
    }
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

  /* ——— Contact: compose mailto, no third-party form host ——— */

  document.querySelectorAll("[data-contact]").forEach(function (form) {
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      var name = (form.querySelector('[name="name"]') || {}).value || "";
      var email = (form.querySelector('[name="email"]') || {}).value || "";
      var topic = (form.querySelector('[name="topic"]') || {}).value || "Contact";
      var message = (form.querySelector('[name="message"]') || {}).value || "";
      var subject = encodeURIComponent("DBX — " + topic);
      var body = encodeURIComponent(
        "Name: " + name + "\nEmail: " + email + "\nTopic: " + topic + "\n\n" + message
      );
      var href = "mailto:hello@dbxdb.io?subject=" + subject + "&body=" + body;
      var note = form.querySelector("[data-contact-status]");
      if (note) {
        note.hidden = false;
        note.textContent =
          "Opening your mail client to hello@dbxdb.io. If nothing opens, write us directly.";
      }
      window.location.href = href;
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
    if (e.key !== "Escape") return;
    hideTip();
    if (lightbox && lightbox.open) lightbox.close();
    if (pop && pop.open) pop.close();
  });
})();
