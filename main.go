package main

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/getlantern/systray"
)

func main() {
	systray.Run(onReady, onExit)
}

// Her detector için renk dairesi ve görünen isim
type detectorMeta struct {
	d       Detector
	active  bool
	emoji   string // logo rengine yakın daire
	label   string // menü etiketi
}

func onReady() {
	systray.SetTitle("🤖")
	systray.SetTooltip("LLM Notifier")

	metas := []detectorMeta{
		{&OpenCodeDetector{},   true,  "⚪", "OpenCode"},
		{&ClaudeCodeDetector{}, true,  "🟠", "Claude Code"},
		{&CodexDetector{},      true,  "🟢", "Codex CLI"},
		{&GeminiCLIDetector{},  true,  "🔵", "Gemini CLI"},
		{&AiderDetector{},      false, "🟡", "Aider"},
		{&CopilotCLIDetector{}, true,  "⚫", "Copilot CLI"},
	}

	states := make([]*watcherState, len(metas))
	for i, m := range metas {
		states[i] = newWatcherState(m.d, m.active)
	}

	for i, ws := range states {
		label := metas[i].emoji + " " + metas[i].label
		item := systray.AddMenuItemCheckbox(label, "İzlemeyi aç/kapat", metas[i].active)
		go safeToggle(item, ws)
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Çıkış", "Kapat")
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()

	startEngine(states, sendNotification)
}

// safeToggle: ClickedCh'yi güvenli şekilde dinler.
// getlantern/systray'de checkbox item'a tıklayınca menü kapanır (macOS native davranışı),
// bu normaldir — uygulama arka planda çalışmaya devam eder.
// ClickedCh kapanırsa (uygulama çıkışında) goroutine temiz şekilde sonlanır.
func safeToggle(item *systray.MenuItem, ws *watcherState) {
	for range item.ClickedCh {
		if item.Checked() {
			item.Uncheck()
			ws.isActive.Store(false)
		} else {
			item.Check()
			ws.isActive.Store(true)
		}
	}
}

func sendNotification(d Detector) {
	// Format: "{emoji} {isim} cevabı tamamladı! [{saat}]"
	emoji := detectorEmoji(d)
	timeStr := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("%s %s cevabı tamamladı! [%s]", emoji, d.Name(), timeStr)
	script := fmt.Sprintf(
		`display notification %q with title "LLM Notifier" sound name "Glass"`,
		msg,
	)
	_ = exec.Command("osascript", "-e", script).Run()
}

func detectorEmoji(d Detector) string {
	switch d.(type) {
	case *OpenCodeDetector:
		return "⚪"
	case *ClaudeCodeDetector:
		return "🟠"
	case *CodexDetector:
		return "🟢"
	case *GeminiCLIDetector:
		return "🔵"
	case *AiderDetector:
		return "🟡"
	case *CopilotCLIDetector:
		return "⚫"
	default:
		return "🤖"
	}
}

func onExit() {}