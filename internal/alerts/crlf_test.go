package alerts

import (
	"strings"
	"testing"
)

// TestLineEndingsDoNotChangeAlerts: markdown arrives however the author's
// editor and VCS left it. A CRLF document used to leave "\r" as the alert's
// title, which is non-empty, so the alert rendered with neither icon nor kind
// name — every alert in a CRLF vault lost its heading.
func TestLineEndingsDoNotChangeAlerts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		src          string
		wantContains []string
		wantOmits    []string
	}{
		{
			name:         "LF, no custom title",
			src:          "> [!NOTE]\n> Body.\n",
			wantContains: []string{`<svg id="note">`, ">Note</p>", "Body."},
		},
		{
			name:         "CRLF, no custom title",
			src:          "> [!NOTE]\r\n> Body.\r\n",
			wantContains: []string{`<svg id="note">`, ">Note</p>", "Body."},
		},
		{
			name:         "trailing spaces, no custom title",
			src:          "> [!NOTE]   \n> Body.\n",
			wantContains: []string{`<svg id="note">`, ">Note</p>"},
		},
		{
			name:         "CRLF with a trailing tab",
			src:          "> [!WARNING]\t\r\n> Body.\r\n",
			wantContains: []string{`<svg id="warning">`, ">Warning</p>"},
		},
		{
			name:         "CRLF, custom title",
			src:          "> [!NOTE] Custom\r\n> Body.\r\n",
			wantContains: []string{">Custom</p>"},
			wantOmits:    []string{"<svg", ">Note</p>"},
		},
		{
			name:         "CRLF, collapsed marker",
			src:          "> [!NOTE]-\r\n> Body.\r\n",
			wantContains: []string{`<svg id="note">`, ">Note</p>"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, tc.src)
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.wantOmits {
				if strings.Contains(got, unwanted) {
					t.Errorf("output should not contain %q:\n%s", unwanted, got)
				}
			}
		})
	}
}
