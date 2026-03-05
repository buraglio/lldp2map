package main

import (
	"os"

	"github.com/buraglio/lldp2map/cmd"
	"github.com/buraglio/lldp2map/gui"
)

func main() {
	// Fyne (GUI nonsense) has to own the main OS thread which is required on macOS and Windows, not sure about linux.
	// Check for --gui before handing off to Cobra so we can call gui.Run()
	// directly from main rather than from a goroutine.
	for _, arg := range os.Args[1:] {
		if arg == "--gui" {
			gui.Run()
			return
		}
	}
	cmd.Execute()
}
