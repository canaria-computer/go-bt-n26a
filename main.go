package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"tinygo.org/x/bluetooth"
)

const BuildMessage = "version 1.0 n26a-bt"

const port = "8080"

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var (
	sseClients      = make(map[chan string]bool)
	sseClientsMutex sync.Mutex
)

func main() {
	// Bluetoothスキャン用のゴルーチン
	go bluetoothScanRoutine()
	// Webサーバー用のゴルーチン
	go webServerRoutine()
	// メインゴルーチンを終了させない
	select {}
}

func credentialScan() {

}

func bluetoothScanRoutine() {
	adapter := bluetooth.DefaultAdapter
	must("enable BLE stack", adapter.Enable())

	for {
		devices := make(map[string]string)
		scanDuration := 10 * time.Second
		scanDone := make(chan bool)

		// スキャン開始
		go func() {
			err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
				if device.RSSI >= -65 {
					devices[device.Address.String()] = device.LocalName()
				}
			})
			if err != nil {
				fmt.Println("Error during scan:", err)
			}
		}()

		// 10秒後にスキャンを停止
		go func() {
			time.Sleep(scanDuration)
			adapter.StopScan()
			scanDone <- true
		}()

		// スキャン完了を待つ
		<-scanDone

		// スキャン結果をログサーバーに送信
		sendLog(devices)

		// スキャン結果をSSEクライアントに送信
		jsonData, err := json.Marshal(devices)
		if err == nil {
			broadcastToSSEClients(string(jsonData))
		}

		// 50秒待機（合計60秒のインターバル）
		time.Sleep(50 * time.Second)
	}

}

func sendLog(devices map[string]string) {
	jsonData, err := json.Marshal(devices)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	resp, err := http.Post("https://example.com/log", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error sending log:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Log sent successfully")
}

func webServerRoutine() {
	http.Handle("/static/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/", webRootHandler)
	http.HandleFunc("/events", sseHandler)
	fmt.Printf("Starting web server on http://localhost:%s/\n", port)
	err := http.ListenAndServe("localhost:8080", nil)
	must("Error starting web server:", err)
}

func webRootHandler(w http.ResponseWriter, r *http.Request) {
	nonce := generateRandomBase64(32)
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	must("template file parse", err)
	policy := fmt.Sprintf("script-src 'strict-dynamic' 'nonce-%[1]s' 'unsafe-inline' https:; style-src 'self' 'nonce-%[1]s' 'unsafe-inline'; frame-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'; style-src-elem 'nonce-%[1]s'; default-src *;upgrade-insecure-requests;", nonce)
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Xss-Protection", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "max-age=20, private, stale-while-revalidate=3600")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	// w.Header().Set("", "")
	data := struct {
		Title   string
		Message string
		Nonce   string
	}{
		Title:   "Embedded Template Example",
		Message: BuildMessage,
		Nonce:   nonce,
	}

	err = tmpl.ExecuteTemplate(w, "index.html", data)
	must("template execute", err)
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan string)

	sseClientsMutex.Lock()
	sseClients[clientChan] = true
	sseClientsMutex.Unlock()

	defer func() {
		sseClientsMutex.Lock()
		delete(sseClients, clientChan)
		sseClientsMutex.Unlock()
	}()

	for {
		id, _ := uuid.NewV7()
		idstr := id.String()
		select {
		case <-r.Context().Done():
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\nid:%s\n\n", msg, idstr)
			w.(http.Flusher).Flush()
		}
	}
}

func broadcastToSSEClients(message string) {
	sseClientsMutex.Lock()
	defer sseClientsMutex.Unlock()

	for clientChan := range sseClients {
		select {
		case clientChan <- message:
		default:
			// クライアントがメッセージを受信できない場合は無視
		}
	}
}

func generateRandomBase64(length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	must("read random", err)
	return base64.RawURLEncoding.EncodeToString(b)
}

func must(action string, err error) {
	if err != nil {
		panic("failed to " + action + ": " + err.Error())
	}
}
