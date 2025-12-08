package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly/v2"
)

// =========================================================
// [設定區]
// =========================================================

var threadsUsers = []string{
	"ctrl.v.book",
	"shuixian1002",
}

var plurkUsers = []string{
	"copypasta",
}

func main() {
	log.Println("[系統] 初始化資料庫...")
	InitDB()
	InitExportFile()

	// ---------------------------------------------------------
	// [階段一] GIF 靜態爬蟲 (補回此功能)
	// ---------------------------------------------------------
	log.Println("=== [階段一] 開始爬取 GIF 梗圖 (Colly) ===")
	RunGifSpider()

	// ---------------------------------------------------------
	// [階段二 & 三] Chromedp (Threads / Plurk)
	// ---------------------------------------------------------
	if len(threadsUsers) > 0 || len(plurkUsers) > 0 {
		runChromedpScrapers()
	}

	// ---------------------------------------------------------
	// [階段四] Colly (PTT)
	// ---------------------------------------------------------
	log.Println("=== [階段四] 開始執行 PTT Joke 版爬蟲 (Colly) ===")
	RunPTTSpider()

	log.Println("[系統] 所有任務執行完畢！")
}

// =========================================================
// 1. GIF 爬蟲
// =========================================================

func RunGifSpider() {
	c := colly.NewCollector(
		colly.CacheDir("./cache"),
		colly.AllowedDomains("www.gif-vif.com"),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       2 * time.Second,
		RandomDelay: 1 * time.Second,
		Parallelism: 5,
	})

	// 處理列表頁與 API 回應
	c.OnResponse(func(r *colly.Response) {
		if strings.Contains(r.Headers.Get("Content-Type"), "application/json") {
			var htmlFragments []string
			if err := json.Unmarshal(r.Body, &htmlFragments); err == nil {
				for _, htmlContent := range htmlFragments {
					doc, _ := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
					doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
						link, exists := s.Attr("href")
						if exists && strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
							c.Visit(r.Request.AbsoluteURL(link))
						}
					})
				}
			}
		} else {
			doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
			doc.Find("div.gif-item a[href]").Each(func(i int, s *goquery.Selection) {
				link, exists := s.Attr("href")
				if exists && strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
					c.Visit(r.Request.AbsoluteURL(link))
				}
			})
		}
	})

	// 處理單一 GIF 頁面
	c.OnHTML(`img.media-show`, func(e *colly.HTMLElement) {
		gifURL := e.Request.AbsoluteURL(e.Attr("src"))
		title := e.Attr("alt")
		if title == "" {
			title = e.DOM.ParentsUntil("html").Find("title").Text()
		}
		tags := strings.Join(strings.Split(title, " "), ", ")

		meme := ExportMeme{
			Title:     title,
			URL:       gifURL,
			Tags:      tags,
			SourceURL: e.Request.URL.String(),
		}

		SaveToJSON(meme)
		if err := InsertMeme(meme); err == nil {
			log.Printf("[GIF SAVE] %s", title)
		}
	})

	// 爬取前 5 頁 (offset 0, 8, 16...)
	for offset := 0; offset <= 40; offset += 8 {
		c.Visit(fmt.Sprintf("https://www.gif-vif.com/loadMore.php?offset=%d", offset))
	}
	c.Wait()
	log.Println("=== GIF 爬蟲執行完畢 ===")
}

// =========================================================
// Chromedp 執行邏輯 (Threads/Plurk)
// =========================================================
func runChromedpScrapers() {
	log.Println(">>> 嘗試連線到 Chrome (ws://127.0.0.1:9222)...")
	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), "ws://127.0.0.1:9222/")
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	log.Println(">>> 連線成功！開始執行 Chromedp 任務...")

	if len(threadsUsers) > 0 {
		log.Println("=== [階段二] 開始爬取 Threads ===")
		runGenericScraper(ctx, threadsUsers, "Threads", scrapeThreadsUser)
	}

	if len(plurkUsers) > 0 {
		log.Println("=== [階段三] 開始爬取 Plurk ===")
		runGenericScraper(ctx, plurkUsers, "Plurk", scrapePlurkUser)
	}
}

// 通用排程器
func runGenericScraper(ctx context.Context, targets []string, platform string, scrapeFunc func(context.Context, string) ([]ExportMeme, error)) {
	for i, target := range targets {
		log.Printf("--- [%s][%d/%d] 正在處理: %s ---", platform, i+1, len(targets), target)

		chromedp.Run(ctx, chromedp.Navigate("about:blank"))
		time.Sleep(1 * time.Second)

		memes, err := scrapeFunc(ctx, target)
		if err != nil {
			log.Printf("[錯誤] %s 爬取失敗: %v", target, err)
			continue
		}

		count := 0
		for _, meme := range memes {
			SaveToJSON(meme)
			if err := InsertMeme(meme); err == nil {
				count++
			}
		}
		log.Printf("    -> 成功入庫: %d 筆", count)
		time.Sleep(time.Duration(3+rand.Intn(3)) * time.Second)
	}
}

// =========================================================
// 2. Threads 專用邏輯 (使用 JS 上下震動滾動，避免卡死)
// =========================================================

func scrapeThreadsUser(parentCtx context.Context, userID string) ([]ExportMeme, error) {
	url := fmt.Sprintf("https://www.threads.net/@%s", userID)

	var allMemes []ExportMeme
	seenContent := make(map[string]bool)

	// 設定總超時
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	defer cancel()

	log.Printf("    -> 前往 Threads: %s", url)

	// 1. 導航
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(c context.Context) error {
			// 嘗試點擊可能的彈窗
			ctxTO, cancelTO := context.WithTimeout(c, 2*time.Second)
			defer cancelTO()
			chromedp.Click(`div[role="dialog"] div[role="button"]`, chromedp.ByQuery).Do(ctxTO)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	// 2. 滾動與解析
	scrollCount := 10
	for i := 0; i < scrollCount; i++ {
		log.Printf("        ... 滾動與解析 %d/%d", i+1, scrollCount)

		var currentHTML string
		// 使用 context timeout 防止滾動指令卡死
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)

		err := chromedp.Run(timeoutCtx,
			// [解法] JS 上下震動法：
			// 1. 滾到底
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(1*time.Second),

			// 2. 往回拉 300px (讓瀏覽器覺得我們離開了底部)
			chromedp.Evaluate(`window.scrollBy(0, -300);`, nil),
			chromedp.Sleep(500*time.Millisecond),

			// 3. 再滾到底 (觸發 "再次到達底部" 事件)
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),

			// 4. 等待載入
			chromedp.Sleep(3*time.Second),

			// 5. 抓取 HTML
			chromedp.OuterHTML("body", &currentHTML),
		)
		timeoutCancel() // 釋放這個回合的 timeout

		if err != nil {
			log.Printf("滾動操作逾時或失敗 (跳過此輪): %v", err)
			continue // 繼續下一輪，不要直接跳出
		}

		// 解析
		currentBatch := parseThreadsHTML(userID, url, currentHTML)

		newCount := 0
		for _, meme := range currentBatch {
			if !seenContent[meme.URL] {
				seenContent[meme.URL] = true
				allMemes = append(allMemes, meme)
				newCount++
			}
		}
		log.Printf("            -> 本次滾動新增 %d 篇 (累計: %d)", newCount, len(allMemes))
	}

	return allMemes, nil
}

func parseThreadsHTML(author string, sourceURL string, html string) []ExportMeme {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	var results []ExportMeme

	headerRegex := regexp.MustCompile(`(?s)^追蹤.*?更多`)

	// [關鍵修正]
	// 1. (?:翻譯\s*)?  --> 讓「翻譯」變成 footer 的一部分，且允許後面有空格。
	// 2. \s* --> 在每個關鍵字(讚,回覆...)之間允許任意空白，避免文字黏在一起時失效。
	// 3. [\d\.\s萬kK\+,]* --> 匹配數字及單位 (包含 290, 1.2萬, 100+)
	footerRegex := regexp.MustCompile(`(?s)(?:翻譯\s*)?讚[\d\.\s萬kK\+,]*回覆[\d\.\s萬kK\+,]*轉發[\d\.\s萬kK\+,]*分享[\d\.\s萬kK\+,]*$`)

	doc.Find("div[data-pressable-container='true']").Each(func(i int, s *goquery.Selection) {
		s.Find("br").ReplaceWithHtml("\n")
		rawText := strings.TrimSpace(s.Text())

		rawText = strings.ReplaceAll(rawText, "\u00A0", " ") // 替換不換行空格

		// 先清理頭部
		cleanText := headerRegex.ReplaceAllString(rawText, "")

		// 再清理尾部 (翻譯 + 數據按鈕)
		cleanText = footerRegex.ReplaceAllString(cleanText, "")

		cleanText = strings.TrimSpace(cleanText)

		if len(cleanText) > 5 && !strings.Contains(cleanText, "Log in") && !strings.Contains(cleanText, "登入") {
			if strings.HasPrefix(cleanText, author) {
				cleanText = strings.TrimPrefix(cleanText, author)
				cleanText = strings.TrimSpace(cleanText)
			}
			meme := ExportMeme{
				Title:     author,
				URL:       cleanText,
				Tags:      "Threads",
				SourceURL: sourceURL,
			}
			results = append(results, meme)
		}
	})
	return results
}

// =========================================================
// 3. Plurk 專用邏輯 (使用 JS 上下震動滾動)
// =========================================================

func scrapePlurkUser(parentCtx context.Context, userID string) ([]ExportMeme, error) {
	url := fmt.Sprintf("https://www.plurk.com/m/u/%s", userID)
	var allMemes []ExportMeme
	seenContent := make(map[string]bool)

	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	defer cancel()

	log.Printf("    -> 前往 Plurk: %s", url)
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(c context.Context) error {
			ctxTO, cancelTO := context.WithTimeout(c, 2*time.Second)
			defer cancelTO()
			chromedp.Evaluate(`
				var buttons = document.querySelectorAll('a, button, input[type="submit"]');
				for (var i = 0; i < buttons.length; i++) {
					var text = (buttons[i].innerText || buttons[i].value || "").toLowerCase();
					if (text.includes("yes") || text.includes("over 18")) {
						buttons[i].click(); break;
					}
				}
			`, nil).Do(ctxTO)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	scrollCount := 10
	for i := 0; i < scrollCount; i++ {
		log.Printf("        ... 滾動與解析 %d/%d", i+1, scrollCount)
		var currentHTML string

		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)
		err := chromedp.Run(timeoutCtx,
			// Plurk 策略：用力滑
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(1*time.Second),
			chromedp.Evaluate(`window.scrollBy(0, -500);`, nil), // 回拉多一點
			chromedp.Sleep(500*time.Millisecond),
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),

			chromedp.Sleep(3*time.Second),
			chromedp.OuterHTML("body", &currentHTML),
		)
		timeoutCancel()

		if err != nil {
			log.Printf("滾動操作逾時 (跳過此輪): %v", err)
			continue
		}

		currentBatch := parsePlurkHTML(userID, url, currentHTML)

		newCount := 0
		for _, meme := range currentBatch {
			if !seenContent[meme.URL] {
				seenContent[meme.URL] = true
				allMemes = append(allMemes, meme)
				newCount++
			}
		}
		log.Printf("            -> 本次滾動新增 %d 篇 (累計: %d)", newCount, len(allMemes))
	}
	return allMemes, nil
}

func parsePlurkHTML(author string, sourceURL string, html string) []ExportMeme {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	var results []ExportMeme
	doc.Find(".plurk").Each(func(i int, s *goquery.Selection) {
		contentNode := s.Find(".plurk-content")
		contentNode.Find("br").ReplaceWithHtml("\n")
		text := strings.TrimSpace(contentNode.Text())
		if len(text) > 1 && !strings.Contains(text, "被標示為含有成人內容") {
			meme := ExportMeme{Title: author, URL: text, Tags: "Plurk", SourceURL: sourceURL}
			results = append(results, meme)
		}
	})
	return results
}

// =========================================================
// 4. PTT 專用邏輯 (Colly 防斷線版)
// =========================================================

func RunPTTSpider() {
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.AllowedDomains("www.ptt.cc"),
		colly.IgnoreRobotsTxt(),
	)

	// 自定義 Transport (防斷線關鍵)
	c.WithTransport(&http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	})

	c.SetCookies("https://www.ptt.cc", []*http.Cookie{
		{Name: "over18", Value: "1", Domain: "www.ptt.cc", Path: "/"},
	})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8")
	})

	c.SetRequestTimeout(30 * time.Second)
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Delay:       2000 * time.Millisecond, //強制延遲，避免伺服器過載
		RandomDelay: 1000 * time.Millisecond, //增加隨機性，模擬人類行為
		Parallelism: 1,                       //單線程，避免過多連線導致被封鎖
	})

	var count int
	const maxPosts = 200

	// 規則 1: 列表頁
	c.OnHTML("div.r-ent > div.title > a[href]", func(e *colly.HTMLElement) {
		if count >= maxPosts {
			return
		}
		link := e.Request.AbsoluteURL(e.Attr("href"))
		e.Request.Visit(link)
	})

	// 規則 2: 文章頁
	c.OnResponse(func(r *colly.Response) {
		if count >= maxPosts {
			return
		}
		if !strings.Contains(r.Request.URL.String(), "/M.") {
			return
		}
		if !strings.Contains(r.Headers.Get("Content-Type"), "text/html") {
			return
		}

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			return
		}

		title := doc.Find(".article-metaline:nth-child(3) .article-meta-value").Text()
		title = strings.TrimSpace(title)

		var content string
		doc.Find("#main-content").Each(func(i int, s *goquery.Selection) {
			s.Find("div.push, div.article-metaline, div.article-metaline-right").Remove()
			html, _ := s.Html()
			if index := strings.Index(html, "--"); index != -1 {
				html = html[:index]
			}
			docFragment, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
			content = docFragment.Text()
		})
		content = strings.TrimSpace(content)

		if content == "" || len(content) < 50 || title == "" {
			return
		}

		post := ExportMeme{
			Title:     title,
			URL:       content,
			Tags:      strings.Join(strings.Fields(title), ", "),
			SourceURL: r.Request.URL.String(),
		}

		fmt.Println("[PTT SAVE] 複製文:", title)
		SaveToJSON(post)
		if err := InsertMeme(post); err == nil {
			count++
		}
	})

	// 規則 3: 翻頁
	c.OnHTML("div.btn-group-paging > a.btn.wide", func(e *colly.HTMLElement) {
		if strings.Contains(e.Text, "上頁") && count < maxPosts {
			prevPage := e.Request.AbsoluteURL(e.Attr("href"))
			fmt.Println("[PAGE] 正在訪問上一頁:", prevPage)
			e.Request.Visit(prevPage)
		}
	})

	startURL := "https://www.ptt.cc/bbs/Joke/index.html"
	fmt.Println("[START] 開始爬:", startURL)
	c.Visit(startURL)
	c.Wait()
	fmt.Println("=== PTT Joke 爬蟲執行完畢，總共儲存", count, "筆 ===")
}
