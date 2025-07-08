package main

import (
	"encoding/xml"
	"fmt"
	"os"
)


func main() {
	s, err := os.ReadFile("feed.xml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		return
	}

	var feed struct{}
	err = xml.Unmarshal(s, &feed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s", err)
		return
	}
}
