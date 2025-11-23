package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// RunDataImporter 讀取 JSON 檔案並匯入資料庫
func RunDataImporter() {
	// 檢查 JSON 檔案是否存在 (ExportFile 常數定義在 database.go 中)
	if _, err := os.Stat(ExportFile); os.IsNotExist(err) {
		log.Printf("[WARNING] 匯入失敗：%s 檔案不存在。請先執行 spider.go 爬蟲。", ExportFile)
		return
	}

	filePath := filepath.Join(".", ExportFile)

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("無法開啟 JSON 檔案 %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0

	log.Println("--- 開始從 JSON 檔案匯入資料 ---")

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rawMeme ExportMeme
		// 1. 解析 JSON 行
		if err := json.Unmarshal(line, &rawMeme); err != nil {
			// 如果 JSON 解析失敗，會明確印出錯誤和內容
			log.Printf("[ERROR] 解析 JSON 行失敗: %v, 內容: %s", err, string(line))
			continue
		}

		// 2. 插入資料庫
		err := InsertMeme(rawMeme)
		if err != nil {
			// 如果 InsertMeme 回傳錯誤 (非 UNIQUE 錯誤)，則打印出來
			log.Printf("[ERROR] 插入資料庫失敗 (%s): %v", rawMeme.URL, err)
		} else {
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("讀取 JSON 檔案時發生錯誤: %v", err)
	}

	log.Printf("--- 資料匯入完成！總共匯入 %d 筆資料 ---", count)
}
