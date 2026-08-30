/* Right-hand table of contents for markbrowse.
 * Scans article.markdown-body h1..h6 (each carries an id from
 * parser.WithAutoHeadingID) and builds a nested, collapsible outline on the
 * right side of the page. Levels h1/h2 start expanded, deeper levels start
 * collapsed; clicking an entry smooth-scrolls to the heading; the section
 * currently in view is highlighted via IntersectionObserver.
 */
(function () {
  "use strict";

  var container = document.getElementById("toc-root");
  if (!container) return;

  var toggleBtn = document.getElementById("toc-toggle");
  if (toggleBtn) {
    toggleBtn.onclick = function () {
      var collapsed = document.body.classList.toggle("toc-collapsed");
      toggleBtn.textContent = collapsed ? "«" : "»"; // « / »
    };
  }

  var headings = document.querySelectorAll(".markdown-body h1, .markdown-body h2, .markdown-body h3, .markdown-body h4, .markdown-body h5, .markdown-body h6");
  if (!headings.length) {
    var toc = container.closest(".mdview-toc");
    if (toc) toc.style.display = "none";
    if (toggleBtn) toggleBtn.style.display = "none";
    return;
  }

  var links = [];

  function build() {
    // Stack of current open <details> elements per heading level.
    var stack = [];
    var root = document.createElement("div");
    root.className = "toc-tree";
    var current = root;

    Array.prototype.forEach.call(headings, function (h) {
      var level = parseInt(h.tagName.charAt(1), 10);
      var id = h.getAttribute("id");
      if (!id) return;
      var text = h.textContent.trim();

      var details = document.createElement("details");
      details.className = "toc-item toc-l" + level;
      // Expand the top two outline levels, collapse deeper ones.
      details.open = level <= 2;

      var summary = document.createElement("summary");
      var a = document.createElement("a");
      a.href = "#" + id;
      a.textContent = text;
      summary.appendChild(a);
      details.appendChild(summary);

      // Pop back to the parent whose level is below ours.
      while (stack.length && stack[stack.length - 1].level >= level) {
        stack.pop();
      }
      if (stack.length) {
        stack[stack.length - 1].node.appendChild(details);
      } else {
        root.appendChild(details);
      }
      stack.push({ level: level, node: details });
      links.push({ anchor: a, heading: h });
    });

    container.appendChild(root);
  }

  function highlight() {
    if (!("IntersectionObserver" in window)) return;
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        links.forEach(function (l) { l.anchor.classList.remove("active"); });
        var match = links.filter(function (l) { return l.heading === entry.target; })[0];
        if (match) match.anchor.classList.add("active");
      });
    }, { rootMargin: "0px 0px -70% 0px" });
    links.forEach(function (l) { io.observe(l.heading); });
  }

  build();
  highlight();
})();
