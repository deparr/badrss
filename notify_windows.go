//go:build windows

package main

import "fmt"

// todo work out an easy way to do this on windows
func notifySend(summary, body string) error {
	fmt.Println(summary)
	fmt.Println(body)
	fmt.Println("[TODO]: send this as a dekstop notification")
	return nil
}
