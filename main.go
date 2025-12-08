package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 抽出 setupRouter 方便測試
func setupRouter() *gin.Engine {
	r := gin.Default()
	r.LoadHTMLFiles("index.html")

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	r.GET("/api/search", func(c *gin.Context) {
		query := c.Query("q")
		mode := c.DefaultQuery("mode", "all")
		results, err := SearchMemes(query, mode)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, results)
	})

	r.GET("/api/random", func(c *gin.Context) {
		mode := c.DefaultQuery("mode", "all")
		meme, err := GetRandomMeme(mode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": "找不到資料"})
			return
		}
		c.JSON(http.StatusOK, meme)
	})

	return r
}

func main() {
	log.Println("=== 正在啟動伺服器 ===")

	// 1. 初始化資料庫 (使用常數 DBFile)
	err := InitDB(DBFile)
	if err != nil {
		log.Fatalf("❌ 資料庫連線失敗: %v", err)
	}
	log.Println("✅ 資料庫連線成功")

	// 2. 啟動時自動匯入 JSON 資料
	RunDataImporter()

	// 3. 檢查資料量
	count, _ := GetMemeCount()
	log.Printf("📊 目前資料庫共有 %d 筆資料", count)

	// 4. 啟動 Web Server
	r := setupRouter()
	log.Println("🚀 伺服器運行中: http://localhost:8080")
	r.Run(":8080")
}
