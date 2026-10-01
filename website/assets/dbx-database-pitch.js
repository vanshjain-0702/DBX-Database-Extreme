(function () {
  var slides = Array.prototype.slice.call(document.querySelectorAll(".slide"));
  var notesEl = document.querySelector(".notes");
  var counter = document.querySelector("[data-counter]");
  var progress = document.querySelector("[data-progress]");
  var index = 0;

  function go(next) {
    index = Math.max(0, Math.min(slides.length - 1, next));
    slides.forEach(function (slide, i) { slide.classList.toggle("is-on", i === index); });
    if (counter) counter.textContent = (index + 1) + " / " + slides.length;
    if (progress) progress.style.width = ((index + 1) / slides.length) * 100 + "%";
    if (notesEl) notesEl.textContent = slides[index].getAttribute("data-notes") || "";
    history.replaceState(null, "", "#s" + (index + 1));
  }

  function fromHash() {
    var match = (location.hash || "").match(/s(\d+)/i);
    return match ? parseInt(match[1], 10) - 1 : 0;
  }

  if (/(?:^|[?&])print=1(?:&|$)/.test(location.search)) document.documentElement.classList.add("print-all");
  document.querySelectorAll("[data-next]").forEach(function (button) { button.addEventListener("click", function () { go(index + 1); }); });
  document.querySelectorAll("[data-prev]").forEach(function (button) { button.addEventListener("click", function () { go(index - 1); }); });
  document.querySelectorAll("[data-notes-toggle]").forEach(function (button) { button.addEventListener("click", function () { document.body.classList.toggle("show-notes"); }); });
  document.querySelectorAll("[data-full]").forEach(function (button) {
    button.addEventListener("click", function () {
      if (!document.fullscreenElement) document.documentElement.requestFullscreen().catch(function () {});
      else document.exitFullscreen().catch(function () {});
    });
  });
  document.querySelectorAll("[data-print]").forEach(function (button) {
    button.addEventListener("click", function () {
      if (!/(?:^|[?&])print=1(?:&|$)/.test(location.search)) location.href = "dbx-database-pitch.html?print=1";
      else window.print();
    });
  });
  window.addEventListener("keydown", function (event) {
    if (["ArrowRight", "PageDown", " ", "Enter"].indexOf(event.key) !== -1) { event.preventDefault(); go(index + 1); }
    else if (["ArrowLeft", "PageUp", "Backspace"].indexOf(event.key) !== -1) { event.preventDefault(); go(index - 1); }
    else if (event.key === "Home") go(0);
    else if (event.key === "End") go(slides.length - 1);
    else if (event.key === "n" || event.key === "N") document.body.classList.toggle("show-notes");
    else if (event.key === "f" || event.key === "F") document.querySelector("[data-full]").click();
    else if ((event.key === "p" || event.key === "P") && !event.ctrlKey && !event.metaKey) window.print();
  });
  document.querySelector(".deck").addEventListener("click", function (event) {
    if (!event.target.closest("a, button")) go(index + 1);
  });
  window.addEventListener("hashchange", function () { go(fromHash()); });
  go(fromHash());
})();
