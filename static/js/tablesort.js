/* Client-side table sorting for markbrowse.
 * Attaches click-to-sort behavior to every <th> in tables rendered from
 * markdown (.markdown-body table) and in the directory listing
 * (table.dir-list). Clicking a header toggles ascending/descending order;
 * the active header shows a ▲/▼ indicator.
 *
 * Sorting is value-aware. A cell is split into alternating number and text
 * segments and the segments are compared in order, so the whole value decides
 * the order rather than the first digits parseFloat happens to find:
 *   - timestamps ("2026-08-02 15:04") and bare dates sort chronologically
 *   - byte sizes with an explicit unit ("42 B", "1.5 KB") sort by actual size
 *   - version-like values with two or more dots compare component by
 *     component, so 8.9.0 sorts before 8.10.0
 *   - digits attached to symbols or units still sort numerically:
 *     "9%" < "95%" < "100%", "$42" < "$1,234", "8x.0" < "9.0"
 *   - letters around the digits still count, so "A1" < "A10" < "B2" and
 *     "Chapter 3" < "Chapter 10" while "notes.md" < "video.mp4"
 *   - a number segment sorts before a text segment, keeping numeric cells
 *     above placeholders such as the "—" in a directory listing
 *
 * Rows that are parent-navigation links (first cell anchor labelled "../") are
 * kept pinned at the top of the table.
 */
(function () {
  "use strict";

  var SIZE_UNITS = { B: 1, KB: 1024, MB: 1024 * 1024, GB: 1024 * 1024 * 1024, TB: 1024 * 1024 * 1024 * 1024 };

  // Byte size with an explicit unit. The unit is required so that a
  // version-like cell ("8.10.0") is never truncated to its first number, and
  // the numeric part is a single well-formed number. A bare "B" additionally
  // needs whitespace in front of it -- the listing always emits "386 B", while
  // "3b" is far more likely to be a label than three bytes.
  var SIZE_WITH_UNIT = /^(\d+(?:\.\d+)?)(?:\s*(KB|MB|GB|TB)|\s+(B))$/i;

  // Version-like value: two or more dots, so the dots cannot all be decimal
  // points ("8.10.0", "v1.2.10", "8.10.0m"). A single dot stays a decimal
  // point, which is why "1.10" still sorts as 1.1 rather than as 1 then 10.
  var VERSION = /^[^\d]*(\d+(?:\.\d+){2,})[^\d]*$/;

  // Alternating runs of digits and non-digits. A dot between digits belongs to
  // the number ("1.2m" -> 1.2, "m"); anything else is text ("8x.0" -> 8, "x.", 0).
  var SEGMENTS = /\d+(?:\.\d+)?|\D+/g;

  // Thousands separators only: a comma between two digits. Commas elsewhere
  // ("Smith, John") are part of the text and must survive.
  var THOUSANDS = /(\d),(?=\d)/g;

  // Splits a cell into comparable segments. Numbers become numbers, text
  // becomes lowercase strings, and a leading minus folds into the first number
  // so "-10" sorts below "-3".
  function segments(t) {
    var negative = /^-\d/.test(t);
    var parts = (negative ? t.slice(1) : t).toLowerCase().match(SEGMENTS);
    if (!parts) return [""]; // empty cell: compare as empty text, not as a number
    var out = parts.map(function (p) {
      return /^\d/.test(p) ? Number(p) : p;
    });
    if (negative) out[0] = -out[0];
    return out;
  }

  // Returns the array of segments a cell is sorted by.
  function parseSortValue(text) {
    var t = (text || "").trim().replace(THOUSANDS, "$1");
    var m;

    // Timestamp "2006-01-02 15:04" (markbrowse directory listings). Bare dates
    // fall through and still sort chronologically as [2026, "-", 5, "-", 30].
    m = t.match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
    if (m) {
      return [Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5])];
    }

    // Byte size with unit: compared in bytes, so "900 B" < "1.5 KB".
    m = t.match(SIZE_WITH_UNIT);
    if (m) {
      return [parseFloat(m[1]) * SIZE_UNITS[(m[2] || m[3]).toUpperCase()]];
    }

    // Version-like values: each component is its own number, so 8.9.0 sorts
    // before 8.10.0 instead of being read as 8.9 against 8.1.
    m = t.match(VERSION);
    if (m) {
      return m[1].split(".").map(Number);
    }

    return segments(t);
  }

  // Compares two segment arrays element by element. Numbers compare
  // numerically, text compares case-insensitively, a number sorts before text,
  // and a value that runs out of segments first sorts above its own extensions
  // ("1" before "1a").
  function compareValues(a, b) {
    var len = Math.max(a.length, b.length);
    for (var i = 0; i < len; i++) {
      if (i >= a.length) return -1;
      if (i >= b.length) return 1;
      var x = a[i];
      var y = b[i];
      var xNum = typeof x === "number";
      var yNum = typeof y === "number";
      if (xNum !== yNum) return xNum ? -1 : 1;
      if (x !== y) return x < y ? -1 : 1;
    }
    return 0;
  }

  // A row is the parent-navigation row when its first cell links to the
  // directory above. The template renders it as an anchor labelled "../" whose
  // href is the parent path ("/", "/guides/"), so match the label as well as
  // the "../"-suffixed href other tables may use.
  function isParentRow(tr) {
    var link = tr.querySelector("td a");
    if (!link) return false;
    if ((link.textContent || "").trim() === "../") return true;
    return /(^|\/)\.\.\/$/.test(link.getAttribute("href") || "");
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
          if (isParentRow(tr)) {
            pinned.push(tr);
          } else {
            // Parse each cell once here rather than on every comparison.
            var cell = tr.children[idx];
            sortable.push({ tr: tr, key: parseSortValue(cell ? cell.textContent : "") });
          }
        });

        sortable.sort(function (a, b) {
          var cmp = compareValues(a.key, b.key);
          return asc ? cmp : -cmp;
        });

        var frag = document.createDocumentFragment();
        pinned.forEach(function (tr) { frag.appendChild(tr); });
        sortable.forEach(function (row) { frag.appendChild(row.tr); });
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
