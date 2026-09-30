package launchd

import (
	"strings"
	"testing"
)

func TestPlistContainsScheduleArgsAndEscapedPaths(t *testing.T) {
	got := Plist(Config{Binary: "/Users/me/go/bin/chainsaw", Root: "/Users/me/co & more", LogPath: "/tmp/prune.log", Hour: 10, Minute: 0})
	for _, want := range []string{
		"<string>com.sam-evani.chainsaw</string>",
		"<string>/Users/me/go/bin/chainsaw</string>",
		"<string>prune</string>",
		"<string>/Users/me/co &amp; more</string>",
		"<key>Hour</key>\n\t\t<integer>10</integer>",
		"<key>Minute</key>\n\t\t<integer>0</integer>",
		"/opt/homebrew/bin",
		"<string>/tmp/prune.log</string>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist missing %q:\n%s", want, got)
		}
	}
}
