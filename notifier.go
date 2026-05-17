package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const opencodeLogDir = "/Users/eneskizilca/.local/share/opencode/log"

// Loglardan yakaladığımız o kesin bitiş sinyali
const targetKeyword = "type=session.idle"

func main() {
	logFile := findLatestLogFile(opencodeLogDir)
	if logFile == "" {
		fmt.Println("Aktif bir OpenCode log dosyası bulunamadı.")
		return
	}

	fmt.Printf("Model izleme monitörü aktif. İzlenen log dosyası: %s\n", logFile)

	file, err := os.Open(logFile)
	if err != nil {
		fmt.Printf("Dosya açılamadı: %v\n", err)
		return
	}
	defer file.Close()

	// Eski logları okumamak için dosyanın sonuna atla
	file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(500 * time.Millisecond) // CPU dostu uyku
				continue
			}
			fmt.Println("Okuma hatası:", err)
			break
		}

		// İstiyorsan debug için aşağıdaki satırı açık bırakabilirsin
		// fmt.Print("Log Okundu: ", line)

		if strings.Contains(line, targetKeyword) {
			fmt.Println("\n>>> [SİSTEM] ANAHTAR KELİME YAKALANDI! Bildirim tetikleniyor... <<<")
			sendNotification("OpenCode", "Qwen İşlemi Bitirdi!", "Model yanıt vermeyi tamamladı, seni bekliyor.")
			time.Sleep(5 * time.Second)
		}
	}
}

// Dosya isimleri (2026-05-17T130939.log) alfabetik olarak tarihe denk geldiği için
// ModTime yerine doğrudan isim sıralaması kullanıyoruz. Bu sayede hata payı sıfıra iniyor.
func findLatestLogFile(dir string) string {
	var latestFile string

	files, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".log") {
			if file.Name() > filepath.Base(latestFile) {
				latestFile = filepath.Join(dir, file.Name())
			}
		}
	}

	return latestFile
}

func sendNotification(title, subtitle, message string) {
	appleScript := fmt.Sprintf(`display notification "%s" with title "%s" subtitle "%s" sound name "Glass"`, message, title, subtitle)
	cmd := exec.Command("osascript", "-e", appleScript)
	
	// Sadece çalıştırmakla kalma, varsa AppleScript'in kendi hatalarını da yakala
	out, err := cmd.CombinedOutput() 
	if err != nil {
		fmt.Printf(">>> [HATA] Bildirim Gönderilemedi: %v | Detay: %s\n", err, string(out))
	} else {
		fmt.Println(">>> [BAŞARILI] Bildirim macOS'e iletildi.")
	}
}