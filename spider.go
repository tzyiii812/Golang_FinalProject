package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// -----------------------------------------------------
// 數據結構與工具
// -----------------------------------------------------

// ExportMeme, Meme 結構體已在 database.go 中定義，此處無需重複宣告。

var outputMutex sync.Mutex

// initExportFile 負責在程式啟動時，清理或初始化輸出檔案
func initExportFile() {
	if err := os.Remove(ExportFile); err == nil {
		log.Printf("[INFO] 舊的輸出檔案 %s 已刪除。", ExportFile)
	} else if !os.IsNotExist(err) {
		log.Printf("[ERROR] 無法移除舊檔案: %v", err)
	}
}

// saveToJSON 負責將單筆數據安全地寫入 JSON 檔案
// 注意：此處使用的 ExportMeme 結構體定義在 database.go 中
func saveToJSON(meme ExportMeme) {
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
// 爬蟲主邏輯
// -----------------------------------------------------

// main 是 spider.go 程式的單獨入口點
func main() {
	RunSpider()
}

// RunSpider 包含所有爬蟲的執行邏輯 (原來的 main 函數內容)
func RunSpider() {
	// 1. 初始化輸出檔案
	initExportFile()

	// 設置主 Collector，處理所有 API 請求和單頁訪問
	c := colly.NewCollector(
		colly.CacheDir("./cache"),
		colly.AllowedDomains("www.gif-vif.com"),
	)

	// 增強爬蟲限制
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       2 * time.Second,
		RandomDelay: 1 * time.Second,
		Parallelism: 5,
	})

	// -----------------------------------------------------
	// 規則 1: 處理 API 返回的 JSON 陣列 (列表解析)
	// -----------------------------------------------------
	c.OnResponse(func(r *colly.Response) {
		if strings.Contains(r.Headers.Get("Content-Type"), "application/json") {

			var htmlFragments []string
			if err := json.Unmarshal(r.Body, &htmlFragments); err != nil {
				log.Printf("[ERROR] 無法解析 API JSON 陣列: %v", err)
				return
			}

			for _, htmlContent := range htmlFragments {
				doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
				if err != nil {
					log.Printf("[ERROR] 解析 HTML 片段失敗: %v", err)
					continue
				}

				doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
					link, exists := s.Attr("href")
					if !exists {
						return
					}

					if strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
						fullURL := r.Request.AbsoluteURL(link)
						fmt.Printf("[FOUND] 找到 GIF 頁面連結: %s\n", fullURL)

						c.Visit(fullURL)
					}
				})
			}

		} else {
			// 處理非 JSON 響應 (例如首頁)
			doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
			doc.Find("div.gif-item a[href]").Each(func(i int, s *goquery.Selection) {
				link, exists := s.Attr("href")
				if !exists {
					return
				}
				if strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
					fullURL := r.Request.AbsoluteURL(link)
					fmt.Printf("[FOUND] 找到 GIF 頁面連結: %s\n", fullURL)
					c.Visit(fullURL)
				}
			})
		}
	})

	// -----------------------------------------------------
	// 規則 2: 處理單一 GIF 頁面的數據提取與 JSON 寫入
	// -----------------------------------------------------
	c.OnHTML(`img.media-show`, func(e *colly.HTMLElement) {
		// 1. 獲取 GIF 原始 URL (img 的 src 屬性)
		gifURL := e.Attr("src")
		fullGIFURL := e.Request.AbsoluteURL(gifURL)

		// 2. 獲取 Title 和 Tags
		title := e.Attr("alt")
		if title == "" {
			title = e.DOM.ParentsUntil("html").Find("title").Text()
		}

		tags := strings.Split(title, " ")
		tagsString := strings.Join(tags, ", ")

		// 3. 儲存到 JSON 檔案
		rawMeme := ExportMeme{
			Title:     title,
			URL:       fullGIFURL,
			Tags:      tagsString,
			SourceURL: e.Request.URL.String(),
		}
		saveToJSON(rawMeme)
	})

	// -----------------------------------------------------
	// 迭代呼叫 API
	// -----------------------------------------------------
	const offsetIncrement = 8
	maxOffset := 50 // 請依需求調整
	startOffset := 0

	fmt.Printf("=== 開始呼叫 API 進行數據收集 (Max Offset: %d) ===\n", maxOffset)

	for offset := startOffset; offset <= maxOffset; offset += offsetIncrement {
		apiURL := fmt.Sprintf("https://www.gif-vif.com/loadMore.php?offset=%d", offset)

		fmt.Printf("[API VISIT] 請求: %s\n", apiURL)

		err := c.Visit(apiURL)
		if err != nil {
			log.Printf("[API ERROR] 訪問 %s 失敗: %v", apiURL, err)
		}
	}

	c.Wait()
	fmt.Println("\n=== 數據收集與 JSON 輸出完成 ===")
}
