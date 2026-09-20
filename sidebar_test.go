package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// sidebar.js is a browser script: unlike tablesort.js it cannot simply be
// unwrapped, because its IIFE returns early when #tree-root is missing and a
// bare `return` is invalid at the top level of a script. So these tests give it
// enough of a DOM to run for real, then drive it the way a user does — type in
// the filter, fire the event, inspect what was rendered. That exercises the
// wiring (debounce, listeners, lazy expansion) as well as the logic.
const sidebarDOM = `
var __timers = [];
function setTimeout(fn) { __timers.push(fn); return __timers.length; }
function clearTimeout(id) { if (id) __timers[id - 1] = null; }
function flushTimers() {
  var due = __timers; __timers = [];
  due.forEach(function (fn) { if (fn) fn(); });
}

function El(tag) {
  this.tagName = tag;
  this.className = "";
  this.style = {};
  this.dataset = {};
  this.childNodes = [];
  this.attrs = {};
  this.listeners = {};
  this.value = "";
  this.disabled = false;
}
El.prototype.appendChild = function (child) {
  if (child && child.__fragment) {
    child.childNodes.forEach(function (c) { this.childNodes.push(c); }, this);
    child.childNodes = [];
  } else {
    this.childNodes.push(child);
  }
  return child;
};
El.prototype.setAttribute = function (k, v) { this.attrs[k] = v; };
El.prototype.getAttribute = function (k) { return this.attrs[k]; };
El.prototype.addEventListener = function (type, fn) {
  (this.listeners[type] = this.listeners[type] || []).push(fn);
};
El.prototype.scrollIntoView = function () { this.scrolled = true; };
Object.defineProperty(El.prototype, "firstChild", {
  get: function () { return this.childNodes[0] || null; }
});
Object.defineProperty(El.prototype, "textContent", {
  get: function () {
    return this.childNodes.map(function (c) { return c.textContent; }).join("");
  },
  set: function (v) {
    this.childNodes = [];
    if (v !== "") this.childNodes.push({ textContent: String(v), childNodes: [] });
  }
});
Object.defineProperty(El.prototype, "classList", {
  get: function () {
    var el = this;
    function parts() { return el.className.split(" ").filter(Boolean); }
    return {
      contains: function (c) { return parts().indexOf(c) !== -1; },
      add: function (c) { if (parts().indexOf(c) === -1) el.className = parts().concat(c).join(" "); },
      remove: function (c) { el.className = parts().filter(function (p) { return p !== c; }).join(" "); },
      toggle: function (c) { if (parts().indexOf(c) === -1) { this.add(c); } else { this.remove(c); } }
    };
  }
});

// Matches only the ".a.b" selector forms this script uses.
function matches(el, sel) {
  return sel.split(".").filter(Boolean).every(function (c) {
    return (" " + el.className + " ").indexOf(" " + c + " ") !== -1;
  });
}
El.prototype.querySelector = function (sel) {
  for (var i = 0; i < this.childNodes.length; i++) {
    var c = this.childNodes[i];
    if (!c.className && c.className !== "") continue;
    if (matches(c, sel)) return c;
    if (c.querySelector) { var deep = c.querySelector(sel); if (deep) return deep; }
  }
  return null;
};

var byId = {
  "tree-root": new El("div"),
  "tree-filter": new El("input"),
  "tree-status": new El("div"),
  "sidebar-toggle": null
};

var document = {
  body: { dataset: { currentPath: CURRENT_PATH } },
  getElementById: function (id) { return byId[id] || null; },
  querySelector: function () { return null; },
  createElement: function (tag) { return new El(tag); },
  createTextNode: function (t) { return { textContent: String(t), childNodes: [] }; },
  createDocumentFragment: function () { var f = new El("#fragment"); f.__fragment = true; return f; }
};

// Synchronous stand-in for fetch: goja has promises, but resolving them needs
// the job queue pumped, and nothing here is actually async.
function settled(value) {
  return {
    then: function (fn) { return settled(fn(value)); },
    catch: function () { return this; }
  };
}
function fetch() {
  return settled({ json: function () { return JSON.parse(TREE_JSON); } });
}

function fire(el, type) {
  (el.listeners[type] || []).forEach(function (fn) { fn({ key: EVENT_KEY }); });
}

// --- assertions reach in through these -------------------------------------
function rootChildren() { return byId["tree-root"].childNodes; }
function status() { return byId["tree-status"].textContent; }
function setFilter(v) { byId["tree-filter"].value = v; fire(byId["tree-filter"], "input"); flushTimers(); }
function pressEscape() { fire(byId["tree-filter"], "keydown"); flushTimers(); }

// Flattens rendered rows to "className|text" so tests can assert on them.
function rendered() {
  var out = [];
  (function walk(nodes) {
    nodes.forEach(function (n) {
      if (n.className) out.push(n.className + "|" + n.textContent);
      if (n.childNodes && n.childNodes.length) walk(n.childNodes);
    });
  })(rootChildren());
  return out;
}

// Counts .tree-children elements that have been populated, which is what
// "lazy" means here.
function populatedFolders() {
  var n = 0;
  (function walk(nodes) {
    nodes.forEach(function (c) {
      if (c.className && c.className.indexOf("tree-children") !== -1 && c.childNodes.length) n++;
      if (c.childNodes && c.childNodes.length) walk(c.childNodes);
    });
  })(rootChildren());
  return n;
}

function clickFolder(name) {
  var found = null;
  (function walk(nodes) {
    nodes.forEach(function (c) {
      if (!found && c.className && c.className.indexOf("tree-folder") !== -1 && c.textContent === name) found = c;
      if (c.childNodes && c.childNodes.length) walk(c.childNodes);
    });
  })(rootChildren());
  if (!found) throw new Error("no folder named " + name);
  fire(found, "click");
  return found;
}
`

// sidebarVM boots sidebar.js against the stub DOM with the given tree and
// current path.
func sidebarVM(t *testing.T, tree any, currentPath string) *goja.Runtime {
	t.Helper()

	raw, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}

	vm := goja.New()
	prelude := fmt.Sprintf("var TREE_JSON = %s;\nvar CURRENT_PATH = %s;\nvar EVENT_KEY = \"Escape\";\n",
		mustJSON(t, string(raw)), mustJSON(t, currentPath))

	for _, chunk := range []string{prelude, sidebarDOM, string(sidebarJS)} {
		if _, err := vm.RunString(chunk); err != nil {
			t.Fatalf("evaluating sidebar.js: %v", err)
		}
	}
	return vm
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func call(t *testing.T, vm *goja.Runtime, fn string, args ...any) goja.Value {
	t.Helper()

	f, ok := goja.AssertFunction(vm.Get(fn))
	if !ok {
		t.Fatalf("%s is not defined", fn)
	}
	vals := make([]goja.Value, len(args))
	for i, a := range args {
		vals[i] = vm.ToValue(a)
	}
	res, err := f(goja.Undefined(), vals...)
	if err != nil {
		t.Fatalf("%s(%v): %v", fn, args, err)
	}
	return res
}

func renderedRows(t *testing.T, vm *goja.Runtime) []string {
	t.Helper()
	var out []string
	if err := vm.ExportTo(call(t, vm, "rendered"), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// node builds a tree node in the shape serveTreeJSON emits.
func node(name, path string, children ...map[string]any) map[string]any {
	n := map[string]any{"name": name, "path": path}
	if children != nil {
		n["isDir"] = true
		n["children"] = children
	}
	return n
}

// sampleTree mirrors a small vault: two folders, one nested two deep.
func sampleTree() map[string]any {
	return node("vault", "/",
		node("guides", "/guides/",
			node("deep", "/guides/deep/",
				node("buried.md", "/guides/deep/buried.md"),
			),
			node("intro.md", "/guides/intro.md"),
		),
		node("notes", "/notes/",
			node("daily.md", "/notes/daily.md"),
		),
		node("README.md", "/README.md"),
	)
}

// TestSidebarRendersLazily is the performance half of issue #18: a collapsed
// folder must not have had its children built. The previous implementation
// built every node on load — ~10,900 elements on a 4,800-file vault, on every
// page navigation.
func TestSidebarRendersLazily(t *testing.T) {
	vm := sidebarVM(t, sampleTree(), "/README.md")

	rows := renderedRows(t, vm)
	// Top level is present.
	if !hasRow(rows, "tree-folder", "guides") || !hasRow(rows, "tree-file", "README.md") {
		t.Fatalf("top level not rendered: %v", rows)
	}
	// Nothing from inside a collapsed folder is.
	if hasRowText(rows, "intro.md") || hasRowText(rows, "buried.md") || hasRowText(rows, "daily.md") {
		t.Errorf("collapsed folders were rendered eagerly: %v", rows)
	}
	if got := call(t, vm, "populatedFolders").ToInteger(); got != 0 {
		t.Errorf("%d folders populated before any was opened, want 0", got)
	}

	// Opening one builds exactly that folder's children, and no deeper.
	call(t, vm, "clickFolder", "guides")
	rows = renderedRows(t, vm)
	if !hasRowText(rows, "intro.md") {
		t.Errorf("opening guides did not render its children: %v", rows)
	}
	if hasRowText(rows, "buried.md") {
		t.Errorf("opening guides also rendered its grandchildren: %v", rows)
	}
}

// TestSidebarExpandsToCurrentPath: the branch holding the page being viewed is
// the one exception to lazy rendering, so the active file is visible on load.
func TestSidebarExpandsToCurrentPath(t *testing.T) {
	vm := sidebarVM(t, sampleTree(), "/guides/deep/buried.md")

	rows := renderedRows(t, vm)
	if !hasRowText(rows, "buried.md") {
		t.Fatalf("the current file was not revealed: %v", rows)
	}
	if !hasRow(rows, "active", "buried.md") {
		t.Errorf("the current file was not marked active: %v", rows)
	}
	// The unrelated branch stays closed.
	if hasRowText(rows, "daily.md") {
		t.Errorf("an unrelated folder was expanded: %v", rows)
	}
}

// TestSidebarFilter is the search half of issue #18.
func TestSidebarFilter(t *testing.T) {
	vm := sidebarVM(t, sampleTree(), "/README.md")

	t.Run("finds a file buried deep", func(t *testing.T) {
		call(t, vm, "setFilter", "buried")
		rows := renderedRows(t, vm)
		if !hasRowText(rows, "buried.md") {
			t.Fatalf("filter did not find buried.md: %v", rows)
		}
		// The folders it lives under are shown as context.
		if !hasRowClassContaining(rows, "tree-result-path", "guides / deep") {
			t.Errorf("no path context for the match: %v", rows)
		}
		if hasRowText(rows, "daily.md") {
			t.Errorf("non-matching files were rendered: %v", rows)
		}
	})

	t.Run("match is highlighted", func(t *testing.T) {
		call(t, vm, "setFilter", "urie")
		rows := renderedRows(t, vm)
		if !hasRowClassContaining(rows, "tree-match", "urie") {
			t.Errorf("the matched substring was not marked: %v", rows)
		}
	})

	t.Run("matches names, not folder names", func(t *testing.T) {
		// "guides" is a directory; without a slash the query only looks at file
		// names, so this must not drag in everything under guides/.
		call(t, vm, "setFilter", "guides")
		if got := call(t, vm, "status").String(); !strings.Contains(got, "No matching files") {
			t.Errorf("status = %q, want no matches for a directory-only query", got)
		}
	})

	t.Run("a slash switches to path matching", func(t *testing.T) {
		call(t, vm, "setFilter", "guides/")
		rows := renderedRows(t, vm)
		if !hasRowText(rows, "intro.md") || !hasRowText(rows, "buried.md") {
			t.Errorf("path query did not match everything under guides/: %v", rows)
		}
		if hasRowText(rows, "daily.md") {
			t.Errorf("path query leaked outside guides/: %v", rows)
		}
	})

	t.Run("case-insensitive", func(t *testing.T) {
		call(t, vm, "setFilter", "README")
		if !hasRowText(renderedRows(t, vm), "README.md") {
			t.Error("uppercase query missed README.md")
		}
		call(t, vm, "setFilter", "readme")
		if !hasRowText(renderedRows(t, vm), "README.md") {
			t.Error("lowercase query missed README.md")
		}
	})

	t.Run("no matches reports so", func(t *testing.T) {
		call(t, vm, "setFilter", "zzzznope")
		if got := call(t, vm, "status").String(); !strings.Contains(got, "No matching files") {
			t.Errorf("status = %q, want a no-matches message", got)
		}
		if rows := renderedRows(t, vm); len(rows) != 0 {
			t.Errorf("rows rendered for a query with no matches: %v", rows)
		}
	})

	t.Run("clearing restores the tree", func(t *testing.T) {
		call(t, vm, "setFilter", "")
		rows := renderedRows(t, vm)
		if !hasRow(rows, "tree-folder", "guides") || !hasRow(rows, "tree-file", "README.md") {
			t.Errorf("tree not restored after clearing: %v", rows)
		}
		if got := call(t, vm, "status").String(); got != "" {
			t.Errorf("status = %q, want empty once the filter is cleared", got)
		}
	})

	t.Run("escape clears the filter", func(t *testing.T) {
		call(t, vm, "setFilter", "buried")
		call(t, vm, "pressEscape")
		rows := renderedRows(t, vm)
		if !hasRow(rows, "tree-file", "README.md") {
			t.Errorf("Escape did not restore the tree: %v", rows)
		}
	})
}

// TestSidebarFilterCapsResults keeps a one-character query from rebuilding the
// whole tree as a flat list, which would undo the lazy rendering.
func TestSidebarFilterCapsResults(t *testing.T) {
	var files []map[string]any
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("note-%03d.md", i)
		files = append(files, node(name, "/bulk/"+name))
	}
	tree := node("vault", "/", node("bulk", "/bulk/", files...))

	vm := sidebarVM(t, tree, "/")
	call(t, vm, "setFilter", "note")

	rows := renderedRows(t, vm)
	links := 0
	for _, r := range rows {
		if strings.HasPrefix(r, "tree-file") {
			links++
		}
	}
	if links > 200 {
		t.Errorf("rendered %d results, want the 200 cap enforced", links)
	}
	got := call(t, vm, "status").String()
	if !strings.Contains(got, "200") || !strings.Contains(got, "500") {
		t.Errorf("status = %q, want it to report showing 200 of 500", got)
	}
}

// TestSidebarEscapesFileNames: names come from the filesystem and go into the
// DOM, so they must be text, never markup.
func TestSidebarEscapesFileNames(t *testing.T) {
	evil := `<img src=x onerror=alert(1)>.md`
	tree := node("vault", "/", node(evil, "/"+evil))

	vm := sidebarVM(t, tree, "/")
	rows := renderedRows(t, vm)
	if !hasRowText(rows, evil) {
		t.Fatalf("the file was not rendered at all: %v", rows)
	}

	// Rendered through textContent, so it is one text node, not parsed markup.
	call(t, vm, "setFilter", "onerror")
	if rows = renderedRows(t, vm); !hasRowText(rows, evil) {
		t.Errorf("filtering mangled the name: %v", rows)
	}
}

// --- row helpers -----------------------------------------------------------

// hasRow reports whether a row carries the given class (among others, since an
// element may also be "collapsed" or "active") and has exactly that text.
func hasRow(rows []string, class, text string) bool {
	for _, r := range rows {
		i := strings.Index(r, "|")
		if i < 0 || r[i+1:] != text {
			continue
		}
		for _, c := range strings.Fields(r[:i]) {
			if c == class {
				return true
			}
		}
	}
	return false
}

func hasRowText(rows []string, text string) bool {
	for _, r := range rows {
		if i := strings.Index(r, "|"); i >= 0 && r[i+1:] == text {
			return true
		}
	}
	return false
}

func hasRowClassContaining(rows []string, class, text string) bool {
	for _, r := range rows {
		i := strings.Index(r, "|")
		if i < 0 {
			continue
		}
		if strings.Contains(r[:i], class) && strings.Contains(r[i+1:], text) {
			return true
		}
	}
	return false
}
