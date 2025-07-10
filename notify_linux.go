//go:build linux

package main

import (
	"os/exec"
)

func notifySend(summary, body string) error {
	binary, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}

	args := []string{
		"--app-name=badrss",
		"--icon=rss",
		"--expire-time=6000",
		"--hint=string:desktop-entry:badrss",
		summary,
		body,
	}

	command := exec.Command(binary, args...)

	return command.Run()
}
