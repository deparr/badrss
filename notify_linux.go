//go:build linux

package main

import (
	"os/exec"
)

func notifySend(summary, body string) (*exec.Cmd, error) {
	binary, err := exec.LookPath("notify-send")
	if err != nil {
		return nil, err
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
	err = command.Start()
	if err != nil {
		return nil, err
	}

	return command, nil
}
