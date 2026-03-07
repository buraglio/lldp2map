package gui

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/buraglio/lldp2map/internal/discover"
)

// terminalTheme overrides colours for the log pane: green text on near-black.
type terminalTheme struct{ fyne.Theme }

func (t terminalTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameForeground, theme.ColorNameDisabled:
		return color.RGBA{R: 57, G: 255, B: 20, A: 255} // #39FF14 bright terminal green
	case theme.ColorNameBackground, theme.ColorNameInputBackground:
		return color.RGBA{R: 13, G: 17, B: 23, A: 255} // #0D1117 near-black
	}
	return t.Theme.Color(n, v)
}

// Run launches the Fyne GUI. Must be called from the main goroutine.
func Run() {
	a := app.NewWithID("io.github.buraglio.lldp2map")
	w := a.NewWindow("lldp2map")
	w.Resize(fyne.NewSize(960, 680))

	// ── SNMP fields ──────────────────────────────────────────────────────────
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("3fff::1 or 192.168.1.1")

	versionSelect := widget.NewSelect([]string{"2c", "3"}, nil)
	versionSelect.SetSelected("2c")

	communityEntry := widget.NewEntry()
	communityEntry.SetText("public")

	// SNMPv3 fields — hidden until version "3" is selected
	usernameEntry := widget.NewEntry()

	authProtoSelect := widget.NewSelect([]string{"MD5", "SHA", "SHA256", "SHA512"}, nil)
	authProtoSelect.SetSelected("SHA")

	authPassEntry := widget.NewPasswordEntry()

	privProtoSelect := widget.NewSelect([]string{"DES", "AES", "AES192", "AES256"}, nil)
	privProtoSelect.SetSelected("AES")

	privPassEntry := widget.NewPasswordEntry()

	secLevelSelect := widget.NewSelect([]string{"noauth", "auth", "authpriv"}, nil)
	secLevelSelect.SetSelected("authpriv")

	v3Form := widget.NewForm(
		widget.NewFormItem("Username", usernameEntry),
		widget.NewFormItem("Auth Protocol", authProtoSelect),
		widget.NewFormItem("Auth Password", authPassEntry),
		widget.NewFormItem("Priv Protocol", privProtoSelect),
		widget.NewFormItem("Priv Password", privPassEntry),
		widget.NewFormItem("Security Level", secLevelSelect),
	)
	v3Section := container.NewVBox(widget.NewSeparator(), widget.NewLabel("SNMPv3 Settings"), v3Form)
	v3Section.Hide()

	versionSelect.OnChanged = func(v string) {
		if v == "3" {
			v3Section.Show()
		} else {
			v3Section.Hide()
		}
	}

	// ── Discovery fields ─────────────────────────────────────────────────────
	portEntry := widget.NewEntry()
	portEntry.SetText("161")

	maxHopsEntry := widget.NewEntry()
	maxHopsEntry.SetText("10")

	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText("5")

	retriesEntry := widget.NewEntry()
	retriesEntry.SetText("2")

	showAddrsCheck := widget.NewCheck("Show interface addresses (--show-addrs)", nil)

	addrFamilySelect := widget.NewSelect([]string{"both", "ipv4", "ipv6"}, nil)
	addrFamilySelect.SetSelected("both")

	ignorePrefixEntry := widget.NewMultiLineEntry()
	ignorePrefixEntry.SetPlaceHolder("One CIDR per line, e.g.\n127.0.0.0/8\nfd68:1::/48")
	ignorePrefixEntry.SetMinRowsVisible(3)

	// ── Output fields ─────────────────────────────────────────────────────────
	formatSelect := widget.NewSelect([]string{"png", "pdf", "drawio", "excalidraw"}, nil)
	formatSelect.SetSelected("png")

	outputEntry := widget.NewEntry()
	outputEntry.SetText("network-map.png")

	extMap := map[string]string{
		"png": ".png", "pdf": ".pdf",
		"drawio": ".drawio", "excalidraw": ".excalidraw",
	}
	formatSelect.OnChanged = func(f string) {
		base := strings.TrimSuffix(outputEntry.Text, filepath.Ext(outputEntry.Text))
		outputEntry.SetText(base + extMap[f])
	}

	browseBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		d := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			uc.Close()
			outputEntry.SetText(uc.URI().Path())
		}, w)
		d.Show()
	})
	outputRow := container.NewBorder(nil, nil, nil, browseBtn, outputEntry)

	// ── Log / progress area ───────────────────────────────────────────────────
	logEntry := widget.NewMultiLineEntry()
	logEntry.Disable()
	logScroll := container.NewScroll(logEntry)

	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()
	progressBar.Stop()

	// ── Buttons ───────────────────────────────────────────────────────────────
	var openBtn *widget.Button
	openBtn = widget.NewButtonWithIcon("Open Result", theme.FileImageIcon(), func() {
		abs, err := filepath.Abs(outputEntry.Text)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		u, err := url.Parse("file://" + abs)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		_ = fyne.CurrentApp().OpenURL(u)
	})
	openBtn.Disable()

	appendLog := func(msg string) {
		fyne.Do(func() {
			cur := logEntry.Text
			if cur == "" {
				logEntry.SetText(msg)
			} else {
				logEntry.SetText(cur + "\n" + msg)
			}
			logScroll.ScrollToBottom()
		})
	}

	// cancelFn cancels the currently running discovery goroutine (if any).
	var cancelFn context.CancelFunc

	// Declare buttons before resetUI so they can all reference each other.
	var runBtn *widget.Button
	var cancelBtn *widget.Button

	resetUI := func(success bool) {
		progressBar.Stop()
		progressBar.Hide()
		runBtn.Enable()
		cancelBtn.Disable()
		if success {
			openBtn.Enable()
		}
	}

	cancelBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		if cancelFn != nil {
			cancelFn()
		}
	})
	cancelBtn.Disable()

	runBtn = widget.NewButton("Run Discovery", func() {
		host := strings.TrimSpace(hostEntry.Text)
		if host == "" {
			dialog.ShowError(fmt.Errorf("seed host is required"), w)
			return
		}

		port, err := strconv.Atoi(portEntry.Text)
		if err != nil || port < 1 || port > 65535 {
			dialog.ShowError(fmt.Errorf("invalid port: %s", portEntry.Text), w)
			return
		}
		maxHops, err := strconv.Atoi(maxHopsEntry.Text)
		if err != nil || maxHops < 1 {
			dialog.ShowError(fmt.Errorf("invalid max-hops: %s", maxHopsEntry.Text), w)
			return
		}
		timeout, err := strconv.Atoi(timeoutEntry.Text)
		if err != nil || timeout < 1 {
			dialog.ShowError(fmt.Errorf("invalid timeout: %s", timeoutEntry.Text), w)
			return
		}
		retries, err := strconv.Atoi(retriesEntry.Text)
		if err != nil || retries < 0 {
			dialog.ShowError(fmt.Errorf("invalid retries: %s", retriesEntry.Text), w)
			return
		}

		outFile := strings.TrimSpace(outputEntry.Text)
		if outFile == "" {
			outFile = "network-map." + formatSelect.Selected
			outputEntry.SetText(outFile)
		}

		// Parse the ignore-prefix box: split on newlines, drop blank lines.
		var ignorePrefixStrs []string
		for _, line := range strings.Split(ignorePrefixEntry.Text, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				ignorePrefixStrs = append(ignorePrefixStrs, s)
			}
		}

		cfg := discover.Config{
			SeedHost:         host,
			Community:        communityEntry.Text,
			Version:          versionSelect.Selected,
			Username:         usernameEntry.Text,
			AuthProto:        authProtoSelect.Selected,
			AuthPass:         authPassEntry.Text,
			PrivProto:        privProtoSelect.Selected,
			PrivPass:         privPassEntry.Text,
			SecLevel:         secLevelSelect.Selected,
			Port:             uint16(port),
			Timeout:          timeout,
			Retries:          retries,
			MaxHops:          maxHops,
			ShowAddrs:        showAddrsCheck.Checked,
			AddrFamily:       addrFamilySelect.Selected,
			IgnorePrefixStrs: ignorePrefixStrs,
			OutputFile:       outFile,
			OutputFormat:     formatSelect.Selected,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancelFn = cancel

		runBtn.Disable()
		openBtn.Disable()
		cancelBtn.Enable()
		logEntry.SetText("")
		progressBar.Show()
		progressBar.Start()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					fyne.Do(func() {
						msg := fmt.Sprintf("panic in discovery goroutine: %v", r)
						logEntry.SetText(logEntry.Text + "\nERROR: " + msg)
						logScroll.ScrollToBottom()
						dialog.ShowError(fmt.Errorf("%s", msg), w)
						resetUI(false)
					})
				}
			}()

			_, runErr := discover.Run(ctx, cfg, appendLog)
			cancel() // release context resources
			fyne.Do(func() {
				if runErr != nil {
					logEntry.SetText(logEntry.Text + "\nERROR: " + runErr.Error())
					logScroll.ScrollToBottom()
					dialog.ShowError(runErr, w)
					resetUI(false)
				} else {
					resetUI(true)
				}
			})
		}()
	})

	// ── Left panel: settings ──────────────────────────────────────────────────
	snmpForm := widget.NewForm(
		widget.NewFormItem("Seed Host", hostEntry),
		widget.NewFormItem("Version", versionSelect),
		widget.NewFormItem("Community", communityEntry),
	)

	discoveryForm := widget.NewForm(
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("Max Hops", maxHopsEntry),
		widget.NewFormItem("Timeout (s)", timeoutEntry),
		widget.NewFormItem("Retries", retriesEntry),
		widget.NewFormItem("Addr Family", addrFamilySelect),
		widget.NewFormItem("Ignore Prefixes", ignorePrefixEntry),
	)

	outputForm := widget.NewForm(
		widget.NewFormItem("Format", formatSelect),
		widget.NewFormItem("Output File", outputRow),
	)

	leftPanel := container.NewVBox(
		widget.NewLabel("SNMP"),
		snmpForm,
		v3Section,
		widget.NewSeparator(),
		widget.NewLabel("Discovery"),
		discoveryForm,
		showAddrsCheck,
		widget.NewSeparator(),
		widget.NewLabel("Output"),
		outputForm,
		widget.NewSeparator(),
		container.NewHBox(runBtn, cancelBtn, openBtn),
	)
	leftScroll := container.NewVScroll(leftPanel)
	leftScroll.SetMinSize(fyne.NewSize(340, 0))

	// ── Right panel: log ──────────────────────────────────────────────────────
	logTerminal := container.NewThemeOverride(logScroll, terminalTheme{theme.Current()})
	rightPanel := container.NewBorder(
		widget.NewLabel("Discovery Log"),
		progressBar,
		nil, nil,
		logTerminal,
	)

	// ── Root layout ───────────────────────────────────────────────────────────
	split := container.NewHSplit(leftScroll, rightPanel)
	split.SetOffset(0.36)

	w.SetContent(split)
	w.ShowAndRun()
}
