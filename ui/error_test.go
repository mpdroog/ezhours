package ui

import (
	"errors"
	"strings"
	"testing"
)

// TestSaveErrorText checks the dialog says all three things it has to: what
// failed, where the entry belongs, and the entry itself, verbatim.
func TestSaveErrorText(t *testing.T) {
	entry := "13mar\n 14:01 - 16:49\nwork on activeusers\n"
	body := saveErrorText("/home/mp/hours/portal.txt", entry, errors.New("write /home/mp/hours/portal.txt: no space left on device"))

	for _, want := range []string{
		"could not save",
		"no space left on device",
		"/home/mp/hours/portal.txt",
		entry,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("saveErrorText() = %q, want it to contain %q", body, want)
		}
	}
	// The entry must be last and unindented, so selecting it is one sweep and
	// what lands on the clipboard is exactly what belongs in the file.
	if !strings.HasSuffix(body, entry) {
		t.Errorf("saveErrorText() = %q, want it to end with the entry", body)
	}
}

func TestAppleScriptString(t *testing.T) {
	// AppleScript has no \n escape, so newlines become `return` joins, and a
	// description with a quote in it must not end the literal.
	got := appleScriptString("say \"hi\"\nsecond\\line")
	want := `"say \"hi\"" & return & "second\\line"`
	if got != want {
		t.Errorf("appleScriptString() = %s, want %s", got, want)
	}
}

func TestBase64Std(t *testing.T) {
	if got := base64Std("a'b\nc"); got != "YSdiCmM=" {
		t.Errorf("base64Std() = %q, want %q", got, "YSdiCmM=")
	}
}
