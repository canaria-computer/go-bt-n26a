package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"tinygo.org/x/bluetooth"
)

// アプリケーションの設定定数
const (
	buildMessage = "version 1.1 n26a-bt"
	port         = "2829"
	appName      = "go-bt-n26a"
	authURL      = "https://n26a_backend.mirai-th-kakenn.workers.dev/auth/login"
	logURL       = "https://n26a_backend.mirai-th-kakenn.workers.dev/number_of_people"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// グローバル変数
var (
	sseClients      = make(map[chan string]bool)
	sseClientsMutex sync.Mutex
	credential      string
	locateID        int
)

// Credentials 構造体：認証トークンと有効期限を保存するための構造体
type Credentials struct {
	Token string `json:"token"`
	Exp   string `json:"exp"`
}

func init() {
	flag.IntVar(&locateID, "locate", -1, "Location ID for logging")
	flag.Parse()

	if locateID == -1 {
		envID, err := strconv.Atoi(os.Getenv("N26A_BT_LOCATE_ID"))
		if err != nil || envID == -1 {
			fmt.Print("ロケーションIDを入力してください: ")
			var input string
			fmt.Scanln(&input)
			inputID, err := strconv.Atoi(input)
			if err != nil {
				fmt.Println("無効な入力です。デフォルト値の-1を使用します。")
			} else {
				locateID = inputID
			}
		} else {
			locateID = envID
		}
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := credentialScan(); err != nil {
		return fmt.Errorf("認証情報のスキャンに失敗しました: %w", err)
	}

	go bluetoothScanRoutine()
	go webServerRoutine()

	select {}
}

func credentialScan() error {
	savedToken, savedExpiry := loadSavedCredentials()
	if savedToken != "" && !isTokenExpired(savedExpiry) {
		fmt.Println("クレデンシャルを再利用しました")
		credential = savedToken
		return nil
	}
	fmt.Println("トークンの有効期限が切れています")

	userid, password := getCredentialsFromEnv()

	token, expiry, err := loginAndGetToken(userid, password)
	if err != nil {
		return fmt.Errorf("ログインに失敗しました: %w", err)
	}

	if err := saveCredentials(token, expiry); err != nil {
		fmt.Println("警告: 認証情報の保存に失敗しました:", err)
	}

	credential = token
	return nil
}

func getCredentialsFromEnv() (string, string) {
	userid := os.Getenv("N26A_BT_USERID")
	password := os.Getenv("N26A_BT_PASS")

	if userid == "" {
		fmt.Print("ユーザーIDを入力してください: ")
		fmt.Scanf("%s\n", &userid)
	}
	if password == "" {
		fmt.Print("パスワードを入力してください: ")
		fmt.Scanf("%s\n", &password)
	}

	return userid, password
}

func isTokenExpired(expiryStr string) bool {
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(expiry)
}

func loginAndGetToken(userid, password string) (string, string, error) {
	data := map[string]string{
		"id":       userid,
		"password": password,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", "", fmt.Errorf("ログインデータのJSONエンコードに失敗しました: %w", err)
	}

	resp, err := http.Post(authURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("ログインリクエストの送信に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("ログインに失敗しました。ステータスコード: %d", resp.StatusCode)
	}

	var result struct {
		Message string `json:"message"`
		Token   struct {
			Token string `json:"token"`
			Exp   string `json:"exp"`
		} `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("ログインレスポンスのデコードに失敗しました: %w", err)
	}

	return result.Token.Token, result.Token.Exp, nil
}

func loadSavedCredentials() (string, string) {
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		token, exp := loadFromDir(filepath.Join(cacheDir, appName))
		fmt.Println("過去のクレデンシャルを発見")
		if token != "" && exp != "" {
			return token, exp
		}
	} else {
		fmt.Println("過去のクレデンシャルがありません。")
	}

	return "", ""
}

func loadFromDir(dir string) (string, string) {
	filePath := filepath.Join(dir, "credentials.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", ""
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", ""
	}

	return creds.Token, creds.Exp
}

func saveCredentials(token, expiry string) error {
	creds := Credentials{
		Token: token,
		Exp:   expiry,
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("認証情報のJSONエンコードに失敗しました: %w", err)
	}

	cacheDir, err := os.UserCacheDir()
	if err == nil {
		if err := saveToDir(filepath.Join(cacheDir, appName), data); err == nil {
			return nil
		}
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		return saveToDir(filepath.Join(configDir, appName), data)
	}

	return fmt.Errorf("認証情報の保存に失敗しました: %w", err)
}

func saveToDir(dir string, data []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("ディレクトリの作成に失敗しました: %w", err)
	}

	filePath := filepath.Join(dir, "credentials.json")
	return os.WriteFile(filePath, data, 0600)
}

func bluetoothScanRoutine() {
	counter := 0
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		fmt.Println("BLEスタックの有効化に失敗しました:", err)
		return
	}

	for {
		devices := make(map[string]string)
		scanDuration := 10 * time.Second
		scanDone := make(chan bool)

		go performBluetoothScan(adapter, devices, scanDone)
		go stopScanAfterDuration(adapter, scanDuration, scanDone)

		<-scanDone
		if counter%5 == 0 {
			if err := sendLog(devices); err != nil {
				fmt.Println("ログの送信中にエラーが発生しました:", err)
			}
		}
		counter++

		// 常にデバイス情報をブロードキャスト
		if err := broadcastDevices(devices); err != nil {
			fmt.Println("デバイス情報のブロードキャスト中にエラーが発生しました:", err)
		}

		time.Sleep(50 * time.Second) // 次のスキャンまで待機
	}
}

func performBluetoothScan(adapter *bluetooth.Adapter, devices map[string]string, done chan<- bool) {
	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		if device.RSSI >= -65 {
			devices[device.Address.String()] = device.LocalName()
		}
	})
	if err != nil {
		fmt.Println("スキャン中にエラーが発生しました:", err)
	}
	done <- true
}

func stopScanAfterDuration(adapter *bluetooth.Adapter, duration time.Duration, done chan<- bool) {
	time.Sleep(duration)
	err := adapter.StopScan()
	if err != nil {
		fmt.Println("スキャン停止中にエラーが発生しました:", err)
	}
	done <- true
}

func sendLog(devices map[string]string) error {
	logData := struct {
		LocateID  int `json:"locateId"`
		SrcTypeID int `json:"srcTypeId"`
		Count     int `json:"count"`
	}{
		LocateID:  locateID,
		SrcTypeID: 3,
		Count:     len(devices),
	}

	jsonData, err := json.Marshal(logData)
	if err != nil {
		return fmt.Errorf("ログデータのJSONエンコードに失敗しました:%w", err)
	}

	req, err := http.NewRequest("POST", logURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ログリクエストの作成に失敗しました:%w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)

	client := &http.Client{
		Timeout: 1 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ログリクエストの送信に失敗しました:%w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ログの送信に失敗しました:%w", err)
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ログの送信に失敗しました。ステータスコード:%d 、レスポンス:%s", resp.StatusCode, string(body))
	}

	fmt.Printf("[%d]ログ送信成功\n", resp.StatusCode)

	return nil
}

// broadcastDevices関数：スキャンされたデバイス情報をSSEクライアントにブロードキャスト
func broadcastDevices(devices map[string]string) error {
	jsonData, err := json.Marshal(devices)
	if err != nil {
		return fmt.Errorf("デバイス情報のJSONエンコードに失敗しました: %w", err)
	}

	broadcastToSSEClients(string(jsonData))
	return nil
}

// webServerRoutine関数：Webサーバーを起動し、ルーティングを設定
func webServerRoutine() {
	http.Handle("/static/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/", webRootHandler)
	http.HandleFunc("/events", sseHandler)

	fmt.Printf("Webサーバーを http://localhost:%s/ で起動しています\n", port)
	if err := http.ListenAndServe("localhost:"+port, nil); err != nil {
		fmt.Printf("Webサーバーの起動中にエラーが発生しました: %v\n", err)
	}
}

// webRootHandler関数：ルートパスへのリクエストを処理
func webRootHandler(w http.ResponseWriter, r *http.Request) {
	nonce := generateRandomBase64(32)
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		http.Error(w, "内部サーバーエラー", http.StatusInternalServerError)
		return
	}

	setSecurityHeaders(w, nonce)

	data := struct {
		Title   string
		Message string
		Nonce   string
	}{
		Title:   "Bluetooth 人数カウント",
		Message: buildMessage,
		Nonce:   nonce,
	}

	if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "内部サーバーエラー", http.StatusInternalServerError)
	}
}

// setSecurityHeaders関数：セキュリティ関連のHTTPヘッダーを設定
func setSecurityHeaders(w http.ResponseWriter, nonce string) {
	policy := fmt.Sprintf("script-src 'strict-dynamic' 'nonce-%[1]s' 'unsafe-inline' https:; style-src 'self' 'nonce-%[1]s' 'unsafe-inline'; frame-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'; style-src-elem 'nonce-%[1]s'; default-src *;upgrade-insecure-requests;", nonce)
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Xss-Protection", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "max-age=20, private, stale-while-revalidate=3600")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
}

// sseHandler関数：Server-Sent Events (SSE) のハンドラ
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

// broadcastToSSEClients関数：全てのSSEクライアントにメッセージをブロードキャスト
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

// generateRandomBase64関数：指定された長さのランダムなBase64文字列を生成
func generateRandomBase64(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("ランダムな値の読み取りに失敗しました: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
