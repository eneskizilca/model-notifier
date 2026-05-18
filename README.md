# LLM Notifier

CLI tabanlı LLM araçları cevabını bitirince macOS bildirimi atar.

## Desteklenen araçlar

| Araç | Log yolu | Bitiş sinyali |
|------|----------|---------------|
| OpenCode | `~/.local/share/opencode/log/*.log` | `type=session.idle` |
| Claude Code | `~/.claude/projects/**/*.jsonl` | `"role":"assistant"` + `"type":"message"` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` | `"type":"turn.completed"` |
| Gemini CLI | `~/.gemini/tmp/<hash>/logs.json` | `"role":"model"` |
| Aider | `<proje>/.aider.chat.history.md` | `>` prompt satırı |
| Copilot CLI | `~/.copilot/session-state/<id>/events.jsonl` | `"type":"session.idle"` |

## Kurulum

```bash
git clone https://github.com/eneskizilca/llm-notifier
cd llm-notifier
go mod tidy
go build -o llm-notifier .
./llm-notifier
```

## Aider için proje dizini ayarı

Aider log dosyasını çalıştığı dizine yazar. `main.go` içinde `AiderDetector` satırını düzenle:

```go
{&AiderDetector{ProjectDir: "/Users/sen/proje"}, true},
```

## Codex CLI native notify (opsiyonel — daha hızlı)

Log izleme yerine Codex'in kendi notify mekanizmasını kullanmak için
`~/.codex/config.toml` dosyasına şunu ekle:

```toml
[notify]
program = "/path/to/llm-notifier-notify"
```

Ardından ayrı bir `llm-notifier-notify` binary derle:

```go
// notify-helper/main.go
package main
import ("os/exec"; "fmt")
func main() {
    exec.Command("osascript", "-e",
        `display notification "Codex cevabı tamamladı!" with title "LLM Notifier" sound name "Glass"`).Run()
}
```

## Claude Code hook (opsiyonel — daha hızlı)

`~/.claude/settings.json` dosyasına ekle:

```json
{
  "hooks": {
    "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "/path/to/llm-notifier-notify"}]}]
  }
}
```

## Yeni LLM eklemek

`detector.go` dosyasına `Detector` interface'ini implemente eden yeni bir struct ekle,
`main.go` içindeki `entries` listesine bir satır ekle. Başka hiçbir yere dokunma.

```go
type BenimLLM struct{}

func (d *BenimLLM) Name() string              { return "Benim LLM" }
func (d *BenimLLM) WatchDir() string          { home, _ := os.UserHomeDir(); return filepath.Join(home, ".benimlm", "logs") }
func (d *BenimLLM) FileExt() string           { return ".log" }
func (d *BenimLLM) IsDone(line string) bool   { return strings.Contains(line, "done_signal") }
func (d *BenimLLM) ResolveFile(dir string) string { return latestFileInDir(dir, ".log") }
```

## Mimari

```
Detector interface  (detector.go)
  ├── OpenCodeDetector     — ~/.local/share/opencode/log/
  ├── ClaudeCodeDetector   — ~/.claude/projects/
  ├── CodexDetector        — ~/.codex/sessions/
  ├── GeminiCLIDetector    — ~/.gemini/tmp/
  ├── AiderDetector        — <proje>/.aider.chat.history.md
  └── CopilotCLIDetector   — ~/.copilot/session-state/

Engine  (engine.go)
  ├── Her detector için ayrı goroutine
  ├── 500ms poll — fsnotify yok, sahte event yok
  ├── mtime kontrolü — dosya gerçekten değişmeden işlem yapılmaz
  ├── Başlangıçta dosya sonuna atla — geçmiş loglar bildirim tetiklemez
  ├── offset takibi — sadece yeni satırları oku
  └── 15s rotation kontrolü — log rotation / yeni session algılar

Systray  (main.go)
  └── Her detector için checkbox — menüden aç/kapat
```
