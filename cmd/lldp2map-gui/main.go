// lldp2map-gui is a standalone GUI-only entry point for lldp2map.
// Build with:
//
//	go build ./cmd/lldp2map-gui
//
// On macOS, wrap in an app bundle for Dock integration:
//
//	fyne package -os darwin -appID io.github.buraglio.lldp2map -name lldp2map
package main

import "github.com/buraglio/lldp2map/gui"

func main() {
	gui.Run()
}
