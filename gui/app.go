package gui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"soreaja-printer/config"
	"soreaja-printer/printer"
)

// terminalTheme provides high-contrast terminal styling with pitch-black background and bright neon-green text.
type terminalTheme struct {
	fyne.Theme
}

func (t *terminalTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground, theme.ColorNameInputBackground:
		return color.RGBA{R: 10, G: 12, B: 16, A: 255} // Deep black background
	case theme.ColorNameForeground, theme.ColorNameDisabled:
		return color.RGBA{R: 57, G: 255, B: 20, A: 255} // High-contrast neon green (#39FF14)
	case theme.ColorNameInputBorder:
		return color.RGBA{R: 24, G: 60, B: 24, A: 255} // Subtle dark-green border
	case theme.ColorNameSelection:
		return color.RGBA{R: 20, G: 100, B: 20, A: 160} // Dark green selection highlight
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 35, G: 120, B: 35, A: 220} // Terminal green scrollbar
	}
	return t.Theme.Color(name, variant)
}

// RunApp initializes and runs the Fyne desktop GUI window.
func RunApp(cfg *config.Config, colorPrinter printer.Service, thermalPrinter printer.Service, logStream *LogStream) {
	a := app.NewWithID("com.soreaja.printer")
	w := a.NewWindow("Soreaja Printer Control Panel")
	w.Resize(fyne.NewSize(750, 700))
	w.SetFixedSize(false)

	serverMgr := NewServerManager(cfg, colorPrinter, thermalPrinter)
	printerTester := NewPrinterTester(colorPrinter, thermalPrinter)

	// ==================== 1. SERVER CONTROL HEADER ====================
	statusDot := canvas.NewCircle(color.RGBA{R: 220, G: 50, B: 50, A: 255})
	statusDot.Resize(fyne.NewSize(12, 12))

	statusLabel := widget.NewLabelWithStyle("STOPPED", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusMsgLabel := widget.NewLabel("Server is currently inactive")

	portEntry := widget.NewEntry()
	portEntry.SetText(cfg.ServerPort)
	portEntry.SetPlaceHolder("8080")

	endpointsLabel := widget.NewLabel("Local:   http://localhost:" + cfg.ServerPort + "\nHealth:  http://localhost:" + cfg.ServerPort + "/health")
	endpointsLabel.TextStyle = fyne.TextStyle{Monospace: true}
	endpointsLabel.Wrapping = fyne.TextWrapWord
	endpointsLabel.Selectable = true

	endpointsBg := canvas.NewRectangle(color.RGBA{R: 10, G: 12, B: 16, A: 255})
	endpointsBox := container.NewThemeOverride(
		container.NewStack(endpointsBg, container.NewPadded(endpointsLabel)),
		&terminalTheme{Theme: theme.DefaultTheme()},
	)

	copyEndpointsBtn := widget.NewButtonWithIcon("Copy All", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(endpointsLabel.Text)
	})
	copyEndpointsBtn.Importance = widget.LowImportance

	endpointsHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Service Endpoints:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		copyEndpointsBtn,
	)

	var toggleBtn *widget.Button

	updateServerUI := func(running bool, msg string) {
		fyne.Do(func() {
			if running {
				statusDot.FillColor = color.RGBA{R: 40, G: 200, B: 60, A: 255}
				statusDot.Refresh()
				statusLabel.SetText("RUNNING")
				statusMsgLabel.SetText("Active on port :" + serverMgr.ActivePort())
				portEntry.Disable()
				toggleBtn.SetText("Stop Server")
				toggleBtn.Importance = widget.DangerImportance
				toggleBtn.SetIcon(theme.MediaStopIcon())
				toggleBtn.Enable()

				var sb strings.Builder
				for _, addr := range serverMgr.GetNetworkAddresses() {
					sb.WriteString(addr + "\n")
				}
				sb.WriteString(fmt.Sprintf("API:     http://localhost:%s/api/v1/print", serverMgr.ActivePort()))
				endpointsLabel.SetText(sb.String())
			} else {
				statusDot.FillColor = color.RGBA{R: 220, G: 50, B: 50, A: 255}
				statusDot.Refresh()
				statusLabel.SetText("STOPPED")
				if msg != "" {
					statusMsgLabel.SetText(msg)
				} else {
					statusMsgLabel.SetText("Server is currently inactive")
				}
				portEntry.Enable()
				toggleBtn.SetText("Start Server")
				toggleBtn.Importance = widget.HighImportance
				toggleBtn.SetIcon(theme.MediaPlayIcon())
				toggleBtn.Enable()
			}
			toggleBtn.Refresh()
		})
	}

	toggleBtn = widget.NewButtonWithIcon("Start Server", theme.MediaPlayIcon(), func() {
		toggleBtn.Disable()
		if serverMgr.IsRunning() {
			go func() {
				_ = serverMgr.Stop()
			}()
		} else {
			port := strings.TrimSpace(portEntry.Text)
			if port == "" {
				port = "8080"
			}
			go func() {
				if err := serverMgr.Start(port); err != nil {
					updateServerUI(false, fmt.Sprintf("Error: %v", err))
				}
			}()
		}
	})
	toggleBtn.Importance = widget.HighImportance

	serverMgr.OnStatusChange(func(running bool, msg string) {
		updateServerUI(running, msg)
	})

	serverHeader := container.NewVBox(
		container.NewHBox(
			widget.NewLabelWithStyle("Soreaja Printer Microservice", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewCenter(statusDot),
			statusLabel,
		),
		statusMsgLabel,
		container.NewBorder(nil, nil, widget.NewLabel("HTTP Port:"), toggleBtn, portEntry),
		endpointsHeader,
		endpointsBox,
	)

	// ==================== 2. PRINTER TESTING TAB ====================
	// --- Color Printer Card ---
	colorQueueEntry := widget.NewEntry()
	colorQueueEntry.SetText(cfg.DefaultColorPrinterName)
	colorQueueEntry.SetPlaceHolder("EPSON_L120_Series")

	colorStatusResult := widget.NewLabel("Ready for diagnostics. Click below to inspect CUPS or send a test swatch.")
	colorStatusResult.TextStyle = fyne.TextStyle{Monospace: true}
	colorStatusResult.Wrapping = fyne.TextWrapWord
	colorStatusResult.Selectable = true

	colorStatusBg := canvas.NewRectangle(color.RGBA{R: 10, G: 12, B: 16, A: 255})
	colorStatusBox := container.NewThemeOverride(
		container.NewStack(colorStatusBg, container.NewPadded(colorStatusResult)),
		&terminalTheme{Theme: theme.DefaultTheme()},
	)

	copyColorBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(colorStatusResult.Text)
	})
	copyColorBtn.Importance = widget.LowImportance

	colorDiagHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Diagnostic Output:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		copyColorBtn,
	)

	checkColorBtn := widget.NewButtonWithIcon("Check CUPS Status", theme.SearchIcon(), func() {
		q := strings.TrimSpace(colorQueueEntry.Text)
		colorStatusResult.SetText(fmt.Sprintf("Querying CUPS for '%s'...", q))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := printerTester.CheckColorStatus(ctx, q)
			fyne.Do(func() {
				if err != nil {
					colorStatusResult.SetText(fmt.Sprintf("ERROR: %v", err))
				} else {
					colorStatusResult.SetText(fmt.Sprintf("SUCCESS:\n%s", out))
				}
			})
		}()
	})

	testColorPrintBtn := widget.NewButtonWithIcon("Print Test Swatch", theme.DocumentPrintIcon(), func() {
		q := strings.TrimSpace(colorQueueEntry.Text)
		colorStatusResult.SetText(fmt.Sprintf("Generating and sending test color swatch to '%s'...", q))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			jobID, err := printerTester.TestColorPrint(ctx, q)
			fyne.Do(func() {
				if err != nil {
					colorStatusResult.SetText(fmt.Sprintf("PRINT FAILED: %v", err))
				} else {
					colorStatusResult.SetText(fmt.Sprintf("PRINT SUCCESS!\nJob ID: %s\nDispatched to CUPS queue.", jobID))
				}
			})
		}()
	})

	colorCard := widget.NewCard(
		"Color Printer (CUPS / Epson)",
		"Test CUPS print queue and send full-color test pattern",
		container.NewVBox(
			container.NewBorder(nil, nil, widget.NewLabel("Printer Queue:"), nil, colorQueueEntry),
			container.NewGridWithColumns(2, checkColorBtn, testColorPrintBtn),
			colorDiagHeader,
			colorStatusBox,
		),
	)

	// --- Thermal Printer Card ---
	thermalTargetEntry := widget.NewEntry()
	targetVal := cfg.DefaultThermalPrinterMAC
	if targetVal == "" {
		targetVal = cfg.DefaultThermalPrinterName
	}
	thermalTargetEntry.SetText(targetVal)
	thermalTargetEntry.SetPlaceHolder("86:67:7A:7C:0A:56 or /dev/cu.RPP02N")

	thermalStatusResult := widget.NewLabel("Ready for diagnostics. Click below to verify device or print test receipt.")
	thermalStatusResult.TextStyle = fyne.TextStyle{Monospace: true}
	thermalStatusResult.Wrapping = fyne.TextWrapWord
	thermalStatusResult.Selectable = true

	thermalStatusBg := canvas.NewRectangle(color.RGBA{R: 10, G: 12, B: 16, A: 255})
	thermalStatusBox := container.NewThemeOverride(
		container.NewStack(thermalStatusBg, container.NewPadded(thermalStatusResult)),
		&terminalTheme{Theme: theme.DefaultTheme()},
	)

	copyThermalBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(thermalStatusResult.Text)
	})
	copyThermalBtn.Importance = widget.LowImportance

	thermalDiagHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Diagnostic Output:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		copyThermalBtn,
	)

	checkThermalBtn := widget.NewButtonWithIcon("Check Connection", theme.SearchIcon(), func() {
		target := strings.TrimSpace(thermalTargetEntry.Text)
		thermalStatusResult.SetText(fmt.Sprintf("Testing connectivity for '%s'...", target))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err := printerTester.CheckThermalStatus(ctx, target)
			fyne.Do(func() {
				if err != nil {
					thermalStatusResult.SetText(fmt.Sprintf("ERROR: %v", err))
				} else {
					thermalStatusResult.SetText(fmt.Sprintf("SUCCESS:\n%s", out))
				}
			})
		}()
	})

	testThermalPrintBtn := widget.NewButtonWithIcon("Print Test Receipt", theme.DocumentPrintIcon(), func() {
		target := strings.TrimSpace(thermalTargetEntry.Text)
		thermalStatusResult.SetText(fmt.Sprintf("Generating and sending test receipt to '%s'...", target))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			jobID, err := printerTester.TestThermalPrint(ctx, target)
			fyne.Do(func() {
				if err != nil {
					thermalStatusResult.SetText(fmt.Sprintf("PRINT FAILED: %v", err))
				} else {
					thermalStatusResult.SetText(fmt.Sprintf("PRINT SUCCESS!\nJob ID: %s\nDispatched to thermal printer.", jobID))
				}
			})
		}()
	})

	thermalCard := widget.NewCard(
		"Thermal Printer (ESC/POS / Bluetooth)",
		"Test Bluetooth MAC, serial device path, or network socket",
		container.NewVBox(
			container.NewBorder(nil, nil, widget.NewLabel("Target (MAC/Path):"), nil, thermalTargetEntry),
			container.NewGridWithColumns(2, checkThermalBtn, testThermalPrintBtn),
			thermalDiagHeader,
			thermalStatusBox,
		),
	)

	printerTestingTab := container.NewVScroll(
		container.NewPadded(
			container.NewVBox(
				colorCard,
				widget.NewSeparator(),
				thermalCard,
			),
		),
	)

	// ==================== 3. LIVE LOGS TAB ====================
	logsEntry := widget.NewMultiLineEntry()
	logsEntry.TextStyle = fyne.TextStyle{Monospace: true}
	logsEntry.Disable()
	logsEntry.SetText(logStream.String())

	autoScrollCheck := widget.NewCheck("Auto-scroll", nil)
	autoScrollCheck.SetChecked(true)

	clearLogsBtn := widget.NewButtonWithIcon("Clear Logs", theme.DeleteIcon(), func() {
		logStream.Clear()
		logsEntry.SetText("")
	})

	copyLogsBtn := widget.NewButtonWithIcon("Copy All Logs", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(logsEntry.Text)
	})
	copyLogsBtn.Importance = widget.LowImportance

	logStream.RegisterListener(func(line string) {
		fyne.Do(func() {
			cur := logsEntry.Text
			if cur == "" {
				logsEntry.SetText(line)
			} else {
				logsEntry.SetText(cur + "\n" + line)
			}
			if autoScrollCheck.Checked {
				logsEntry.CursorRow = len(strings.Split(logsEntry.Text, "\n"))
			}
		})
	})

	logsControlBar := container.NewHBox(
		autoScrollCheck,
		clearLogsBtn,
		copyLogsBtn,
	)

	// High-contrast terminal styling: solid black background with vibrant green text
	terminalBg := canvas.NewRectangle(color.RGBA{R: 10, G: 12, B: 16, A: 255})
	terminalStack := container.NewStack(terminalBg, logsEntry)
	themedLogsView := container.NewThemeOverride(terminalStack, &terminalTheme{Theme: theme.DefaultTheme()})

	logsTab := container.NewBorder(
		logsControlBar,
		nil,
		nil,
		nil,
		themedLogsView,
	)

	// ==================== MAIN TABS & WINDOW ====================
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Printer Diagnostics", theme.SettingsIcon(), printerTestingTab),
		container.NewTabItemWithIcon("Live Logs", theme.DocumentIcon(), logsTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	mainContent := container.NewBorder(
		container.NewVBox(serverHeader, widget.NewSeparator()),
		nil,
		nil,
		nil,
		tabs,
	)

	w.SetContent(mainContent)

	// Automatically start the server on GUI launch
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = serverMgr.Start(cfg.ServerPort)
	}()

	w.SetOnClosed(func() {
		if serverMgr.IsRunning() {
			_ = serverMgr.Stop()
		}
	})

	w.ShowAndRun()
}

