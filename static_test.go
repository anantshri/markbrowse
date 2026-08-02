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
		".markdown-alert-note{border-left-color:var(--borderColor-accent-emphasis);background-color:var(--bgColor-accent-muted)}",
		".markdown-alert-tip{border-left-color:var(--borderColor-success-emphasis);background-color:var(--bgColor-success-muted)}",
		".markdown-alert-important{border-left-color:var(--borderColor-done-emphasis);background-color:var(--bgColor-done-muted)}",
		".markdown-alert-warning{border-left-color:var(--borderColor-attention-emphasis);background-color:var(--bgColor-attention-muted)}",
		".markdown-alert-caution{border-left-color:var(--borderColor-danger-emphasis);background-color:var(--bgColor-danger-muted)}",
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Errorf("defaultCSS missing %q", want)
		}
	}
}
