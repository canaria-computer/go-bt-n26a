# ソースファイル名（適宜変更してください）
SOURCE_FILE="main.go"

# Linux AMD64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o dist/linux-amd64/app_linux-amd64 $SOURCE_FILE

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -trimpath -o dist/linux-arm64/app_linux-arm64 $SOURCE_FILE

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o dist/darwin-amd64/app_darwin-amd64 $SOURCE_FILE

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -trimpath -o dist/darwin-arm64/app_darwin-arm64 $SOURCE_FILE

# Windows 386
GOOS=windows GOARCH=386 go build -ldflags="-s -w" -trimpath -o dist/win386/app_win386.exe $SOURCE_FILE

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o dist/winamd64/app_winamd64.exe $SOURCE_FILE

echo "クロスコンパイルが完了しました。バイナリは dist/ ディレクトリ内にあります。"