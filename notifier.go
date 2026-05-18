package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/getlantern/systray"
)

type WatchConfig struct {
	ID        string
	IDEName   string
	ModeName  string
	ModelName string
	TargetDir string
	Keyword   string
	isActive  atomic.Bool

	mu             sync.Mutex
	lastNotifiedID string
	lastNotifiedAt time.Time
}

func (w *WatchConfig) Active() bool     { return w.isActive.Load() }
func (w *WatchConfig) SetActive(v bool) { w.isActive.Store(v) }

func (w *WatchConfig) tryNotify(id string, cooldown time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if id == w.lastNotifiedID && time.Since(w.lastNotifiedAt) < cooldown {
		return false
	}
	w.lastNotifiedID = id
	w.lastNotifiedAt = time.Now()
	return true
}

func (w *WatchConfig) resetNotifyID() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastNotifiedID = ""
}

var watchers = []*WatchConfig{
	{
		ID:        "opencode_qwen",
		IDEName:   "OpenCode",
		ModeName:  "Terminal CLI",
		ModelName: "Qwen 3.6",
		TargetDir: "/Users/eneskizilca/.local/share/opencode/log",
		Keyword:   "type=session.idle",
	},
	{
		ID:        "vscode_copilot_gemini",
		IDEName:   "VSCode",
		ModeName:  "Copilot Chat",
		ModelName: "Gemini 3.1 Pro",
		TargetDir: "/Users/eneskizilca/Library/Application Support/Code/User/workspaceStorage",
		Keyword:   `"completedAt":`,
	},
}

func init() {
	watchers[0].SetActive(false)
	watchers[1].SetActive(true)
}

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("🤖")
	systray.SetTooltip("Model Notifier")

	ideMenus := make(map[string]*systray.MenuItem)

	for _, w := range watchers {
		if _, exists := ideMenus[w.IDEName]; !exists {
			ideMenus[w.IDEName] = systray.AddMenuItem(w.IDEName, "")
		}
		subMenuText := fmt.Sprintf("%s (%s)", w.ModeName, w.ModelName)
		subItem := ideMenus[w.IDEName].AddSubMenuItemCheckbox(subMenuText, "İzlemeyi Aç/Kapat", w.Active())

		go watchWorker(w)

		go func(item *systray.MenuItem, config *WatchConfig) {
			for range item.ClickedCh {
				if item.Checked() {
					item.Uncheck()
					config.SetActive(false)
				} else {
					item.Check()
					config.SetActive(true)
				}
			}
		}(subItem, w)
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Çıkış", "Kapat")
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

func watchWorker(config *WatchConfig) {
	for {
		if !config.Active() {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if config.IDEName == "VSCode" {
			runVSCodeWatcher(config)
		} else {
			runSimpleWatcher(config, config.TargetDir)
		}

		time.Sleep(1 * time.Second)
	}
}

// runVSCodeWatcher: workspaceStorage altındaki TÜM chatSessions dizinlerini
// tek bir watcher ile izler. "En güncel dizini bul" mantığı tamamen kaldırıldı.
func runVSCodeWatcher(config *WatchConfig) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	_ = watcher.Add(config.TargetDir)
	addAllChatSessions(watcher, config.TargetDir)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		if !config.Active() {
			return
		}

		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Create != 0 && isDir(event.Name) {
				chatDir := filepath.Join(event.Name, "chatSessions")
				if isDir(chatDir) {
					_ = watcher.Add(chatDir)
				}
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if !strings.HasSuffix(event.Name, ".jsonl") {
				continue
			}

			handleVSCode(config, event.Name)

		case <-watcher.Errors:
			return

		case <-ticker.C:
			addAllChatSessions(watcher, config.TargetDir)
		}
	}
}

func addAllChatSessions(watcher *fsnotify.Watcher, baseDir string) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}
	for _, d := range entries {
		if !d.IsDir() {
			continue
		}
		chatDir := filepath.Join(baseDir, d.Name(), "chatSessions")
		if isDir(chatDir) {
			_ = watcher.Add(chatDir)
		}
	}
}

func runSimpleWatcher(config *WatchConfig, watchDir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	if err := watcher.Add(watchDir); err != nil {
		return
	}

	for {
		if !config.Active() {
			return
		}
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if !strings.HasSuffix(event.Name, ".log") {
				continue
			}
			handleOpenCode(config, event.Name)

		case <-watcher.Errors:
			return
		}
	}
}

// Dosyanın son görülen mtime'ını tut
var fileModTimes sync.Map // map[string]time.Time

func handleVSCode(config *WatchConfig, filePath string) {
	// Dosya gerçekten değişti mi kontrol et
	info, err := os.Stat(filePath)
	if err != nil {
		return
	}
	
	lastMod, _ := fileModTimes.Load(filePath)
	if lastMod != nil && !info.ModTime().After(lastMod.(time.Time)) {
		return // mtime değişmemiş, sahte event — yoksay
	}
	fileModTimes.Store(filePath, info.ModTime())

	completionID := extractCompletionID(filePath)
	if completionID == "" {
		return
	}
	if config.tryNotify(completionID, 10*time.Second) {
		fireNotification(config)
	}
}

func handleOpenCode(config *WatchConfig, filePath string) {
	if !checkKeywordInFile(filePath, config.Keyword) {
		return
	}
	if config.tryNotify("qwen_idle", 3*time.Second) {
		fireNotification(config)
		time.AfterFunc(3*time.Second, config.resetNotifyID)
	}
}

var completedAtRe = regexp.MustCompile(`"k":\["requests",(\d+),"modelState"\],"v":\{"value":1,"completedAt":(\d+)\}`)

func extractCompletionID(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return ""
	}

	const windowSize = 102400
	start := stat.Size() - windowSize
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}

	buf := make([]byte, windowSize)
	n, _ := f.Read(buf)
	matches := completedAtRe.FindAllStringSubmatch(string(buf[:n]), -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	return last[1] + ":" + last[2]
}

func checkKeywordInFile(filePath, keyword string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return false
	}

	const windowSize = 2048
	start := stat.Size() - windowSize
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return false
	}

	buf := make([]byte, windowSize)
	n, _ := f.Read(buf)
	return strings.Contains(string(buf[:n]), keyword)
}

func fireNotification(config *WatchConfig) {
	timeStr := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("[%s] %s içindeki %s cevabı tamamladı!", timeStr, config.IDEName, config.ModelName)
	script := fmt.Sprintf(
		`display notification %q with title "Model Notifier" subtitle %q sound name "Glass"`,
		msg, config.ModeName,
	)
	_ = exec.Command("osascript", "-e", script).Run()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func onExit() {}