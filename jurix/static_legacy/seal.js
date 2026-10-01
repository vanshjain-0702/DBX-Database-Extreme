/* ═══════════════════════════════════════════════════════════════════
   Living seal on the load screen + Auditor's Seal celebration.
   Three.js when the network allows, ink rings otherwise.
   ═══════════════════════════════════════════════════════════════════ */
(function () {
  var canvas = document.getElementById("seal-canvas");
  if (!canvas) return;
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var stopped = false;

  function size() {
    var box = canvas.parentElement.getBoundingClientRect();
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.max(1, box.width) * dpr;
    canvas.height = Math.max(1, box.height) * dpr;
    return dpr;
  }

  /* ── 2D ink rings fallback ─────────────────────────────────────── */
  function inkRings() {
    var ctx = canvas.getContext("2d");
    var t = 0;
    function frame() {
      if (stopped) return;
      var dpr = size();
      var w = canvas.width;
      var h = canvas.height;
      ctx.clearRect(0, 0, w, h);
      var cx = w / 2;
      var cy = h * 0.42;
      ctx.save();
      ctx.translate(cx, cy);

      /* Outer ring — champagne gold */
      ctx.lineWidth = 1.4 * dpr;
      ctx.strokeStyle = "rgba(184, 115, 51, 0.6)";
      ctx.beginPath();
      ctx.arc(0, 0, 78 * dpr, t, t + Math.PI * 1.35);
      ctx.stroke();

      /* Inner ring — darker, counter-rotating */
      ctx.strokeStyle = "rgba(196, 165, 116, 0.35)";
      ctx.beginPath();
      ctx.arc(0, 0, 104 * dpr, -t * 0.55, -t * 0.55 + Math.PI * 1.65);
      ctx.stroke();

      /* Decorative tick marks around the seal */
      ctx.strokeStyle = "rgba(196, 165, 116, 0.15)";
      ctx.lineWidth = 0.8 * dpr;
      for (var i = 0; i < 36; i++) {
        var angle = (Math.PI * 2 / 36) * i + t * 0.3;
        var r1 = 55 * dpr;
        var r2 = 60 * dpr;
        ctx.beginPath();
        ctx.moveTo(Math.cos(angle) * r1, Math.sin(angle) * r1);
        ctx.lineTo(Math.cos(angle) * r2, Math.sin(angle) * r2);
        ctx.stroke();
      }

      /* Core dot — copper */
      ctx.fillStyle = "rgba(184, 115, 51, 0.85)";
      ctx.beginPath();
      ctx.arc(0, 0, 3.5 * dpr, 0, Math.PI * 2);
      ctx.fill();

      /* Pulsing glow on the core */
      var pulse = Math.sin(t * 2) * 0.3 + 0.5;
      ctx.fillStyle = "rgba(196, 165, 116, " + (pulse * 0.25) + ")";
      ctx.beginPath();
      ctx.arc(0, 0, 10 * dpr, 0, Math.PI * 2);
      ctx.fill();

      ctx.restore();
      t += reduce ? 0 : 0.01;
      requestAnimationFrame(frame);
    }
    frame();
  }

  /* ── Three.js enhanced seal ────────────────────────────────────── */
  function startThree(THREE) {
    var renderer = new THREE.WebGLRenderer({ canvas: canvas, alpha: true, antialias: true });
    renderer.setClearColor(0x000000, 0);
    var scene = new THREE.Scene();
    var camera = new THREE.PerspectiveCamera(32, 1, 0.1, 30);
    camera.position.set(0, 0.15, 5.4);

    /* Materials — copper/brass legal seal palette */
    var brass = new THREE.MeshStandardMaterial({ color: 0xB87333, metalness: 0.85, roughness: 0.2 });
    var ink = new THREE.MeshStandardMaterial({ color: 0x3a3128, metalness: 0.35, roughness: 0.5 });
    var gold = new THREE.MeshStandardMaterial({ color: 0xC4A574, metalness: 0.75, roughness: 0.3 });

    var ring = new THREE.Mesh(new THREE.TorusGeometry(1.05, 0.02, 24, 140), brass);
    var outer = new THREE.Mesh(new THREE.TorusGeometry(1.38, 0.008, 16, 120), ink);
    outer.rotation.x = 0.55;
    var inner = new THREE.Mesh(new THREE.TorusGeometry(0.72, 0.006, 16, 100), gold);
    inner.rotation.x = -0.3;
    var core = new THREE.Mesh(new THREE.SphereGeometry(0.05, 24, 16), brass);

    scene.add(ring, outer, inner, core);

    /* Lighting — warm, dramatic */
    var key = new THREE.DirectionalLight(0xfff6e8, 2.6);
    key.position.set(2.2, 2.4, 3);
    var fill = new THREE.DirectionalLight(0xB87333, 0.6);
    fill.position.set(-2, -1, 2);
    scene.add(key, fill, new THREE.AmbientLight(0xf4efe6, 0.5));

    function resize() {
      var box = canvas.parentElement.getBoundingClientRect();
      renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
      renderer.setSize(box.width, box.height, false);
      camera.aspect = box.width / Math.max(box.height, 1);
      camera.updateProjectionMatrix();
    }
    resize();
    window.addEventListener("resize", resize);

    function frame() {
      if (stopped) return;
      if (!reduce) {
        ring.rotation.z += 0.004;
        ring.rotation.x = Math.sin(ring.rotation.z * 0.7) * 0.35;
        outer.rotation.z -= 0.003;
        inner.rotation.z += 0.006;
        inner.rotation.y = Math.sin(ring.rotation.z * 0.5) * 0.2;
      }
      renderer.render(scene, camera);
      requestAnimationFrame(frame);
    }
    frame();
  }

  import("https://unpkg.com/three@0.170.0/build/three.module.js")
    .then(startThree)
    .catch(inkRings);

  /* ═══════════════════════════════════════════════════════════════
     Seal Stamp Celebration
     Called when an audit returns all-clear to give the user
     a satisfying "approved" branding moment.
     ═══════════════════════════════════════════════════════════════ */
  window.JurixSealStamp = {
    /**
     * Show the seal stamp celebration overlay.
     * @param {object} opts
     * @param {string} opts.text - Main text in the stamp (e.g., "CLEARED")
     * @param {string} opts.sub  - Subtitle (e.g., "All items passed")
     * @param {number} opts.duration - How long to show (ms, default 2200)
     */
    celebrate: function (opts) {
      opts = opts || {};
      var text = opts.text || "CLEARED";
      var sub = opts.sub || "Audit complete";
      var duration = opts.duration || 2200;

      /* Create overlay if not present */
      var overlay = document.querySelector(".seal-stamp-overlay");
      if (!overlay) {
        overlay = document.createElement("div");
        overlay.className = "seal-stamp-overlay";
        overlay.innerHTML =
          '<div class="seal-stamp">' +
            '<div class="seal-stamp-ring"></div>' +
            '<div class="seal-stamp-text">' +
              '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" class="seal-check" style="margin:0 auto 4px;display:block">' +
                '<path d="M5 13l4 4L19 7" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="stroke-dasharray:28;stroke-dashoffset:28"/>' +
              '</svg>' +
              '<span class="stamp-main"></span><br>' +
              '<span class="stamp-sub" style="font-size:9px;letter-spacing:0.2em;opacity:0.7"></span>' +
            '</div>' +
          '</div>';
        document.body.appendChild(overlay);
      }

      overlay.querySelector(".stamp-main").textContent = text;
      overlay.querySelector(".stamp-sub").textContent = sub;

      /* Trigger animation */
      overlay.classList.remove("celebrating", "active");
      void overlay.offsetWidth; /* Force reflow */
      overlay.classList.add("active", "celebrating");

      /* Auto-dismiss */
      setTimeout(function () {
        overlay.classList.remove("celebrating");
        setTimeout(function () {
          overlay.classList.remove("active");
        }, 400);
      }, duration);
    }
  };
})();
