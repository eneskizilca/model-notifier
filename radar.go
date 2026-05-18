package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sadece chatSessions klasörlerine giren süper hızlı tarayıcı
func findNewestLogFile() (string, time.Time) {
	baseDir := "/Users/eneskizilca/Library/Application Support/Code/User/workspaceStorage"
	var latestFile string
	var latestTime time.Time

	dirs, _ := os.ReadDir(baseDir)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		chatDir := filepath.Join(baseDir, d.Name(), "chatSessions")
		files, err := os.ReadDir(chatDir)
		if err == nil {
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".jsonl") {
					info, err := f.Info()
					if err == nil && info.ModTime().After(latestTime) {
						latestTime = info.ModTime()
						latestFile = filepath.Join(chatDir, f.Name())
					}
				}
			}
		}
	}
	return latestFile, latestTime
}

func main() {
	fmt.Println("📡 [RADAR AKTİF] VSCode JSONL Dosyaları İzleniyor...")
	fmt.Println("Lütfen IDE'ye geçip Gemini'ye bir soru sorun. Çıktılar buraya akacak.")
	fmt.Println(strings.Repeat("-", 60))

	var lastFile string
	var lastSize int64

	keyword := `"modelState":{"value":1` // Kelimeyi biraz daha kısalttık, daha geniş yakalasın diye

	for {
		newestFile, modTime := findNewestLogFile()

		if newestFile != "" {
			info, err := os.Stat(newestFile)
			if err == nil {
				// Dosya değişmişse veya yeni bir dosya gelmişse
				if newestFile != lastFile || info.Size() != lastSize {
					
					// Ekrana durum bilgisi bas
					fmt.Printf("\n⚡ \033[36mHaraket Algılandı!\033[0m\n")
					fmt.Printf("📁 Dosya: %s\n", filepath.Base(newestFile))
					fmt.Printf("⏱️ Zaman: %s | Boyut: %d byte\n", modTime.Format("15:04:05.000"), info.Size())

					// Dosyanın son 1000 byte'ını oku
					file, _ := os.Open(newestFile)
					
					var start int64 = 0
					if info.Size() > 1000 {
						start = info.Size() - 1000
					}
					file.Seek(start, io.SeekStart)
					
					buf := make([]byte, 1000)
					n, _ := file.Read(buf)
					content := string(buf[:n])
					file.Close()

					// Son okunan kısmı terminalde sarı renkte göster
					fmt.Printf("📝 \033[33mSon Satırlar:\033[0m\n%s\n", content)

					// Bitiş sinyali var mı?
					if strings.Contains(content, keyword) {
						fmt.Println("\n🎉 \033[42;30m [BİLDİRİM TETİKLENMELİ] 'modelState:1' BULUNDU! \033[0m 🎉")
					}

					fmt.Println(strings.Repeat("-", 40))

					lastFile = newestFile
					lastSize = info.Size()
				}
			}
		}
		// CPU'yu yormadan saniyede 2 kez tara
		time.Sleep(500 * time.Millisecond) 
	}
}