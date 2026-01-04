package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

const MaxScanTokenSize = 5 * 1024 * 1024

func RunDataImporter() {
	if _, err := os.Stat(ExportFile); os.IsNotExist(err) {
		log.Printf("[系統] 無匯入來源：%s 檔案不存在 (若為初次執行可忽略)", ExportFile)
		return
	}

	filePath := filepath.Join(".", ExportFile)
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("無法開啟 JSON 檔案 %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, MaxScanTokenSize)
	scanner.Buffer(buf, MaxScanTokenSize)

	count := 0
	log.Println("--- [Importer] 開始從 JSON 檔案還原資料 ---")

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rawMeme ExportMeme
		if err := json.Unmarshal(line, &rawMeme); err != nil {
			log.Printf("[ERROR] 解析 JSON 行失敗: %v", err)
			continue
		}

		err := InsertMeme(rawMeme)
		if err == nil {
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("讀取 JSON 檔案時發生錯誤: %v", err)
	}

	log.Printf("--- [Importer] 資料匯入完成！本次新增 %d 筆資料 ---", count)
}
