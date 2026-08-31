package apptracker

import (
	"testing"
	"time"
)

func TestTitleToApp(t *testing.T) {
	cases := map[string]string{
		"PAY — Mozilla Firefox":            "Mozilla Firefox",
		"#algemeen | XS News - Discord":    "Discord",
		"✳ PayPal payment_id missing rcur": "PayPal payment_id missing rcur",
		"🔔 git diff paysys ~/g/s/c":        "git diff paysys ~/g/s/c",
		"Mozilla Firefox":                  "Mozilla Firefox",
	}
	for in, want := range cases {
		if got := titleToApp(in); got != want {
			t.Errorf("titleToApp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeMergesSpinnerFrames(t *testing.T) {
	frames := []string{"✳ Build failing", "◑ Build failing", "◐ Build failing", "🔔 Build failing"}
	for _, f := range frames {
		if got := normalize(f); got != "Build failing" {
			t.Errorf("normalize(%q) = %q", f, got)
		}
	}
}

func TestGetUsageCapsAndSummarises(t *testing.T) {
	tr := New()
	tr.appTime = map[string]time.Duration{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		tr.appTime[n] = 5 * time.Minute
	}
	tr.appTime["blip"] = 3 * time.Second

	usage := tr.GetUsage()
	if len(usage) != maxApps+1 {
		t.Fatalf("got %d entries, want %d", len(usage), maxApps+1)
	}
	last := usage[len(usage)-1]
	if last.Name != "other (3 apps)" {
		t.Errorf("summary name = %q", last.Name)
	}
	if want := 10*time.Minute + 3*time.Second; last.Duration != want {
		t.Errorf("summary duration = %s, want %s", last.Duration, want)
	}
}
