//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
)

var scriptFormat = `Add-Type -AssemblyName System.Windows.Forms;
Add-Type -AssemblyName System.Drawing;
$ErrorActionPreference= "silentlycontinue";
$notifyIcon = New-Object System.Windows.Forms.NotifyIcon;
$notifyIcon.Icon = New-Object System.Drawing.Icon("%s") || [System.Drawing.SystemIcons]::Information;
$notifyIcon.BalloonTipTitle = "%s";
$notifyIcon.BalloonTipText = "%s";
$notifyIcon.Visible = $true;
$notifyIcon.ShowBalloonTip(5000);
Start-Sleep -Seconds 6;
$notifyIcon.Dispose();`

// todo don't create the script everytime
func notifySend(summary, body string) (*exec.Cmd, error) {
	command, err := exec.LookPath("pwsh")
	if err != nil {
		return nil, err
	}

	config, _ := path.Split(options.blogRoll)
	iconPath := path.Join(config, "badrss.ico")

	escaper := strings.NewReplacer("\n", "`n", "'", "`'", "\"", "`\"", "`", "``")
	escapedBody := escaper.Replace(body)

	script := fmt.Sprintf(strings.ReplaceAll(scriptFormat, "\n", " "),
		iconPath,
		summary,
		escapedBody,
	)

	cmd := exec.Command(command, "-c", script)

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}
