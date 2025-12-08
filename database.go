package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// =========================================================
// [資料結構定義]
// =========================================================

type ExportMeme struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Tags      string `json:"tags"`
	SourceURL string `json:"source_url"`
}

type Meme = ExportMeme

var db *sql.DB

const ExportFile = "memes_raw_data.json"

// =========================================================
// [初始化與檔案操作]
// =========================================================

func InitExportFile() {
	if err := os.Remove(ExportFile); err == nil {
		log.Printf("[INFO] 舊的輸出檔案 %s 已刪除。", ExportFile)
	}
}

func SaveToJSON(meme ExportMeme) {
	f, err := os.OpenFile(ExportFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[ERROR] 無法開啟 JSON 檔案: %v", err)
		return
	}
	defer f.Close()

	data, _ := json.Marshal(meme)
	f.Write(append(data, '\n'))
}

// =========================================================
// [資料庫操作]
// =========================================================

func InitDB() error {
	var err error
	db, err = sql.Open("sqlite3", "./memes.db")
	if err != nil {
		return fmt.Errorf("開啟資料庫失敗: %v", err)
	}

	if err = db.Ping(); err != nil {
		return fmt.Errorf("無法連線資料庫: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS memes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT,
		url TEXT UNIQUE,
		tags TEXT,
		source_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("建立表格失敗: %v", err)
	}
	return nil
}

func InsertMeme(m ExportMeme) error {
	if db == nil {
		return fmt.Errorf("資料庫尚未初始化")
	}
	query := `INSERT OR IGNORE INTO memes (title, url, tags, source_url) VALUES (?, ?, ?, ?)`
	_, err := db.Exec(query, m.Title, m.URL, m.Tags, m.SourceURL)
	return err
}

func GetMemeCount() (int, error) {
	if db == nil {
		return 0, fmt.Errorf("資料庫未連線")
	}
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM memes").Scan(&count)
	return count, err
}

// ---------------------------------------------------------
// [修正] 搜尋功能：加入對 URL (內容) 欄位的搜尋
// ---------------------------------------------------------
func SearchMemes(query string, mode string) ([]Meme, error) {
	// [修正 1] SQL 加入 OR url LIKE ?
	// 因為對於純文字梗圖，內容是存在 url 欄位裡的
	baseSQL := `SELECT title, url, tags, source_url FROM memes WHERE (title LIKE ? OR tags LIKE ? OR url LIKE ?)`

	filterSQL := ""
	if mode == "image" {
		filterSQL = ` AND url LIKE 'http%'`
	} else if mode == "text" {
		filterSQL = ` AND url NOT LIKE 'http%'`
	}

	finalSQL := baseSQL + filterSQL + ` ORDER BY id DESC LIMIT 50`

	likeQuery := "%" + query + "%"

	// [修正 2] 這裡要傳入三次 likeQuery (對應 title, tags, url)
	rows, err := db.Query(finalSQL, likeQuery, likeQuery, likeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memes := []Meme{}
	for rows.Next() {
		var m Meme
		if err := rows.Scan(&m.Title, &m.URL, &m.Tags, &m.SourceURL); err != nil {
			log.Printf("讀取資料列失敗: %v", err)
			continue
		}
		memes = append(memes, m)
	}
	return memes, nil
}

func GetRandomMeme(mode string) (Meme, error) {
	var m Meme
	sqlQuery := `SELECT title, url, tags, source_url FROM memes`

	whereClause := ""
	if mode == "image" {
		whereClause = ` WHERE url LIKE 'http%'`
	} else if mode == "text" {
		whereClause = ` WHERE url NOT LIKE 'http%'`
	}

	sqlQuery += whereClause + ` ORDER BY RANDOM() LIMIT 1`

	err := db.QueryRow(sqlQuery).Scan(&m.Title, &m.URL, &m.Tags, &m.SourceURL)
	if err == sql.ErrNoRows {
		return Meme{}, fmt.Errorf("找不到資料")
	}
	return m, err
}
