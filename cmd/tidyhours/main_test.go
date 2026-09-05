package main

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"✳ PayPal payment_id missing rcur":        "Claude Code",
		"◐ PayPal payment_id missing rcur":        "Claude Code",
		"🔔 git diff paysys ~/g/s/c":               "Terminal",
		"PAY trial-3RY55 — Mozilla Firefox":       "Firefox",
		"#algemeen | XS News - Discord":           "Discord",
		"Friends - Discord":                       "Discord",
		"Inbox - rootdev@x - Mozilla Thunderbird": "Thunderbird",
		"Write: Re: Invoice - Thunderbird":        "Thunderbird",
		"Sending Message - My hitnews v2":         "Thunderbird",
		"root@solomon-grundy: /home/cleavr":       "Terminal",
		"~/g/s/c/s/c/xsnews-action":               "Terminal",
		"ssh mark@aws001.xsne ~/g/s/c":            "Terminal",
		"./ezhours  ~/g/s/g/m/ezhours":            "Terminal",
		"! sudo cp -a /etc/de ~":                  "Terminal",
		"Terminal - hours.txt (/tmp) - NVIM":      "Terminal",
		"mp - Thunar":                             "Thunar",
		"Screenshot":                              "Screenshot", // unknown: kept as-is
	}
	for in, want := range cases {
		if got := classify(in); got != want {
			t.Errorf("classify(%q) = %q, want %q", in, got, want)
		}
	}
}

// Titles contain commas, so fields end at a duration, not at every ", ".
func TestParseAppsHandlesCommasInTitles(t *testing.T) {
	got := parseApps("ChatGPT: Chat, Work, Create — Mozilla Firefox 4m, mydb — Mozilla Firefox 2m, ✳ Bug 30s")
	if want := 6 * time.Minute; got["Firefox"] != want {
		t.Errorf("Firefox = %s, want %s", got["Firefox"], want)
	}
	if want := 30 * time.Second; got["Claude Code"] != want {
		t.Errorf("Claude Code = %s, want %s", got["Claude Code"], want)
	}
	if len(got) != 2 {
		t.Errorf("got %d apps, want 2: %v", len(got), got)
	}
}

func TestTidyRewritesOnlyAppsLines(t *testing.T) {
	in := "21aug\n 10:54 - 11:24\npaypalv2 cron\n" +
		"  [Apps: ✳ Bug 8m, ◑ Bug 2m, #algemeen | XS News - Discord 4m, PAY — Mozilla Firefox 16s]\n"
	want := "21aug\n 10:54 - 11:24\npaypalv2 cron\n" +
		"  [Apps: Claude Code 10m, Discord 4m]\n"
	if got := tidy(in); got != want {
		t.Errorf("tidy() =\n%q\nwant\n%q", got, want)
	}
}
