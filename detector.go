package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Detector: yeni bir CLI LLM eklemek için sadece bu interface'i implemente et.
type Detector interface {
	Name() string        // Menüde görünecek isim
	WatchDir() string    // İzlenecek dizin
	FileExt() string     // İzlenecek dosya uzantısı
	IsDone(line string) bool // Bu log satırı bitiş sinyali mi?
	ResolveFile(dir string) string // Dizinden aktif dosyayı seç
}

// ── 1. OpenCode ───────────────────────────────────────────────────────────────
// Log: ~/.local/share/opencode/log/YYYY-MM-DDTHHMMSS.log
// Sinyal: "type=session.idle" — LLM döngüsünden çıktı, tam bitti.

type OpenCodeDetector struct{}

func (d *OpenCodeDetector) Name() string { return "OpenCode" }

func (d *OpenCodeDetector) WatchDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode", "log")
}

func (d *OpenCodeDetector) FileExt() string { return ".log" }

func (d *OpenCodeDetector) IsDone(line string) bool {
	return strings.Contains(line, "type=session.idle")
}

func (d *OpenCodeDetector) ResolveFile(dir string) string {
	return latestFileInDir(dir, ".log")
}

// ── 2. Claude Code ────────────────────────────────────────────────────────────
// Log: ~/.claude/projects/<encoded-path>/<session-uuid>.jsonl
// Sinyal: "role":"assistant" içeren satır — asistan cevabı tamamlandı.
// NOT: Claude Code'un hook sistemi de var (Stop event).
// Bu detector hook kurmak istemeyenler için log tabanlı fallback'tir.
// Hook tabanlı kullanım için README'ye bakın.

type ClaudeCodeDetector struct{}

func (d *ClaudeCodeDetector) Name() string { return "Claude Code" }

func (d *ClaudeCodeDetector) WatchDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func (d *ClaudeCodeDetector) FileExt() string { return ".jsonl" }

// Claude Code JSONL formatı: {"type":"assistant","message":{"role":"assistant",...}}
// "role":"assistant" sadece tamamlanmış asistan mesajlarında geçer.
func (d *ClaudeCodeDetector) IsDone(line string) bool {
	return strings.Contains(line, `"role":"assistant"`) &&
		strings.Contains(line, `"type":"message"`)
}

// En son değiştirilen .jsonl dosyasını bul — recursive (projects altında subdirler var)
func (d *ClaudeCodeDetector) ResolveFile(dir string) string {
	return latestFileRecursive(dir, ".jsonl")
}

// ── 3. Codex CLI ──────────────────────────────────────────────────────────────
// Log: ~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<uuid>.jsonl
// Sinyal: "type":"turn.completed" — bir tur tamamlandı.
// NOT: Codex'in native `notify` mekanizması da var. README'de açıklandı.

type CodexDetector struct{}

func (d *CodexDetector) Name() string { return "Codex CLI" }

func (d *CodexDetector) WatchDir() string {
	home, _ := os.UserHomeDir()
	// CODEX_HOME override'ı destekle
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return filepath.Join(h, "sessions")
	}
	return filepath.Join(home, ".codex", "sessions")
}

func (d *CodexDetector) FileExt() string { return ".jsonl" }

// Codex JSONL formatı: {"type":"turn.completed","usage":{...}}
func (d *CodexDetector) IsDone(line string) bool {
	return strings.Contains(line, `"type":"turn.completed"`)
}

// sessions/YYYY/MM/DD/ altında recursive en güncel dosya
func (d *CodexDetector) ResolveFile(dir string) string {
	return latestFileRecursive(dir, ".jsonl")
}

// ── 4. Gemini CLI ─────────────────────────────────────────────────────────────
// Log: ~/.gemini/tmp/<project-hash>/logs.json
// Sinyal: logs.json güncellenir ve son entry "role":"model" içerir.
// Bu dosya JSONL değil, tek bir JSON array — tüm dosyayı okuyup son satıra bakarız.

type GeminiCLIDetector struct{}

func (d *GeminiCLIDetector) Name() string { return "Gemini CLI" }

func (d *GeminiCLIDetector) WatchDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "tmp")
}

func (d *GeminiCLIDetector) FileExt() string { return ".json" }

// Gemini logs.json'da her satır ya user ya model — son model satırı bitti demek.
func (d *GeminiCLIDetector) IsDone(line string) bool {
	return strings.Contains(line, `"role":"model"`) &&
		!strings.Contains(line, `"role":"user"`)
}

// ~/.gemini/tmp/<hash>/logs.json — en son değiştirilen hash dizinindeki logs.json
func (d *GeminiCLIDetector) ResolveFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(dir, e.Name(), "logs.json")
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = candidate
		}
	}
	return latest
}

// ── 5. Aider ─────────────────────────────────────────────────────────────────
// Log: <proje-dizini>/.aider.chat.history.md
// Sinyal: "> " ile başlayan satır — Aider prompt'a döndü, cevap tamamlandı.
// Aider log dosyasını çalıştığı dizine yazar; kullanıcı dizini config'de belirtir.

type AiderDetector struct {
	// ProjectDir: Aider'ın çalıştığı dizin.
	// Boş bırakılırsa home dizinine bakar.
	// main.go'da kullanıcıya göre doldurulabilir.
	ProjectDir string
}

func (d *AiderDetector) Name() string { return "Aider" }

func (d *AiderDetector) WatchDir() string {
	if d.ProjectDir != "" {
		return d.ProjectDir
	}
	home, _ := os.UserHomeDir()
	return home
}

func (d *AiderDetector) FileExt() string { return ".md" }

func (d *AiderDetector) IsDone(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), ">")
}

// Sabit isim, tek dizine bak — Walk yok, izin sorunu yok
func (d *AiderDetector) ResolveFile(dir string) string {
	target := filepath.Join(dir, ".aider.chat.history.md")
	if _, err := os.Stat(target); err == nil {
		return target
	}
	return ""
}

// ── 6. GitHub Copilot CLI ─────────────────────────────────────────────────────
// Log: ~/.copilot/session-state/<session-id>/events.jsonl
// Sinyal: "type":"session.idle" — tur tamamlandı, idle durumuna geçildi.
// Her session ayrı dizinde — en son değiştirilen dizini izle.

type CopilotCLIDetector struct{}

func (d *CopilotCLIDetector) Name() string { return "Copilot CLI" }

func (d *CopilotCLIDetector) WatchDir() string {
	// COPILOT_HOME override'ı destekle
	if h := os.Getenv("COPILOT_HOME"); h != "" {
		return filepath.Join(h, "session-state")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".copilot", "session-state")
}

func (d *CopilotCLIDetector) FileExt() string { return ".jsonl" }

// Copilot CLI events.jsonl formatı: {"type":"session.idle",...}
func (d *CopilotCLIDetector) IsDone(line string) bool {
	return strings.Contains(line, `"type":"session.idle"`)
}

// session-state/<uuid>/events.jsonl — en son değiştirilen session'ın events.jsonl'u
func (d *CopilotCLIDetector) ResolveFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(dir, e.Name(), "events.jsonl")
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = candidate
		}
	}
	return latest
}

// ── Yardımcı fonksiyonlar ─────────────────────────────────────────────────────

// latestFileInDir: dizindeki (recursive değil) en son değiştirilen dosyayı döndürür
func latestFileInDir(dir, ext string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}
	return latest
}

// latestFileRecursive: dizin ağacında en son değiştirilen dosyayı döndürür.
// Sadece belirli kök dizinler için kullanılır (home taraması yapılmaz).
func latestFileRecursive(root, ext string) string {
	var latest string
	var latestTime time.Time
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ext) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = path
		}
		return nil
	})
	return latest
}
