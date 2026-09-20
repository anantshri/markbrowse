package main

import (
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/dop251/goja"
)

// tablesort.js is a browser script wrapped in an IIFE, so parseSortValue and
// friends are not reachable from the outside. These tests unwrap that IIFE and
// evaluate the body in a JS engine's global scope, which keeps them running
// against the exact bytes embedded into the binary instead of a Go
// re-implementation that could drift from the shipped asset.
// \r? so a CRLF checkout does not turn "the script changed shape" into the
// reported failure. .gitattributes should prevent that, but the message this
// produces otherwise ("no longer a bare IIFE") sends the reader hunting in
// entirely the wrong place.
var (
	iifeOpen  = regexp.MustCompile(`(?m)^\(function \(\) \{\r?$`)
	iifeClose = regexp.MustCompile(`(?m)^\}\)\(\);\r?$`)
)

// Just enough of the DOM for init() to be a no-op, plus helpers that drive the
// unwrapped functions the way makeSortable's click handler does.
const tablesortHarness = `
var document = {
  readyState: "complete",
  querySelectorAll: function () { return []; },
  addEventListener: function () {}
};
function sortColumn(values) {
  var keyed = values.map(function (v) { return { text: v, key: parseSortValue(v) }; });
  keyed.sort(function (a, b) { return compareValues(a.key, b.key); });
  return keyed.map(function (row) { return row.text; });
}
function parentRow(label, href) {
  return isParentRow({
    querySelector: function () {
      return { textContent: label, getAttribute: function () { return href; } };
    }
  });
}
function rowWithoutLink() {
  return isParentRow({ querySelector: function () { return null; } });
}
`

func tablesortVM(t *testing.T) *goja.Runtime {
	t.Helper()

	src := string(tablesortJS)
	if !iifeOpen.MatchString(src) || !iifeClose.MatchString(src) {
		t.Fatal("tablesort.js is no longer a bare IIFE; update the unwrapping in this test")
	}
	src = iifeClose.ReplaceAllString(iifeOpen.ReplaceAllString(src, ""), "")

	vm := goja.New()
	for _, chunk := range []string{tablesortHarness, src} {
		if _, err := vm.RunString(chunk); err != nil {
			t.Fatalf("evaluating tablesort.js: %v", err)
		}
	}
	return vm
}

func sortColumn(t *testing.T, vm *goja.Runtime, values []string) []string {
	t.Helper()

	fn, ok := goja.AssertFunction(vm.Get("sortColumn"))
	if !ok {
		t.Fatal("sortColumn helper missing")
	}
	res, err := fn(goja.Undefined(), vm.ToValue(values))
	if err != nil {
		t.Fatalf("sortColumn(%v): %v", values, err)
	}
	var out []string
	if err := vm.ExportTo(res, &out); err != nil {
		t.Fatalf("exporting sort result: %v", err)
	}
	return out
}

// TestTableSortOrders locks in the ordering of every value shape the sorter
// claims to understand. Ascending order is what a first header click produces;
// descending is the exact reverse, which the handler gets by negating cmp.
func TestTableSortOrders(t *testing.T) {
	vm := tablesortVM(t)

	for _, tc := range []struct {
		name  string
		input []string
		want  []string
	}{
		// Issue #19: "9.0" used to sort ahead of "8x.0" because the letter
		// pushed the whole cell into the text branch, and "%" did the same.
		{
			name:  "issue 19 digits with a letter between them",
			input: []string{"9.0", "8x.0", "10.0"},
			want:  []string{"8x.0", "9.0", "10.0"},
		},
		{
			name:  "issue 19 percentages",
			input: []string{"9%", "100%", "95%"},
			want:  []string{"9%", "95%", "100%"},
		},
		{
			name:  "attached unit letters",
			input: []string{"1.2m", "950k", "3.4m"},
			want:  []string{"1.2m", "3.4m", "950k"},
		},
		{
			name:  "currency with thousands separators",
			input: []string{"$1,250", "$42", "$999"},
			want:  []string{"$42", "$999", "$1,250"},
		},
		{
			name:  "version components, not decimals",
			input: []string{"8.10.0", "8.9.0", "10.0.1", "1.2.3"},
			want:  []string{"1.2.3", "8.9.0", "8.10.0", "10.0.1"},
		},
		{
			name:  "a single dot stays a decimal point",
			input: []string{"1.10", "1.9", "1.2"},
			want:  []string{"1.10", "1.2", "1.9"},
		},
		{
			name:  "plain and comma-separated numbers",
			input: []string{"1,234", "99", "1000", "-5", "3.5"},
			want:  []string{"-5", "3.5", "99", "1000", "1,234"},
		},
		{
			name:  "negatives sort below zero",
			input: []string{"-3", "5", "-10", "0"},
			want:  []string{"-10", "-3", "0", "5"},
		},
		{
			name:  "byte sizes compare in bytes",
			input: []string{"900 B", "1.5 KB", "42 B", "2 MB"},
			want:  []string{"42 B", "900 B", "1.5 KB", "2 MB"},
		},
		{
			name:  "byte sizes without a space before the unit",
			input: []string{"2.0MB", "900B", "42 B", "1.5 KB"},
			want:  []string{"42 B", "900B", "1.5 KB", "2.0MB"},
		},
		{
			name:  "listing timestamps sort chronologically",
			input: []string{"2026-06-14 08:30", "2026-05-30 16:44", "2026-08-02 15:04"},
			want:  []string{"2026-05-30 16:44", "2026-06-14 08:30", "2026-08-02 15:04"},
		},
		{
			name:  "bare dates sort chronologically",
			input: []string{"2026-06-14", "2026-05-30", "2026-12-01", "2026-01-05"},
			want:  []string{"2026-01-05", "2026-05-30", "2026-06-14", "2026-12-01"},
		},
		// The letters around the digits still decide the order, so an
		// identifier column is not reduced to its numbers.
		{
			name:  "letter-prefixed identifiers",
			input: []string{"A1", "B2", "A10", "C3"},
			want:  []string{"A1", "A10", "B2", "C3"},
		},
		{
			name:  "numbered labels sort naturally",
			input: []string{"Chapter 10", "Chapter 3", "Chapter 1"},
			want:  []string{"Chapter 1", "Chapter 3", "Chapter 10"},
		},
		{
			name:  "file names sort alphabetically",
			input: []string{"video.mp4", "notes.md", "archive.zip", "a.md"},
			want:  []string{"a.md", "archive.zip", "notes.md", "video.mp4"},
		},
		{
			name:  "same number, different suffix",
			input: []string{"3b", "3a", "3c"},
			want:  []string{"3a", "3b", "3c"},
		},
		{
			name:  "case-insensitive text",
			input: []string{"zebra", "Apple", "mango", "Banana", "ant"},
			want:  []string{"ant", "Apple", "Banana", "mango", "zebra"},
		},
		// Numbers ahead of text keeps the "—" placeholders of a directory
		// listing below the real sizes.
		{
			name:  "numbers sort above placeholders and empty cells",
			input: []string{"—", "386 B", "", "42 B"},
			want:  []string{"42 B", "386 B", "", "—"},
		},
		{
			name:  "mixed numeric and text column",
			input: []string{"alpha", "42", "beta", "7"},
			want:  []string{"7", "42", "alpha", "beta"},
		},
		{
			name:  "a prefix sorts above its extensions",
			input: []string{"1a", "1", "1b"},
			want:  []string{"1", "1a", "1b"},
		},
		{
			name:  "commas outside numbers are text",
			input: []string{"Smith, John", "Adams, Zoe"},
			want:  []string{"Adams, Zoe", "Smith, John"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortColumn(t, vm, tc.input); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ascending = %v, want %v", got, tc.want)
			}

			// A second click reverses the comparator, so the descending order
			// must be the exact mirror of the ascending one.
			reversed := make([]string, len(tc.input))
			for i, v := range tc.input {
				reversed[len(tc.input)-1-i] = v
			}
			wantDesc := make([]string, len(tc.want))
			for i, v := range tc.want {
				wantDesc[len(tc.want)-1-i] = v
			}
			got := sortColumn(t, vm, reversed)
			for i, j := 0, len(got)-1; i < j; i, j = i+1, j-1 {
				got[i], got[j] = got[j], got[i]
			}
			if !reflect.DeepEqual(got, wantDesc) {
				t.Errorf("descending = %v, want %v", got, wantDesc)
			}
		})
	}
}

// TestTableSortIsStable checks the comparator reports ties instead of
// inventing an order for cells that carry the same value.
func TestTableSortIsStable(t *testing.T) {
	vm := tablesortVM(t)

	// "1,234" and "1234" are the same number; "3A" and "3a" the same label.
	for _, tc := range []struct{ a, b string }{
		{"1,234", "1234"},
		{"3A", "3a"},
		{"1.5 KB", "1536"},
	} {
		v, err := vm.RunString(`compareValues(parseSortValue(` + strconv.Quote(tc.a) + `), parseSortValue(` + strconv.Quote(tc.b) + `))`)
		if err != nil {
			t.Fatalf("comparing %q and %q: %v", tc.a, tc.b, err)
		}
		if got := v.ToInteger(); got != 0 {
			t.Errorf("compareValues(%q, %q) = %d, want 0", tc.a, tc.b, got)
		}
	}
}

// TestTableSortParentRow covers the "../" row detection. The listing template
// renders the row as <a href="{{.ParentPath}}">../</a>, so the label is what
// identifies it; the href form is matched too for tables that spell it "../".
func TestTableSortParentRow(t *testing.T) {
	vm := tablesortVM(t)

	parentRow, ok := goja.AssertFunction(vm.Get("parentRow"))
	if !ok {
		t.Fatal("parentRow helper missing")
	}

	for _, tc := range []struct {
		name  string
		label string
		href  string
		want  bool
	}{
		{"root parent row", "../", "/", true},
		{"nested parent row", "../", "/sub/", true},
		{"padded label", " ../ ", "/sub/", true},
		{"relative href", "up", "../", true},
		{"relative href below a path", "up", "/sub/../", true},
		{"ordinary file row", "notes.md", "/notes.md", false},
		{"directory row", "guides/", "/guides/", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parentRow(goja.Undefined(), vm.ToValue(tc.label), vm.ToValue(tc.href))
			if err != nil {
				t.Fatalf("parentRow: %v", err)
			}
			if got := v.ToBoolean(); got != tc.want {
				t.Errorf("isParentRow(label=%q, href=%q) = %v, want %v", tc.label, tc.href, got, tc.want)
			}
		})
	}

	rowWithoutLink, ok := goja.AssertFunction(vm.Get("rowWithoutLink"))
	if !ok {
		t.Fatal("rowWithoutLink helper missing")
	}
	v, err := rowWithoutLink(goja.Undefined())
	if err != nil {
		t.Fatalf("rowWithoutLink: %v", err)
	}
	if v.ToBoolean() {
		t.Error("a row with no anchor in its first cell must not be pinned")
	}
}
