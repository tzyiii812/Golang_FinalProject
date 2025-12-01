package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. 初始化資料庫
	initDB()

	// 2. 執行一次性數據匯入
	log.Println("[INFO] 正在執行一次性數據匯入...")
	RunDataImporter()

	// 3. 建立 Gin 路由器
	router := gin.Default()

	// 跨域設定 (允許瀏覽器在本地開發時存取 API)
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 設置靜態檔案路徑 (提供 index.html)
	router.GET("/", func(c *gin.Context) {
		c.File("index.html")
	})

	// 4. 定義 API 路由
	api := router.Group("/api")
	{
		api.GET("/search", handleSearch)
		api.GET("/random", handleRandom)
	}

	// 5. 啟動 Web 服務
	port := ":8080"
	fmt.Printf("Web 服務已啟動，請訪問 http://localhost%s/ 或 /api/random\n", port)
	if err := router.Run(port); err != nil {
		log.Fatalf("無法啟動 Web 服務: %v", err)
	}
}

// handleSearch 處理搜尋請求
func handleSearch(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要提供查詢參數 'q'"})
		return
	}

	memes, err := SearchMemes(query)
	if err != nil {
		log.Printf("資料庫搜尋失敗: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "內部伺服器錯誤"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"count":   len(memes),
		"results": memes,
	})
}

// handleRandom 處理隨機生成請求
func handleRandom(c *gin.Context) {
	meme, err := GetRandomMeme()
	if err != nil {
		log.Printf("資料庫隨機查詢失敗: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "內部伺服器錯誤"})
		return
	}

	if meme.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "資料庫中沒有任何資料"})
		return
	}

	c.JSON(http.StatusOK, meme)
}
