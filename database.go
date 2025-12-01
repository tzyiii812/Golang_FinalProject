package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3" // 引入 SQLite 驅動
)

// DBFile 定義資料庫檔案名稱
const DBFile = "project_memes.db"
const ExportFile = "memes_raw_data.json"

// Meme 結構體用於存放資料庫中的單筆資料
type Meme struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"` // 原始 GIF 檔案的 URL (現在是複製文內容)
	Tags      string `json:"tags"`
	SourceURL string `json:"source_url"`
}

// ExportMeme 結構體用於從 JSON 檔案中讀取數據（與爬蟲輸出的結構相同）
type ExportMeme struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Tags      string `json:"tags"`
	SourceURL string `json:"source_url"`
}

var db *sql.DB             // 全域資料庫連線物件
var outputMutex sync.Mutex // 用於多執行緒寫入 JSON 檔案的鎖

// -----------------------------------------------------
// 爬蟲/匯入通用函式
// -----------------------------------------------------

// InitExportFile 初始化輸出檔案 (供 spider.go 調用)
func InitExportFile() {
	if err := os.Remove(ExportFile); err == nil {
		log.Printf("[INFO] 舊的輸出檔案 %s 已刪除。", ExportFile)
	} else if !os.IsNotExist(err) {
		log.Printf("[ERROR] 無法移除舊檔案: %v", err)
	}
}

// SaveToJSON 將單筆資料寫入 JSON 檔案 (供 spider.go 調用)
func SaveToJSON(meme ExportMeme) {
	outputMutex.Lock()
	defer outputMutex.Unlock()

	f, err := os.OpenFile(ExportFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[ERROR] 無法開啟 JSON 檔案: %v", err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(meme)
	if err != nil {
		log.Printf("[ERROR] JSON 編碼失敗: %v", err)
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		log.Printf("[ERROR] 寫入 JSON 檔案失敗: %v", err)
	}
}

// -----------------------------------------------------
// 資料庫操作函式
// -----------------------------------------------------

// initDB 檢查並初始化 SQLite 資料庫連線和表格
func initDB() {
	var err error

	// 連接或建立資料庫檔案
	db, err = sql.Open("sqlite3", DBFile)
	if err != nil {
		log.Fatalf("無法開啟資料庫檔案 (%s): %v", DBFile, err)
	}

	// 建立 memes 表格，URL 設為 UNIQUE 以避免重複匯入
	query := `
	CREATE TABLE IF NOT EXISTS memes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		url TEXT NOT NULL UNIQUE,
		tags TEXT,
		source_url TEXT
	);
	`
	_, err = db.Exec(query)
	if err != nil {
		log.Fatalf("建立資料表失敗: %v", err)
	}
	log.Println("資料庫初始化成功，表格 memes 已準備就緒。")
}

// SearchMemes 根據關鍵字在 title 或 tags 欄位中進行模糊查詢
func SearchMemes(query string) ([]Meme, error) {
	likeQuery := "%" + query + "%"
	sqlQuery := `SELECT id, title, url, tags, source_url FROM memes 
				WHERE title LIKE ? OR tags LIKE ? LIMIT 100`

	rows, err := db.Query(sqlQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memes := []Meme{}
	for rows.Next() {
		var m Meme
		err := rows.Scan(&m.ID, &m.Title, &m.URL, &m.Tags, &m.SourceURL)
		if err != nil {
			return nil, err
		}
		memes = append(memes, m)
	}
	return memes, nil
}

// GetRandomMeme 隨機獲取一筆資料
func GetRandomMeme() (Meme, error) {
	var m Meme
	sqlQuery := `SELECT id, title, url, tags, source_url FROM memes 
				ORDER BY RANDOM() LIMIT 1`

	row := db.QueryRow(sqlQuery)
	err := row.Scan(&m.ID, &m.Title, &m.URL, &m.Tags, &m.SourceURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return Meme{}, nil // 資料庫為空
		}
		return Meme{}, err
	}
	return m, nil
}

// InsertMeme 將單筆資料存入資料庫
func InsertMeme(meme ExportMeme) error {
	// 使用 ExportMeme 結構來接收數據
	stmt, err := db.Prepare("INSERT INTO memes(title, url, tags, source_url) VALUES(?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(meme.Title, meme.URL, meme.Tags, meme.SourceURL)
	if err != nil {
		// 處理 URL 唯一性約束錯誤，避免重複匯入時程式中斷
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil
		}
		return err
	}
	return nil
}
