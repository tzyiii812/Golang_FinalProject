# 🚀 梗圖與複製文搜尋引擎 (Meme & Copypasta Search Engine)

這是一個基於 Go 語言開發的全端專案，整合了多種爬蟲策略（靜態與動態網頁），將網路上的梗圖 (GIF) 與文字笑話 (PTT/Threads/Plurk) 彙整至本地資料庫，並提供一個 Web 介面供使用者搜尋或隨機抽取。

## 📂 檔案結構與作用

本專案由以下核心檔案組成：

| 檔案名稱 | 說明 |
| :--- | :--- |
| **`spider.go`** | **爬蟲主程式**。包含所有爬取邏輯：<br>1. **GIF 爬蟲**：使用 `Colly` 爬取靜態圖片網站。<br>2. **PTT 爬蟲**：使用 `Colly` 並設定 Cookie 繞過 18 禁驗證。<br>3. **動態爬蟲**：使用 `Chromedp` 控制瀏覽器，透過「上下震動滾動法」爬取 Threads 與 Plurk。 |
| **`main.go`** | **Web 伺服器入口**。使用 `Gin` 框架建立 API 與網頁伺服器。<br>負責處理前端的搜尋請求 (`/api/search`) 與隨機請求 (`/api/random`)。 |
| **`database.go`** | **資料庫核心**。定義了資料結構 (`ExportMeme`) 與 SQLite 操作邏輯 (初始化、新增、搜尋、隨機讀取)。 |
| **`index.html`** | **前端介面**。提供搜尋框、模式切換 (圖片/文字) 與結果展示卡片。內建防盜連機制 (`no-referrer`) 以確保圖片能正常顯示。 |
| **`memes.db`** | **資料庫檔案** (自動生成)。儲存所有爬取到的資料。 |
| **`results/`** | **備份資料夾** (自動生成)。爬蟲執行時會將每一筆資料額外存成 JSON 檔作為備份。 |

-----

## 🛠️ 事前準備 (環境設定)

在執行專案前，請確保您已安裝：

1.  **Go (Golang)**: [下載連結](https://go.dev/dl/)
2.  **Google Chrome 瀏覽器**: 用於動態網頁爬蟲。
3.  **GCC 編譯器**: 因為使用了 SQLite (`go-sqlite3`)，Windows 用戶通常需要安裝 [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) 才能成功編譯。

-----

## 🔑 關鍵步驟：啟動 Chrome 遠端除錯模式

為了讓爬蟲可以爬取 **Threads** 和 **Plurk**，我們需要手動開啟一個 Chrome 視窗供程式連線。這樣做的好處是：**你可以手動登入帳號**，讓爬蟲直接使用你的登入狀態，避免被網站阻擋。

### Windows 使用者：

1.  關閉所有目前已開啟的 Chrome 視窗。
2.  開啟「命令提示字元 (CMD)」或 PowerShell。
3.  輸入以下指令來啟動 Chrome (請依據您的安裝路徑調整)：

<!-- end list -->

```bash
"C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222 --user-data-dir="C:\selenium\ChromeProfile"
```

> **注意**：`--user-data-dir` 可以設定在你喜歡的任何空資料夾路徑，這樣會建立一個乾淨的或是保留登入資訊的設定檔。

4.  **重要**：在這個新開啟的 Chrome 視窗中，**前往 Threads 和 Plurk 網站並完成登入**。
5.  **不要關閉這個視窗**，將它最小化即可。

### Mac 使用者：

在終端機輸入：

```bash
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222 --user-data-dir="/tmp/chrome_dev_test"
```

-----

## 🚀 啟動方式

專案分為「爬蟲」與「網站」兩個部分，建議分開執行。

### 第一步：安裝依賴套件

在專案根目錄執行：

```bash
go mod init myproject  # 如果還沒初始化過
go get .               # 下載所有必要的套件 (colly, chromedp, gin, sqlite3 等)
```

### 第二步：執行爬蟲 (Spider)

確保上一步的 Chrome (Port 9222) 已經開啟，然後執行：

```bash
go run spider.go database.go
```

  * 程式會依序執行：GIF -\> Threads/Plurk -\> PTT。
  * 觀察終端機 (Terminal) 的輸出，確認資料有成功寫入 (`[Spider] ... 存入`)。
  * **注意**：Threads 和 Plurk 爬取時，你會看到那個 Chrome 視窗自動導航和滾動，**請勿干擾它**。

### 第三步：啟動網站伺服器 (Server)

當爬蟲執行完畢（或你想邊爬邊看結果），可以開啟另一個終端機視窗執行：

```bash
go run main.go database.go
```

  * 看到 `伺服器運行中: http://localhost:8080` 代表啟動成功。

-----

## 🖥️ 使用說明

1.  打開瀏覽器前往 `http://localhost:8080`。
2.  **搜尋功能**：
      * 輸入關鍵字，按下 Enter 或搜尋按鈕。
      * **模式切換**：可選擇「全部」、「只找圖片 (GIF)」或「只找文字 (PTT/Threads)」。
3.  **隨機功能**：
      * 按下「🎲 隨機抽取」，系統會依照當前選擇的模式，隨機顯示一則內容。

-----

## ❓ 常見問題排解

**Q1: 執行爬蟲時顯示 `connection refused` 或無法連線 Chrome？**

  * **A**: 請確認您是否已按照「關鍵步驟」使用指令開啟了 Chrome，並且 Port 設定為 `9222`。

**Q2: 爬 Threads 時顯示「新增 0 篇」？**

  * **A**: 這是正常的。
    1.  可能是網站尚未載入新內容（爬蟲會自動重試滾動）。
    2.  可能是您已經抓過這些資料了（程式有去重機制 `seenContent`）。
    3.  如果一直為 0，請確認該 Chrome 視窗是否**已登入** Threads。

**Q3: 圖片無法顯示 (破圖)？**

  * **A**: 請確認 `index.html` 的 `<head>` 中是否包含 `<meta name="referrer" content="no-referrer">`。這是為了繞過部分網站的防盜連機制。

**Q4: 執行時報錯 `undefined: InitDB` 或 `undefined: ExportMeme`？**

  * **A**: Go 語言編譯時需要包含所有相關檔案。請務必使用 `go run spider.go database.go` 或 `go run main.go database.go data_importer.go` 來執行，不能只打單一檔案名稱。