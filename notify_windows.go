//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

var scriptFormat = `Add-Type -AssemblyName System.Windows.Forms;
Add-Type -AssemblyName System.Drawing;

$ErrorActionPreference= 'silentlycontinue';
$notifyIcon = New-Object System.Windows.Forms.NotifyIcon;
$notifyIcon.Icon = New-Object System.Drawing.Icon("%s") || [System.Drawing.SystemIcons]::Information;
$notifyIcon.BalloonTipTitle = "%s";
$notifyIcon.BalloonTipText = "%s";
$notifyIcon.Visible = $true;

$notifyIcon.ShowBalloonTip(5000);
Start-Sleep -Seconds 6;
$notifyIcon.Dispose();`

// todo don't create the script everytime
func notifySend(summary, body string) error {
	command, err := exec.LookPath("pwsh")
	if err != nil {
		return err
	}

	config, _ := path.Split(options.blogRoll)
	iconPath := path.Join(config, "badrss.ico")
	bodyWithLines := strings.ReplaceAll(body, "\n", "`n")

	script := fmt.Sprintf(scriptFormat,
		iconPath,
		summary,
		bodyWithLines,
	)

	f, err := os.CreateTemp("", "*badrss.ps1")
	defer os.Remove(f.Name())
	_, err = f.WriteString(script)
	if err != nil {
		return err
	}
	f.Close()

	cmd := exec.Command(command, f.Name())

	return cmd.Run()
}
