(function () {
  var slides = Array.prototype.slice.call(document.querySelectorAll(".slide"));
  var notesEl = document.querySelector(".notes");
  var counter = document.querySelector("[data-counter]");
  var progress = document.querySelector("[data-progress]");
  var i = 0;

  function go(n) {
    i = Math.max(0, Math.min(slides.length - 1, n));
    slides.forEach(function (s, idx) {
      s.classList.toggle("is-on", idx === i);
    });
    if (counter) counter.textContent = i + 1 + " / " + slides.length;
    if (progress) progress.style.width = ((i + 1) / slides.length) * 100 + "%";
    if (notesEl) notesEl.textContent = slides[i].getAttribute("data-notes") || "";
    history.replaceState(null, "", "#s" + (i + 1));
  }

  function fromHash() {
    var m = (location.hash || "").match(/s(\d+)/i);
    return m ? parseInt(m[1], 10) - 1 : 0;
  }

  if (/(?:^|[?&])print=1(?:&|$)/.test(location.search)) {
    document.documentElement.classList.add("print-all");
  }

  document.querySelectorAll("[data-next]").forEach(function (b) {
    b.addEventListener("click", function () { go(i + 1); });
  });
  document.querySelectorAll("[data-prev]").forEach(function (b) {
    b.addEventListener("click", function () { go(i - 1); });
  });
  document.querySelectorAll("[data-notes-toggle]").forEach(function (b) {
    b.addEventListener("click", function () {
      document.body.classList.toggle("show-notes");
    });
  });
  function toggleFull() {
    if (!document.fullscreenElement) {
      document.documentElement.requestFullscreen().catch(function () {});
    } else {
      document.exitFullscreen().catch(function () {});
    }
  }

  document.querySelectorAll("[data-print]").forEach(function (b) {
    b.addEventListener("click", function () {
      if (!/(?:^|[?&])print=1(?:&|$)/.test(location.search)) {
        location.href = "pitch.html?print=1";
        return;
      }
      window.print();
    });
  });
  document.querySelectorAll("[data-full]").forEach(function (b) {
    b.addEventListener("click", function () { toggleFull(); });
  });

  window.addEventListener("keydown", function (e) {
    if (e.key === "ArrowRight" || e.key === "PageDown" || e.key === " " || e.key === "Enter") {
      e.preventDefault();
      go(i + 1);
    } else if (e.key === "ArrowLeft" || e.key === "PageUp" || e.key === "Backspace") {
      e.preventDefault();
      go(i - 1);
    } else if (e.key === "Home") {
      go(0);
    } else if (e.key === "End") {
      go(slides.length - 1);
    } else if (e.key === "n" || e.key === "N") {
      document.body.classList.toggle("show-notes");
    } else if (e.key === "f" || e.key === "F") {
      e.preventDefault();
      toggleFull();
    } else if (e.key === "p" || e.key === "P") {
      if (!e.metaKey && !e.ctrlKey) {
        e.preventDefault();
        window.print();
      }
    }
  });

  document.querySelector(".deck").addEventListener("click", function (e) {
    if (e.target.closest("a, button")) return;
    go(i + 1);
  });

  window.addEventListener("hashchange", function () {
    go(fromHash());
  });

  go(fromHash());
})();
