package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	pattern := "/Users/eneskizilca/Library/Application Support/Code/User/workspaceStorage/*/chatSessions/*.jsonl"
	fmt.Println("🔍 SRE Teşhis Aracı Başlatıldı...")
	fmt.Println("📂 Aranan Dizin:", pattern)

	matches, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Println("❌ Glob arama hatası:", err)
		return
	}

	if len(matches) == 0 {
		fmt.Println("❌ HATA: Hiç .jsonl dosyası bulunamadı! Yol yanlış olabilir.")
		return
	}

	var latestFile string
	var latestTime time.Time

	for _, file := range matches {
		info, err := os.Stat(file)
		if err == nil {
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = file
			}
		}
	}

	fmt.Printf("\n✅ Hedef Bulundu!\nDosya: %s\nSon Güncellenme: %v\n", latestFile, latestTime.Format("15:04:05"))
	fmt.Println("\n⏳ Canlı log akışı dinleniyor... Lütfen VSCode'a geçip Gemini'ye bir soru sorun.")
	fmt.Println(strings.Repeat("-", 60))

	file, err := os.Open(latestFile)
	if err != nil {
		fmt.Println("❌ Dosya açılamadı:", err)
		return
	}
	defer file.Close()

	// Mevcut logları atlayıp dosyanın sonuna gidiyoruz
	file.Seek(0, io.SeekEnd)

	keyword := `"modelState":{"value":1,"completedAt"`
	var buffer string

	for {
		buf := make([]byte, 512)
		n, err := file.Read(buf)
		
		if n > 0 {
			chunk := string(buf[:n])
			// Gelen her veriyi anında sarı renkle terminale basıyoruz ki ne okuduğumuzu görelim
			fmt.Printf("\033[33m%s\033[0m", chunk) 
			
			buffer += chunk

			if strings.Contains(buffer, keyword) {
				fmt.Println("\n\n🎉 [BİLDİRİM TETİKLENDİ] 'completedAt' SİNYALİ YAKALANDI! 🎉\n")
				buffer = "" // Buffer'ı sıfırla
			}

			// Buffer çok şişmesin diye limitliyoruz
			if len(buffer) > 5000 {
				buffer = buffer[len(buffer)-2000:]
			}
		}

		if err != nil {
			if err == io.EOF {
				time.Sleep(200 * time.Millisecond) // Sürekli dosyayı kontrol et
				continue
			}
			fmt.Println("\n❌ Okuma hatası:", err)
			break
		}
	}
}