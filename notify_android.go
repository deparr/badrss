//go:build android

package main

import (
	"fmt"
	"os/exec"
)

func notifySend(summary, body string) (*exec.Cmd, error) {
	fmt.Println(summary, "\n")
	fmt.Print(body)

	return nil, nil
}
