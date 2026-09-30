package launchd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const Label = "com.sam-evani.chainsaw"

type Config struct {
	Binary  string
	Root    string
	LogPath string
	Hour    int
	Minute  int
}

func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func Plist(c Config) string {
	esc := func(s string) string {
		var b bytes.Buffer
		xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>prune</string>
		<string>--root</string>
		<string>%s</string>
	</array>
	<key>StartCalendarInterval</key>
	<dict>
		<key>Hour</key>
		<integer>%d</integer>
		<key>Minute</key>
		<integer>%d</integer>
	</dict>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
	</dict>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, Label, esc(c.Binary), esc(c.Root), c.Hour, c.Minute, esc(c.LogPath), esc(c.LogPath))
}

type Launchctl func(args ...string) ([]byte, error)

func SystemLaunchctl(args ...string) ([]byte, error) {
	return exec.Command("launchctl", args...).CombinedOutput()
}

func domain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

func Install(c Config, run Launchctl) (string, error) {
	path, err := PlistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(c.LogPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(Plist(c)), 0o644); err != nil {
		return "", err
	}
	run("bootout", domain()+"/"+Label)
	if out, err := run("bootstrap", domain(), path); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	return path, nil
}

func Uninstall(run Launchctl) (string, error) {
	path, err := PlistPath()
	if err != nil {
		return "", err
	}
	run("bootout", domain()+"/"+Label)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

// Loaded reports whether the agent is currently loaded in the user domain.
func Loaded(run Launchctl) bool {
	_, err := run("print", domain()+"/"+Label)
	return err == nil
}
