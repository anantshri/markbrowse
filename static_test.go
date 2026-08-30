package main

import (
	"strings"
	"testing"
)

func TestAlertCSSIncludesBackgroundTints(t *testing.T) {
	// GitHub renders alert callouts with a tinted background per type, not
	// just a colored left border. Ensure the embedded stylesheet carries
	// both the muted background variables and the per-type background rules.
	for _, want := range []string{
		"--bgColor-accent-muted",
		"--bgColor-success-muted",
		"--bgColor-attention-muted",
		"--bgColor-danger-muted",
		"--bgColor-done-muted",
		".markdown-body .markdown-alert.markdown-alert-note{border-left-color:var(--borderColor-accent-emphasis);background-color:var(--bgColor-accent-muted)}",
		".markdown-body .markdown-alert.markdown-alert-tip{border-left-color:var(--borderColor-success-emphasis);background-color:var(--bgColor-success-muted)}",
		".markdown-body .markdown-alert.markdown-alert-important{border-left-color:var(--borderColor-done-emphasis);background-color:var(--bgColor-done-muted)}",
		".markdown-body .markdown-alert.markdown-alert-warning{border-left-color:var(--borderColor-attention-emphasis);background-color:var(--bgColor-attention-muted)}",
		".markdown-body .markdown-alert.markdown-alert-caution{border-left-color:var(--borderColor-danger-emphasis);background-color:var(--bgColor-danger-muted)}",
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("defaultCSS missing %q", want)
		}
	}
}

func TestTablesortCSSPresent(t *testing.T) {
	for _, want := range []string{
		".markdown-body th,.dir-list th{cursor:pointer;user-select:none}",
		".sort-ind{font-size:.8em;opacity:.6;margin-left:2px}",
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("defaultCSS missing %q", want)
		}
	}
}

func TestTocCSSPresent(t *testing.T) {
	for _, want := range []string{
		".mdview-toc{",
		".mdview-toc .toc-tree a.active{",
		"scroll-behavior:smooth",
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("defaultCSS missing %q", want)
		}
	}
}

func TestTocToggleCSSPresent(t *testing.T) {
	for _, want := range []string{
		"#toc-toggle{position:fixed;top:12px;right:240px;",
		"body.toc-collapsed .mdview-toc{width:0;min-width:0;overflow:hidden;border-left:none;padding-left:0;padding-right:0}",
		"body.toc-collapsed #toc-toggle{right:12px}",
		// Below 1100px the TOC can't show, so the button must hide too.
		"@media(max-width:1100px){.mdview-toc{display:none}#toc-toggle{display:none}}",
		// Collapse animates like the left sidebar.
		"transition:width .2s ease,min-width .2s ease",
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("defaultCSS missing %q", want)
		}
	}
}
