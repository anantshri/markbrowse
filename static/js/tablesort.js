/* Client-side table sorting for markbrowse.
 * Attaches click-to-sort behavior to every <th> in tables rendered from
 * markdown (.markdown-body table) and in the directory listing
 * (table.dir-list). Clicking a header toggles ascending/descending order;
 * the active header shows a ▲/▼ indicator.
 *
 * Sorting is value-aware:
 *   - plain numbers and comma-separated numbers ("1,234") sort numerically
 *   - byte sizes with units ("1.5 KB", "42 B") sort by actual size
 *   - timestamps ("2026-08-02 15:04") sort chronologically
 *   - everything else falls back to a case-insensitive string comparison
 *
 * Rows that are parent-navigation links (first cell anchor href ending in
 * "../") are kept pinned at the top of the table.
 */
(function () {
  "use strict";

  var SIZE_UNITS = { B: 1, KB: 1024, MB: 1024 * 1024, GB: 1024 * 1024 * 1024, TB: 1024 * 1024 * 1024 * 1024 };

  // Returns { v: sortValue, n: isNumeric } for a cell's text content.
  function parseSortValue(text) {
    var t = text.trim();
    var m;

    // Timestamp "2006-01-02 15:04" (markbrowse directory listings).
    m = t.match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
    if (m) {
      return { v: Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5]), n: true };
    }

    // Byte size with unit: "1.5 KB", "42 B".
    m = t.match(/^([\d.]+)\s*(B|KB|MB|GB|TB)?$/i);
    if (m) {
      var mult = SIZE_UNITS[(m[2] || "B").toUpperCase()] || 1;
      return { v: parseFloat(m[1]) * mult, n: true };
    }

    // Plain number, possibly with thousands separators.
    m = t.replace(/,/g, "").match(/^-?[\d.]+$/);
    if (m) {
      return { v: parseFloat(m[0]), n: true };
    }

    return { v: t.toLowerCase(), n: false };
  }

  function makeSortable(table) {
    var thead = table.querySelector("thead");
    if (!thead) return;
    var ths = Array.prototype.slice.call(thead.querySelectorAll("th"));
    if (!ths.length) return;
    var tbody = table.querySelector("tbody");
    if (!tbody) return;

    ths.forEach(function (th, idx) {
      th.style.cursor = "pointer";
      th.title = "Click to sort";
      th.addEventListener("click", function () {
        var asc = th.getAttribute("data-sort") !== "asc";

        // Clear state and indicators from every header in this table.
        ths.forEach(function (other) {
          other.removeAttribute("data-sort");
          var ind = other.querySelector(".sort-ind");
          if (ind) ind.remove();
        });
        th.setAttribute("data-sort", asc ? "asc" : "desc");
        var ind = document.createElement("span");
        ind.className = "sort-ind";
        ind.textContent = asc ? " \u25B2" : " \u25BC"; // ▲ / ▼
        th.appendChild(ind);

        var pinned = [];
        var sortable = [];
        Array.prototype.slice.call(tbody.querySelectorAll("tr")).forEach(function (tr) {
          var firstLink = tr.querySelector("td a");
          if (firstLink && /\.\.\/$/.test(firstLink.getAttribute("href") || "")) {
            pinned.push(tr);
          } else {
            sortable.push(tr);
          }
        });

        sortable.sort(function (a, b) {
          var ca = a.children[idx];
          var cb = b.children[idx];
          if (!ca || !cb) return 0;
          var va = parseSortValue(ca.textContent || "");
          var vb = parseSortValue(cb.textContent || "");
          var cmp;
          if (va.n && vb.n) {
            cmp = va.v - vb.v;
          } else if (va.n) {
            cmp = -1;
          } else if (vb.n) {
            cmp = 1;
          } else {
            cmp = va.v < vb.v ? -1 : va.v > vb.v ? 1 : 0;
          }
          return asc ? cmp : -cmp;
        });

        var frag = document.createDocumentFragment();
        pinned.forEach(function (tr) { frag.appendChild(tr); });
        sortable.forEach(function (tr) { frag.appendChild(tr); });
        tbody.appendChild(frag);
      });
    });
  }

  function init() {
    var tables = document.querySelectorAll(".markdown-body table, table.dir-list");
    Array.prototype.forEach.call(tables, makeSortable);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
