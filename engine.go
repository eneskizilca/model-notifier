package main

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type watcherState struct {
	detector Detector
	isActive atomic.Bool

	mu             sync.Mutex
	lastDoneAt     time.Time
	lastFile       string
	lastOffset     int64
	lastMtime      time.Time
	initialized    bool // başlangıçta dosya sonuna atlandı mı?
}

func newWatcherState(d Detector, active bool) *watcherState {
	ws := &watcherState{detector: d}
	ws.isActive.Store(active)
	return ws
}

func (ws *watcherState) tryNotify(cooldown time.Duration) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if time.Since(ws.lastDoneAt) < cooldown {
		return false
	}
	ws.lastDoneAt = time.Now()
	return true
}

// ── Engine ────────────────────────────────────────────────────────────────────

func startEngine(states []*watcherState, notify func(Detector)) {
	for _, ws := range states {
		go runWatcher(ws, notify)
	}
}

func runWatcher(ws *watcherState, notify func(Detector)) {
	const (
		pollInterval = 500 * time.Millisecond
		cooldown     = 5 * time.Second
		rotateCheck  = 15 * time.Second
	)

	var lastRotateCheck time.Time

	// İlk başlangıç: mevcut dosyayı bul ve sonuna atla
	// Böylece geçmiş loglar bildirim tetiklemez
	initWatcher(ws)

	for {
		if !ws.isActive.Load() {
			time.Sleep(pollInterval)
			continue
		}

		// Periyodik olarak aktif dosyayı yeniden çöz (log rotation / yeni session)
		if time.Since(lastRotateCheck) > rotateCheck {
			newFile := ws.detector.ResolveFile(ws.detector.WatchDir())
			if newFile != "" && newFile != ws.lastFile {
				// Yeni dosya — sonuna atla (geçmiş log olabilir)
				ws.lastFile = newFile
				ws.lastOffset = 0
				if info, err := os.Stat(newFile); err == nil {
					ws.lastOffset = info.Size()
					ws.lastMtime = info.ModTime()
				}
			}
			lastRotateCheck = time.Now()
		}

		if ws.lastFile == "" {
			ws.lastFile = ws.detector.ResolveFile(ws.detector.WatchDir())
			if ws.lastFile == "" {
				time.Sleep(2 * time.Second)
				continue
			}
			// İlk kez bulunan dosya — sonuna atla
			if info, err := os.Stat(ws.lastFile); err == nil {
				ws.lastOffset = info.Size()
				ws.lastMtime = info.ModTime()
			}
		}

		// Dosya gerçekten değişti mi? (mtime kontrolü — sahte event filtresi)
		info, err := os.Stat(ws.lastFile)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		if !info.ModTime().After(ws.lastMtime) {
			time.Sleep(pollInterval)
			continue
		}
		ws.lastMtime = info.ModTime()

		done := readNewLines(ws.lastFile, &ws.lastOffset, ws.detector.IsDone)
		if done && ws.tryNotify(cooldown) {
			notify(ws.detector)
		}

		time.Sleep(pollInterval)
	}
}

// initWatcher: goroutine başlarken çağrılır.
// Mevcut dosyayı bulur ve sonuna atlar — geçmiş logları yoksayar.
func initWatcher(ws *watcherState) {
	file := ws.detector.ResolveFile(ws.detector.WatchDir())
	if file == "" {
		return
	}
	ws.lastFile = file
	if info, err := os.Stat(file); err == nil {
		ws.lastOffset = info.Size()
		ws.lastMtime = info.ModTime()
	}
}

// readNewLines: dosyayı offset'ten itibaren okur.
// IsDone dönen herhangi bir satır bulunursa true döner.
// offset güncellenir — bir sonraki çağrıda kaldığı yerden devam eder.
func readNewLines(path string, offset *int64, isDone func(string) bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false
	}

	// Dosya küçüldüyse (truncate/rotation) baştan başla
	if info.Size() < *offset {
		*offset = 0
	}

	if _, err := f.Seek(*offset, io.SeekStart); err != nil {
		return false
	}

	found := false
	scanner := bufio.NewScanner(f)
	// Büyük log satırları için buffer'ı genişlet (Codex/Claude Code JSON satırları uzun olabilir)
	buf := make([]byte, 512*1024)
	scanner.Buffer(buf, 512*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if isDone(line) {
			found = true
			// found'u true yaptık ama okumaya devam ediyoruz
			// böylece offset dosyanın sonuna kadar ilerliyor
		}
	}

	// Yeni offset'i kaydet
	if pos, err := f.Seek(0, io.SeekCurrent); err == nil {
		*offset = pos
	}

	return found
}
