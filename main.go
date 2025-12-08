package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// main 是網站伺服器的入口
// 執行指令: go run main.go database.go
func main() {
	log.Println("=== 正在啟動伺服器 ===")
	RunDataImporter()
	// 1. 連線資料庫 (記得捕捉錯誤)
	err := InitDB()
	if err != nil {
		log.Fatalf("❌ 資料庫連線失敗: %v\n請確認 memes.db 檔案是否被其他程式占用", err)
	}
	log.Println("✅ 資料庫連線成功")

	// 2. 檢查資料庫是否有資料
	count, err := GetMemeCount()
	if err != nil {
		log.Printf("⚠️ 無法讀取資料數量: %v", err)
	} else {
		log.Printf("📊 目前資料庫共有 %d 筆資料", count)
		if count == 0 {
			log.Println("⚠️ 警告：資料庫是空的！請先執行爬蟲： go run spider.go database.go")
		}
	}

	// 3. 設定 Gin 路由
	r := gin.Default()
	r.LoadHTMLFiles("index.html")

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 搜尋 API
	r.GET("/api/search", func(c *gin.Context) {
		query := c.Query("q")
		mode := c.DefaultQuery("mode", "all")

		log.Printf("[API] 搜尋請求: 關鍵字='%s', 模式='%s'", query, mode) // 加入 Log

		results, err := SearchMemes(query, mode)
		if err != nil {
			log.Printf("❌ 搜尋錯誤: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("✅ 搜尋結果: 找到 %d 筆", len(results))
		c.JSON(http.StatusOK, results)
	})

	// 隨機 API
	r.GET("/api/random", func(c *gin.Context) {
		mode := c.DefaultQuery("mode", "all")

		log.Printf("[API] 隨機請求: 模式='%s'", mode) // 加入 Log

		meme, err := GetRandomMeme(mode)
		if err != nil {
			log.Printf("❌ 隨機錯誤: %v", err)
			c.JSON(http.StatusOK, gin.H{"error": "找不到資料 (資料庫可能是空的，或該分類無資料)"})
			return
		}
		c.JSON(http.StatusOK, meme)
	})

	log.Println("🚀 伺服器運行中: http://localhost:8080")
	r.Run(":8080")
}
