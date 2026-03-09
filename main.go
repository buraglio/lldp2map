//go:build !nogui

package main

import (
	"os"

	"github.com/buraglio/lldp2map/cmd"
	"github.com/buraglio/lldp2map/gui"
)

func main() {
	// Fyne requires the main OS thread on macOS and Windows. Check for --gui
	// before handing off to Cobra so gui.Run() is called from main directly.
	for _, arg := range os.Args[1:] {
		if arg == "--gui" {
			gui.Run()
			return
		}
	}
	cmd.Execute()
}
