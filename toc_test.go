package main

import (
	"testing"

	"github.com/dop251/goja"
)

// toc.js is a browser script wrapped in an IIFE that returns early when there is
// no outline to build, so it cannot be unwrapped the way tablesort.js is — a bare
// return is invalid at the top level of a script. Like sidebar_test.go, these
// tests therefore run the file as-is against a DOM shim and drive it from the
// outside: the shim captures the IntersectionObserver callback that highlight()
// registers, and the tests fire it the way scrolling does.
const tocHarness = `
var __observers = [];

// A global as well as a window property, because toc.js calls the bare
// IntersectionObserver constructor.
function IntersectionObserver(cb, opts) {
  __observers.push({ cb: cb, opts: opts });
}
IntersectionObserver.prototype.observe = function () {};

function El(tag) {
  this.tagName = tag;
  this.className = "";
  this.open = false;
  this.childNodes = [];
  this.attrs = {};
  this.__text = "";
}
El.prototype.appendChild = function (c) { this.childNodes.push(c); return c; };
El.prototype.setAttribute = function (k, v) { this.attrs[k] = v; };
El.prototype.getAttribute = function (k) { return this.attrs[k]; };
Object.defineProperty(El.prototype, "textContent", {
  get: function () {
    if (this.childNodes.length) {
      return this.childNodes.map(function (c) { return c.textContent; }).join("");
    }
    return this.__text;
  },
  set: function (v) { this.childNodes = []; this.__text = String(v); }
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

var __headings = HEADINGS.map(function (h) {
  var el = new El("h" + h.level);
  el.attrs.id = h.id;
  el.textContent = h.text;
  return el;
});

var __tocRoot = new El("nav");
__tocRoot.closest = function () { return null; };

var document = {
  body: { classList: { add: function () {}, remove: function () {}, toggle: function () {} } },
  getElementById: function (id) { return id === "toc-root" ? __tocRoot : null; },
  querySelectorAll: function () { return __headings; },
  createElement: function (tag) { return new El(tag); }
};

var window = { IntersectionObserver: IntersectionObserver };

// Fires the observer for one heading, as scrolling it into view would.
function intersect(index) {
  __observers[0].cb([{ isIntersecting: true, target: __headings[index] }]);
}

function observerCount() { return __observers.length; }

function collect(activeOnly) {
  var out = [];
  (function walk(nodes) {
    nodes.forEach(function (n) {
      if (n.tagName === "a" && (!activeOnly || n.className.split(" ").indexOf("active") !== -1)) {
        out.push(n.href || "");
      }
      if (n.childNodes && n.childNodes.length) walk(n.childNodes);
    });
  })([__tocRoot]);
  return out;
}

// The outline's link hrefs in document order.
function allHrefs() { return collect(false); }

// The hrefs of the outline links currently marked active.
function activeHrefs() { return collect(true); }
`

func tocVM(t *testing.T, headingsJSON string) *goja.Runtime {
	t.Helper()

	vm := goja.New()
	for _, chunk := range []string{"var HEADINGS = " + headingsJSON + ";", tocHarness, string(tocJS)} {
		if _, err := vm.RunString(chunk); err != nil {
			t.Fatalf("evaluating toc.js: %v", err)
		}
	}
	// If toc.js stops registering an observer — renamed, restructured, or its
	// highlight() no longer called — every assertion below would pass vacuously.
	if got := call(t, vm, "observerCount").ToInteger(); got != 1 {
		t.Fatalf("toc.js registered %d IntersectionObservers, want 1; the test harness no longer covers it", got)
	}
	return vm
}

func tocStrings(t *testing.T, vm *goja.Runtime, fn string) []string {
	t.Helper()

	var out []string
	if err := vm.ExportTo(call(t, vm, fn), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const twoHeadings = `[{"id":"one","level":1,"text":"One"},{"id":"two","level":2,"text":"Two"}]`

// TestTocBuildsOutlineFromHeadings: toc.js still renders a link per heading, and
// nothing is highlighted before anything has been scrolled to.
func TestTocBuildsOutlineFromHeadings(t *testing.T) {
	vm := tocVM(t, twoHeadings)

	hrefs := tocStrings(t, vm, "allHrefs")
	if len(hrefs) != 2 || hrefs[0] != "#one" || hrefs[1] != "#two" {
		t.Errorf("outline links = %v, want [#one #two]", hrefs)
	}
	if got := tocStrings(t, vm, "activeHrefs"); len(got) != 0 {
		t.Errorf("nothing has scrolled into view, but %v is already active", got)
	}
}

// TestTocHighlightsOnlyTheHeadingInView: scrolling to a heading marks that entry,
// and moving on marks the next one. The previously active entry is the one
// cleared, rather than every link in the document being cleared first.
func TestTocHighlightsOnlyTheHeadingInView(t *testing.T) {
	vm := tocVM(t, twoHeadings)

	call(t, vm, "intersect", 0)
	if got := tocStrings(t, vm, "activeHrefs"); len(got) != 1 || got[0] != "#one" {
		t.Fatalf("active = %v, want [#one]", got)
	}

	call(t, vm, "intersect", 1)
	if got := tocStrings(t, vm, "activeHrefs"); len(got) != 1 || got[0] != "#two" {
		t.Errorf("active = %v, want only [#two]", got)
	}

	// A repeat report of the heading already in view changes nothing.
	call(t, vm, "intersect", 1)
	if got := tocStrings(t, vm, "activeHrefs"); len(got) != 1 || got[0] != "#two" {
		t.Errorf("active = %v after a repeated report, want [#two]", got)
	}
}
