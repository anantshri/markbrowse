/* Sidebar file tree for markbrowse.
 *
 * Fetches /__mdview/tree.json once and renders it into #tree-root, with a
 * quick filter above the listing.
 *
 * Rendering is lazy. The previous version walked the whole tree on load and
 * built a DOM element for every folder and every file, collapsed or not: on a
 * 4,800-file vault that is ~10,900 elements constructed on every single page
 * navigation. Here a folder's children are built the first time it opens, so
 * the initial render costs one element per top-level entry. The parsed tree
 * stays in memory, which is what lets the filter search the whole vault
 * without asking the server for anything.
 */
(function () {
  "use strict";

  // Matches beyond this are not rendered. A one-character query on a large
  // vault matches thousands of files, and drawing them all would reintroduce
  // exactly the stall this file exists to avoid.
  var MAX_RESULTS = 200;

  // Long enough that typing a word does not render once per keystroke, short
  // enough to feel immediate.
  var FILTER_DEBOUNCE_MS = 120;

  var currentPath = document.body.dataset.currentPath || "/";
  var container = document.getElementById("tree-root");
  if (!container) return;

  var filterInput = document.getElementById("tree-filter");
  var statusEl = document.getElementById("tree-status");
  var tree = null;

  fetch("/__mdview/tree.json")
    .then(function (r) { return r.json(); })
    .then(function (data) {
      tree = data;
      renderTree();
      if (filterInput) filterInput.disabled = false;
    })
    .catch(function () {
      setStatus("Could not load the file tree.");
    });

  // ---------------------------------------------------------------- helpers

  function setStatus(text) {
    if (!statusEl) return;
    statusEl.textContent = text || "";
    statusEl.style.display = text ? "block" : "none";
  }

  function indent(el, depth) {
    el.style.paddingLeft = (12 + depth * 16) + "px";
  }

  // Builds the <a> for a file. `match` is an optional [start, end) range in
  // the name to mark as the filter hit.
  function fileRow(node, depth, match) {
    var wrap = document.createElement("div");
    wrap.className = "tree-item";

    var link = document.createElement("a");
    link.className = "tree-file";
    link.href = node.path;
    indent(link, depth);

    if (match) {
      // Built from text nodes, never innerHTML: the highlighted span is
      // derived from what the user typed.
      link.appendChild(document.createTextNode(node.name.slice(0, match[0])));
      var mark = document.createElement("span");
      mark.className = "tree-match";
      mark.textContent = node.name.slice(match[0], match[1]);
      link.appendChild(mark);
      link.appendChild(document.createTextNode(node.name.slice(match[1])));
    } else {
      link.textContent = node.name;
    }

    if (node.path === currentPath) link.classList.add("active");
    wrap.appendChild(link);
    return wrap;
  }

  // Builds a folder row. children are produced by build(childrenEl) the first
  // time the folder opens, or immediately when `open` is true.
  function folderRow(node, depth, open, build) {
    var wrap = document.createElement("div");
    wrap.className = "tree-item";

    var btn = document.createElement("button");
    btn.className = "tree-folder";
    btn.type = "button";
    btn.textContent = node.name;
    btn.setAttribute("aria-expanded", open ? "true" : "false");
    indent(btn, depth);

    var kids = document.createElement("div");
    kids.className = "tree-children";

    var built = false;
    function populate() {
      if (built) return;
      built = true;
      build(kids);
    }

    if (open) {
      populate();
    } else {
      btn.classList.add("collapsed");
      kids.classList.add("collapsed");
    }

    btn.addEventListener("click", function () {
      var nowOpen = btn.classList.contains("collapsed");
      if (nowOpen) populate();
      btn.classList.toggle("collapsed");
      kids.classList.toggle("collapsed");
      btn.setAttribute("aria-expanded", nowOpen ? "true" : "false");
    });

    wrap.appendChild(btn);
    wrap.appendChild(kids);
    return wrap;
  }

  // Children arrive already sorted (directories first, then by name) from the
  // server's sortTree, so there is nothing to sort here.
  function childrenOf(node) {
    return node.children || [];
  }

  // ------------------------------------------------------------- full tree

  // True when node is, or contains, the page currently being viewed. Used to
  // decide which folders to open on load — the only branch rendered eagerly.
  function containsCurrent(node) {
    if (!node.isDir) return node.path === currentPath;
    // A directory path is a prefix of everything beneath it.
    return currentPath.indexOf(node.path) === 0;
  }

  function renderInto(parentEl, nodes, depth) {
    var frag = document.createDocumentFragment();
    nodes.forEach(function (node) {
      frag.appendChild(renderNode(node, depth));
    });
    parentEl.appendChild(frag);
  }

  function renderNode(node, depth) {
    if (!node.isDir) return fileRow(node, depth, null);
    return folderRow(node, depth, containsCurrent(node), function (kids) {
      renderInto(kids, childrenOf(node), depth + 1);
    });
  }

  function renderTree() {
    container.textContent = "";
    setStatus("");
    if (!tree) return;
    renderInto(container, childrenOf(tree), 0);
    var active = container.querySelector(".tree-file.active");
    if (active) active.scrollIntoView({ block: "nearest" });
  }

  // ---------------------------------------------------------------- filter

  // Collects files matching the query. A query containing "/" is matched
  // against the whole path so "guides/int" works; otherwise only the file name
  // is considered, which keeps a short query from matching every file by way
  // of a directory name high up the tree.
  function search(query) {
    var q = query.toLowerCase();
    var byPath = q.indexOf("/") !== -1;
    var results = [];
    var total = 0;

    (function walk(node, trail) {
      var children = childrenOf(node);
      for (var i = 0; i < children.length; i++) {
        var child = children[i];
        if (child.isDir) {
          walk(child, trail.concat(child));
          continue;
        }
        var hay = (byPath ? child.path : child.name).toLowerCase();
        var at = hay.indexOf(q);
        if (at === -1) continue;
        total++;
        if (results.length < MAX_RESULTS) {
          // Highlight the hit in the name. For a path match the offset is
          // relative to the path, so only mark it when it lands in the name.
          var nameAt = byPath ? child.name.toLowerCase().indexOf(q) : at;
          results.push({
            node: child,
            trail: trail,
            match: nameAt === -1 ? null : [nameAt, nameAt + q.length]
          });
        }
      }
    })(tree, []);

    return { results: results, total: total };
  }

  // Renders matches as a flat list, each with the folders it sits under, so a
  // file found "deep down" still says where it lives.
  function renderResults(found) {
    container.textContent = "";

    if (!found.results.length) {
      setStatus("No matching files.");
      return;
    }

    setStatus(found.total > found.results.length
      ? "Showing " + found.results.length + " of " + found.total + " matches."
      : found.total + (found.total === 1 ? " match." : " matches."));

    var frag = document.createDocumentFragment();
    found.results.forEach(function (hit) {
      var wrap = document.createElement("div");
      wrap.className = "tree-item tree-result";

      if (hit.trail.length) {
        var crumb = document.createElement("div");
        crumb.className = "tree-result-path";
        crumb.textContent = hit.trail.map(function (d) { return d.name; }).join(" / ");
        indent(crumb, 0);
        wrap.appendChild(crumb);
      }

      wrap.appendChild(fileRow(hit.node, 0, hit.match).firstChild);
      frag.appendChild(wrap);
    });
    container.appendChild(frag);
  }

  function applyFilter() {
    if (!tree) return;
    var q = filterInput.value.trim();
    if (!q) {
      renderTree();
      return;
    }
    renderResults(search(q));
  }

  if (filterInput) {
    filterInput.disabled = true; // enabled once the tree has loaded
    var timer = null;
    filterInput.addEventListener("input", function () {
      if (timer) clearTimeout(timer);
      timer = setTimeout(applyFilter, FILTER_DEBOUNCE_MS);
    });
    filterInput.addEventListener("keydown", function (e) {
      if (e.key !== "Escape") return;
      if (timer) clearTimeout(timer);
      filterInput.value = "";
      applyFilter();
    });
  }

  // ------------------------------------------------------- sidebar toggles

  var toggle = document.getElementById("sidebar-toggle");
  if (toggle) {
    toggle.onclick = function () {
      var sb = document.querySelector(".mdview-sidebar");
      if (window.innerWidth <= 767) {
        sb.classList.toggle("sidebar-open");
        var ov = document.querySelector(".sidebar-overlay");
        if (ov) ov.classList.toggle("visible");
      } else {
        document.body.classList.toggle("sidebar-collapsed");
      }
    };
  }

  var overlay = document.querySelector(".sidebar-overlay");
  if (overlay) {
    overlay.onclick = function () {
      document.querySelector(".mdview-sidebar").classList.remove("sidebar-open");
      overlay.classList.remove("visible");
    };
  }
})();
