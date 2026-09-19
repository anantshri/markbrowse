/* Client-side table sorting for markbrowse.
 * Attaches click-to-sort behavior to every <th> in tables rendered from
 * markdown (.markdown-body table) and in the directory listing
 * (table.dir-list). Clicking a header toggles ascending/descending order;
 * the active header shows a ▲/▼ indicator.
 *
 * Sorting is value-aware:
 *   - timestamps ("2026-08-02 15:04") and bare dates sort chronologically
 *   - byte sizes with an explicit unit ("42 B", "1.5 KB") sort by actual size
 *   - version-like values ("8.10.0", "v1.2.10m") compare component by
 *     component, so 8.9.0 sorts before 8.10.0
 *   - every other cell with numbers carries the whole value, not just its
 *     first digits: all digit runs are compared in order, so "9.0" no longer
 *     jumps ahead of "8x.0"
 *   - surrounding non-numeric characters are ignored, so "95%", "$1,234" and
 *     "1.2m" sort by their numbers instead of falling back to text
 *   - everything else falls back to a case-insensitive string comparison;
 *     cells whose letters spell a word ("video.mp4", "Section 3") stay textual
 *     so file names and labels are not mistaken for numbers
 *
 * Rows that are parent-navigation links (first cell anchor labelled "../") are
 * kept pinned at the top of the table.
 */
(function () {
  "use strict";

  var SIZE_UNITS = { B: 1, KB: 1024, MB: 1024 * 1024, GB: 1024 * 1024 * 1024, TB: 1024 * 1024 * 1024 * 1024 };

  // Byte size with an explicit unit. The unit is required so that a
  // version-like cell ("8.10.0") is never truncated to its first number by
  // parseFloat, and the numeric part is a single well-formed number.
  var SIZE_WITH_UNIT = /^(\d+(?:\.\d+)?)\s*(B|KB|MB|GB|TB)$/i;

  // Every digit run in the cell, decimals kept intact:
  // "8x.0" -> [8, 0], "1.2m" -> [1.2], "8.10.0" -> [8, 10, 0].
  var NUMBERS = /\d+(?:\.\d+)?/g;

  // Non-numeric residue that may sit next to the digits without turning the
  // cell into text: symbols (%, $, /, -) and at most two letters of unit
  // ("m", "ms", "KB", "px"). Words are rejected, which keeps "video.mp4" and
  // "Section 3" in the string-comparison branch.
  var NUMERIC_NOISE = /^[^a-z]*[a-z]{0,2}[^a-z]*$/i;

  // Version-like value with two or more dots ("8.10.0", "v1.2.10", "8.10.0m").
  var VERSION = /^[^\d]*(\d+(?:\.\d+){2,})[^\d]*$/;

  // Returns { v: [numbers...], s: text, n: true } for cells that carry numbers
  // and { v: lowercaseText, s: lowercaseText, n: false } for everything else.
  // `s` is the tie-breaker when two cells share the same numeric value
  // ("3a" vs "3b"), keeping equal rows in a stable, predictable order.
  function parseSortValue(text) {
    var t = (text || "").trim();
    var s = t.toLowerCase();
    var m;

    // Timestamp "2006-01-02 15:04" (markbrowse directory listings).
    m = t.match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})/);
    if (m) {
      return { v: [Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5])], s: s, n: true };
    }

    // Byte size with unit: compared in bytes, so "900 B" < "1.5 KB".
    m = t.replace(/,/g, "").match(SIZE_WITH_UNIT);
    if (m) {
      return { v: [parseFloat(m[1]) * (SIZE_UNITS[m[2].toUpperCase()] || 1)], s: s, n: true };
    }

    // Version-like values: every component is compared as an integer, so
    // 8.9.0 sorts before 8.10.0 (the full number, not its first digits).
    m = t.replace(/,/g, "").match(VERSION);
    if (m && NUMERIC_NOISE.test(t.replace(/[0-9.,+-]/g, ""))) {
      return { v: m[1].split(".").map(Number), s: s, n: true };
    }

    // Numbers with % / unit / version punctuation attached: compare the digits.
    var nums = t.replace(/,/g, "").match(NUMBERS);
    if (nums && NUMERIC_NOISE.test(t.replace(/[0-9.,+-]/g, ""))) {
      var v = nums.map(Number);
      if (/^-/.test(t)) v[0] = -v[0];
      return { v: v, s: s, n: true };
    }

    return { v: s, s: s, n: false };
  }

  // Compares two parsed values: numeric cells compare component by component
  // ("8.10.0" -> 8, 10, 0), missing components count as 0, numbers sort before
  // text, and text compares case-insensitively.
  function compareValues(a, b) {
    if (a.n && b.n) {
      var len = Math.max(a.v.length, b.v.length);
      for (var i = 0; i < len; i++) {
        var x = i < a.v.length ? a.v[i] : 0;
        var y = i < b.v.length ? b.v[i] : 0;
        if (x !== y) return x < y ? -1 : 1;
      }
      return a.s < b.s ? -1 : a.s > b.s ? 1 : 0;
    }
    if (a.n) return -1;
    if (b.n) return 1;
    return a.v < b.v ? -1 : a.v > b.v ? 1 : 0;
  }

  // A row is the parent-navigation row when its first cell links to the
  // directory above: the template renders it as an anchor labelled "../" whose
  // href is the parent path ("/", "/guides/"). Match the label as well as the
  // legacy "../"-suffixed href so the row stays pinned regardless of which
  // form the server emits.
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
            sortable.push(tr);
          }
        });

        sortable.sort(function (a, b) {
          var ca = a.children[idx];
          var cb = b.children[idx];
          if (!ca || !cb) return 0;
          var cmp = compareValues(parseSortValue(ca.textContent || ""), parseSortValue(cb.textContent || ""));
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
