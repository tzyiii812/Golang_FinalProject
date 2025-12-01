package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

// main 函式作為程式的入口點，調用爬蟲邏輯。
func main() {
	RunSpider()
}

// RunSpider 包含了 PTT Joke 看板的爬蟲邏輯
func RunSpider() {
	// 呼叫 database.go 中的函式，清除舊的 JSON 檔案
	InitExportFile()

	// 設置 Collector
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.AllowedDomains("www.ptt.cc"),
		colly.IgnoreRobotsTxt(), // 忽略 robots.txt
	)

	// 增強 User-Agent 和延遲設定
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
	c.SetRequestTimeout(30 * time.Second) // 設置超時時間
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       1500 * time.Millisecond, // 增加延遲，對 PTT 較友善
		RandomDelay: 500 * time.Millisecond,
		Parallelism: 3,
	})

	var count int
	const maxPosts = 100 // 限制爬取文章數量

	// -----------------------------------------------------
	// 規則 1: 處理列表頁面 (追蹤文章連結)
	// -----------------------------------------------------
	c.OnHTML("div.r-ent > div.title > a[href]", func(e *colly.HTMLElement) {
		if count >= maxPosts {
			return
		}
		link := e.Request.AbsoluteURL(e.Attr("href"))
		fmt.Println("[FOUND] 複製文網址:", link)
		e.Request.Visit(link)
	})

	// -----------------------------------------------------
	// 規則 2: 處理獨立帖子頁面 (擷取標題和內容)
	// -----------------------------------------------------
	c.OnResponse(func(r *colly.Response) {
		if count >= maxPosts {
			return
		}
		if !strings.Contains(r.Headers.Get("Content-Type"), "text/html") {
			return
		}

		// 使用 goquery 解析 HTML
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			log.Println("[ERROR] HTML 解析失敗:", err)
			return
		}

		// 找到標題 (在 .article-meta-value 的第三個，通常是標題)
		title := doc.Find(".article-metaline:nth-child(3) .article-meta-value").Text()
		title = strings.TrimSpace(title)

		// 擷取文章內容 (#main-content)
		var content string
		doc.Find("#main-content").Each(func(i int, s *goquery.Selection) {

			// 移除所有 metadata (作者, 看板, 標題, 時間) 和推文
			s.Find("div.push, div.article-metaline, div.article-metaline-right").Remove()

			// 移除文章結束標記，並獲取純文本
			html, _ := s.Html()
			if index := strings.Index(html, "--"); index != -1 {
				html = html[:index]
			}

			// 修正 goquery: NewDocumentFromReader 返回兩個值，忽略錯誤
			docFragment, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
			content = docFragment.Text()
		})

		content = strings.TrimSpace(content)

		// 最終檢查
		if content == "" || len(content) < 50 || title == "" {
			return
		}

		// 修正錯誤：使用 ExportMeme 結構 (與 database.go 保持一致)
		post := ExportMeme{
			Title:     title,
			URL:       content, // 複製文的內容放在 URL 欄位
			Tags:      "ptt, joke, " + strings.Join(strings.Fields(title), ", "),
			SourceURL: r.Request.URL.String(),
		}

		fmt.Println("[SAVE] 複製文:", title)
		SaveToJSON(post)
		count++
	})

	// -----------------------------------------------------
	// 規則 3: 處理分頁 (翻頁功能)
	// -----------------------------------------------------
	// 翻頁功能 (找「‹ 上頁」按鈕)
	c.OnHTML("div.btn-group-paging > a.btn.wide", func(e *colly.HTMLElement) {
		if e.Text == "‹ 上頁" && count < maxPosts {
			prevPage := e.Request.AbsoluteURL(e.Attr("href"))
			fmt.Println("[PAGE] 正在訪問上一頁:", prevPage)
			e.Request.Visit(prevPage)
		}
	})

	// -----------------------------------------------------
	// 訪問目標 URL
	// -----------------------------------------------------
	startURL := "https://www.ptt.cc/bbs/Joke/index.html"
	fmt.Println("[START] 開始爬:", startURL)

	err := c.Visit(startURL)
	if err != nil {
		log.Printf("[VISIT ERROR] 訪問 %s 失敗: %v", startURL, err)
	}

	c.Wait()
	fmt.Println("=== PTT Joke 爬蟲執行完畢，總共儲存", count, "筆 ===")
}
