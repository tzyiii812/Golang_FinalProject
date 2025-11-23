# Golang_FinalProject
數據獲取： 使用 Go 語言爬蟲 (spider.go) 批量收集網路上的 GIF 數據。  
數據持久化： 使用 SQLite 資料庫 (project_memes.db) 儲存數據，實現高效查詢。  
API 服務： 提供兩個核心 API：GET /api/search 和 GET /api/random。  
前端展示： 使用原生 HTML/CSS/JavaScript 呼叫後端 API，動態顯示內容。   

main.go	Web 服務入口，負責啟動 Gin 路由器和 API 服務。  
database.go	資料庫核心層，包含 Meme 結構定義、SQLite 初始化、搜尋和隨機查詢函式。  
data_importer.go	數據匯入工具，用於讀取 memes_raw_data.json 並插入資料庫。  
spider.go	爬蟲工具，執行數據收集並輸出為 JSON 格式。  
index.html	前端展示頁面，使用 JavaScript 呼叫後端 API。  
memes_raw_data.json	爬蟲輸出的原始數據檔案（由 spider.go 生成）。  

 執行：  
 ```
 # 下載所有必要的 Go 函式庫（Gin, SQLite 驅動等）
 go mod tidy
 # 運行爬蟲程式碼，生成 memes_raw_data.json
 go run spider.go database.go
 # 運行 Web 服務：同時編譯所有核心檔案
go run main.go database.go data_importer.go
 ```
 如果啟動成功，終端機將輸出：
 ```
 [INFO] 資料庫初始化成功...
--- 資料匯入完成！總共匯入 XX 筆資料 ---
Web 服務已啟動，請訪問 http://localhost:8080/...
[GIN] Listening and serving HTTP on :8080
```
網頁主頁測試：  
直接在瀏覽器中開啟 index.html 檔案進行測試    

API 測試（後端功能驗證）：  
隨機生成	http://localhost:8080/api/random  
搜尋功能	http://localhost:8080/api/search?q=cat  
查詢錯誤	http://localhost:8080/api/search  

