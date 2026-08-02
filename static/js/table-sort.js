// Sortable tables: click any table header to sort rows by that column.
// Works on every <table> on the page (markdown tables and the directory
// listing). Toggles ascending/descending, sorts numbers numerically, and
// keeps parent rows (e.g. the "../" entry) pinned at the top.
(function () {
  'use strict';

  function cellValue(row, col) {
    var cell = row.children[col];
    return cell ? cell.textContent.trim() : '';
  }

  function compare(a, b) {
    var cleanA = a.replace(/[,%\s]/g, '');
    var cleanB = b.replace(/[,%\s]/g, '');
    var isNumA = /^[+-]?\d+(\.\d+)?$/.test(cleanA);
    var isNumB = /^[+-]?\d+(\.\d+)?$/.test(cleanB);
    if (isNumA && isNumB) {
      return parseFloat(cleanA) - parseFloat(cleanB);
    }
    return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
  }

  document.querySelectorAll('table').forEach(function (table) {
    var head = table.querySelector('thead');
    var body = table.querySelector('tbody');
    if (!head || !body) return;
    var ths = head.querySelectorAll('th');

    ths.forEach(function (th, col) {
      th.style.cursor = 'pointer';
      th.setAttribute('title', 'Sort by this column');
      th.addEventListener('click', function () {
        var asc = th.getAttribute('aria-sort') !== 'ascending';
        ths.forEach(function (h) {
          h.removeAttribute('aria-sort');
          var arrow = h.querySelector('.sort-arrow');
          if (arrow) arrow.remove();
        });
        th.setAttribute('aria-sort', asc ? 'ascending' : 'descending');
        var arrow = document.createElement('span');
        arrow.className = 'sort-arrow';
        arrow.textContent = asc ? ' \u25B2' : ' \u25BC';
        th.appendChild(arrow);

        var rows = Array.prototype.slice.call(body.querySelectorAll('tr'));
        var pinned = rows.filter(function (r) { return r.classList.contains('dir-parent'); });
        var sortable = rows.filter(function (r) { return !r.classList.contains('dir-parent'); });
        sortable.sort(function (a, b) {
          var cmp = compare(cellValue(a, col), cellValue(b, col));
          return asc ? cmp : -cmp;
        });
        pinned.forEach(function (row) { body.appendChild(row); });
        sortable.forEach(function (row) { body.appendChild(row); });
      });
    });
  });
})();
