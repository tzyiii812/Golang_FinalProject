package main

import (
	"bytes"
	"context"
	"crypto/tls"
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
	initDB()
	InitExportFile()

	// ---------------------------------------------------------
	// [階段一 & 二] Chromedp (Threads / Plurk)
	// ---------------------------------------------------------
	if len(threadsUsers) > 0 || len(plurkUsers) > 0 {
		runChromedpScrapers()
	}

	// ---------------------------------------------------------
	// [階段三] Colly (PTT)
	// ---------------------------------------------------------
	log.Println("=== [階段三] 開始執行 PTT Joke 版爬蟲 (Colly) ===")
	RunPTTSpider()

	log.Println("[系統] 所有任務執行完畢！")
}

// =========================================================
// Chromedp 執行邏輯
// =========================================================
func runChromedpScrapers() {
	log.Println(">>> 嘗試連線到 Chrome (ws://127.0.0.1:9222)...")
	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), "ws://127.0.0.1:9222/")
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	log.Println(">>> 連線成功！開始執行 Chromedp 任務...")

	if len(threadsUsers) > 0 {
		log.Println("=== [階段一] 開始爬取 Threads ===")
		runGenericScraper(ctx, threadsUsers, "Threads", scrapeThreadsUser)
	}

	if len(plurkUsers) > 0 {
		log.Println("=== [階段二] 開始爬取 Plurk ===")
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
// 1. Threads 專用邏輯 (已修正：邊滾邊抓模式)
// =========================================================

func scrapeThreadsUser(parentCtx context.Context, userID string) ([]ExportMeme, error) {
	url := fmt.Sprintf("https://www.threads.net/@%s", userID)

	// [關鍵修改] 改為累積式儲存
	var allMemes []ExportMeme
	seenContent := make(map[string]bool)

	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	defer cancel()

	log.Printf("    -> 前往 Threads: %s", url)

	// 1. 導航與前置作業
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		// 關閉彈窗
		chromedp.ActionFunc(func(c context.Context) error {
			ctxTO, cancelTO := context.WithTimeout(c, 2*time.Second)
			defer cancelTO()
			chromedp.Click(`div[role="dialog"] div[role="button"]`, chromedp.ByQuery).Do(ctxTO)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	// 2. 邊滾動邊解析 (解決 Virtual DOM 導致舊文章消失的問題)
	scrollCount := 10
	for i := 0; i < scrollCount; i++ {
		log.Printf("       ... 滾動與解析 %d/%d", i+1, scrollCount)

		var currentHTML string
		// 執行滾動並抓取當下 HTML
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(c context.Context) error {
				var currentURL string
				chromedp.Evaluate(`window.location.href`, &currentURL).Do(c)
				if strings.Contains(currentURL, "login") {
					return fmt.Errorf("被導向登入頁面")
				}
				return nil
			}),
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(4*time.Second), // Threads 載入較慢，給多一點時間
			chromedp.OuterHTML("body", &currentHTML),
		)
		if err != nil {
			log.Printf("滾動中斷: %v", err)
			break
		}

		// 解析當前批次
		currentBatch := parseThreadsHTML(userID, url, currentHTML)

		// 加入總表 (去重)
		newCount := 0
		for _, meme := range currentBatch {
			if !seenContent[meme.URL] {
				seenContent[meme.URL] = true
				allMemes = append(allMemes, meme)
				newCount++
			}
		}
		log.Printf("           -> 本次滾動新增 %d 篇 (累計: %d)", newCount, len(allMemes))
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
	footerRegex := regexp.MustCompile(`(?s)讚.*?分享$`)

	doc.Find("div[data-pressable-container='true']").Each(func(i int, s *goquery.Selection) {
		s.Find("br").ReplaceWithHtml("\n")
		rawText := strings.TrimSpace(s.Text())

		cleanText := headerRegex.ReplaceAllString(rawText, "")
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
// 2. Plurk 專用邏輯 (邊滾邊抓)
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
		log.Printf("       ... 滾動與解析 %d/%d", i+1, scrollCount)
		var currentHTML string
		err := chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(3*time.Second),
			chromedp.OuterHTML("body", &currentHTML),
		)
		if err != nil {
			break
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
		log.Printf("           -> 本次滾動新增 %d 篇 (累計: %d)", newCount, len(allMemes))
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
// 3. PTT 專用邏輯 (Colly 防斷線版)
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
		Delay:       2000 * time.Millisecond,
		RandomDelay: 1000 * time.Millisecond,
		Parallelism: 1,
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
			Tags:      "PTT, Joke, " + strings.Join(strings.Fields(title), ", "),
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
