package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// downloadGIF 負責下載單一 GIF 檔案 (保持不變)
func downloadGIF(gifURL string) {
	parts := strings.Split(gifURL, "/")
	fileName := parts[len(parts)-1]

	if fileName == "" || !strings.HasSuffix(strings.ToLower(fileName), ".gif") {
		log.Printf("[SKIP] 無效或非 GIF 連結: %s", gifURL)
		return
	}

	// 創建 gifs 目錄
	if _, err := os.Stat("gifs"); os.IsNotExist(err) {
		os.Mkdir("gifs", 0755)
	}
	filePath := filepath.Join("gifs", fileName)

	// 檢查檔案是否已存在，避免重複下載
	if _, err := os.Stat(filePath); err == nil {
		fmt.Printf("[INFO] 檔案已存在，跳過: %s\n", fileName)
		return
	}

	fmt.Printf("[DOWNLOAD] 正在下載: %s\n", fileName)

	// 發送 HTTP 請求下載檔案
	resp, err := http.Get(gifURL)
	if err != nil {
		log.Printf("[ERROR] 下載失敗 (%s): %v", fileName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[ERROR] 下載失敗，HTTP 狀態碼: %d (%s)", resp.StatusCode, fileName)
		return
	}

	// 創建並寫入檔案
	out, err := os.Create(filePath)
	if err != nil {
		log.Printf("[ERROR] 建立檔案失敗 (%s): %v", fileName, err)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("[ERROR] 寫入檔案失敗 (%s): %v", fileName, err)
		return
	}

	fmt.Printf("[SUCCESS] 檔案儲存成功: %s\n", fileName)
}

// scrapeSingleGIFPage 負責進入單一 GIF 頁面並找出實際的 GIF URL (保持不變)
func scrapeSingleGIFPage(url string) {
	c := colly.NewCollector()

	// 選擇器: 抓取實際 GIF 檔案的 URL (img.media-show)
	c.OnHTML(`img.media-show`, func(e *colly.HTMLElement) {
		gifURL := e.Attr("src")
		fullGIFURL := e.Request.AbsoluteURL(gifURL)
		downloadGIF(fullGIFURL)
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("[ERROR] 爬取單頁失敗: %s: URL: %s", err, r.Request.URL)
	})

	c.Visit(url)
}

func main() {
	// 設置主 Collector，負責處理 API 請求
	c := colly.NewCollector(
		colly.CacheDir("./cache"),
		colly.AllowedDomains("www.gif-vif.com"),
	)

	// 設置延遲，這是爬蟲的好習慣
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       500 * time.Millisecond,
		RandomDelay: 100 * time.Millisecond,
	})

	// -----------------------------------------------------
	// 核心邏輯: 處理 API 返回的 JSON 陣列 (使用 goquery)
	// -----------------------------------------------------
	c.OnResponse(func(r *colly.Response) {
		// 1. 檢查是否為 JSON 格式
		if strings.Contains(r.Headers.Get("Content-Type"), "application/json") {

			// 設置變數接收 JSON 陣列 (字串陣列)
			var htmlFragments []string
			if err := json.Unmarshal(r.Body, &htmlFragments); err != nil {
				log.Printf("[ERROR] 無法解析 API JSON 陣列: %v", err)
				return
			}

			// 遍歷每一個 HTML 片段並使用 goquery 解析
			for _, htmlContent := range htmlFragments {
				doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
				if err != nil {
					log.Printf("[ERROR] 解析 HTML 片段失敗: %v", err)
					continue
				}

				// 尋找包含 GIF 頁面連結的元素 (選擇器：a[href])
				doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
					link, exists := s.Attr("href")
					if !exists {
						return
					}

					// 檢查連結是否指向 GIF 頁面 (/gifs/) 且不是下載連結 (/download/)
					if strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
						fullURL := r.Request.AbsoluteURL(link)
						fmt.Printf("[FOUND] 找到 GIF 頁面連結: %s\n", fullURL)

						// 異步訪問單一 GIF 頁面進行下載
						go scrapeSingleGIFPage(fullURL)
					}
				})
			}

		} else {
			// 如果不是 JSON，可能是訪問的首頁或錯誤頁面，使用 goquery 解析 response body
			doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
			if err != nil {
				log.Printf("[ERROR] 解析 response body 失敗: %v", err)
				return
			}
			// 尋找首頁中的 GIF 連結 (選擇器：div.gif-item a[href])
			doc.Find("div.gif-item a[href]").Each(func(i int, s *goquery.Selection) {
				link, exists := s.Attr("href")
				if !exists {
					return
				}
				if strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
					fullURL := r.Request.AbsoluteURL(link)
					fmt.Printf("[FOUND] 找到 GIF 頁面連結: %s\n", fullURL)

					// 異步訪問單一 GIF 頁面進行下載
					go scrapeSingleGIFPage(fullURL)
				}
			})
		}
	})

	// **【優化點】**：移除冗餘的 c.OnHTML 規則，所有列表解析邏輯皆在 OnResponse 內處理。

	// -----------------------------------------------------
	// 迭代呼叫 API
	// -----------------------------------------------------
	const offsetIncrement = 8
	// 爬取數量限制：請根據需求調整這個值
	maxOffset := 5000 //
	startOffset := 0

	fmt.Printf("=== 開始呼叫 API 進行爬取 (Max Offset: %d) ===\n", maxOffset)

	for offset := startOffset; offset <= maxOffset; offset += offsetIncrement {
		// 構造 API URL
		apiURL := fmt.Sprintf("https://www.gif-vif.com/loadMore.php?offset=%d", offset)

		fmt.Printf("[API VISIT] 請求: %s\n", apiURL)

		err := c.Visit(apiURL)
		if err != nil {
			log.Printf("[API ERROR] 訪問 %s 失敗: %v", apiURL, err)
		}
	}

	// 等待所有異步的下載任務完成
	c.Wait()
	fmt.Println("\n=== API 呼叫、爬取與下載完成 ===")
}
