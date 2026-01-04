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

var threadsUsers = []string{"ctrl.v.book", "shuixian1002"}
var plurkUsers = []string{"copypasta"}

func main() {
	log.Println("=== 獨立爬蟲程序啟動 ===")

	// 1. 強制清除舊資料 (由爬蟲負責清理)
	log.Println("[系統] 正在重置資料庫與備份檔...")
	ResetDBFiles()

	// 2. 初始化全新資料庫
	// 注意：這裡使用常數 DBFile (defined in database.go)
	InitDB(DBFile)
	log.Println("資料庫初始化完成")

	// 3. 執行爬蟲
	StartSpider()

	log.Println("=== 爬蟲程序執行完畢，即將退出 ===")
}

func StartSpider() {
	log.Println("[Spider] 開始執行所有任務...")

	log.Println("[Spider] 步驟 1/3: 開始爬取 GIF 梗圖...")
	RunGifSpider()

	log.Println("[Spider] 步驟 2/3: 開始爬取 PTT Joke...")
	RunPTTSpider()

	log.Println("[Spider] 步驟 3/3: 開始爬取動態網頁 (Threads/Plurk)...")
	runChromedpScrapers()

	log.Println("[Spider] 所有任務完成！")
}

// ---------------------------------------------------------
// GIF 爬蟲 (Colly)
// ---------------------------------------------------------
func RunGifSpider() {
	c := colly.NewCollector(
		colly.CacheDir("./cache"),
		colly.AllowedDomains("www.gif-vif.com"),
	)
	c.Limit(&colly.LimitRule{DomainGlob: "*", Delay: 2 * time.Second, Parallelism: 5})

	c.OnResponse(func(r *colly.Response) {
		if strings.Contains(r.Headers.Get("Content-Type"), "application/json") {
			var htmlFragments []string
			if err := json.Unmarshal(r.Body, &htmlFragments); err == nil {
				for _, htmlContent := range htmlFragments {
					parseGifHTML(htmlContent, c, r.Request)
				}
			}
		} else {
			parseGifHTML(string(r.Body), c, r.Request)
		}
	})

	c.OnHTML(`img.media-show`, func(e *colly.HTMLElement) {
		gifURL := e.Request.AbsoluteURL(e.Attr("src"))
		title := e.Attr("alt")
		if title == "" {
			title = e.DOM.ParentsUntil("html").Find("title").Text()
		}
		tags := strings.Join(strings.Split(title, " "), ", ")

		meme := ExportMeme{Title: title, URL: gifURL, Tags: tags, SourceURL: e.Request.URL.String()}
		SaveToJSON(meme)
		if err := InsertMeme(meme); err == nil {
			log.Printf("[GIF SAVE] %s", title)
		}
	})

	for offset := 0; offset <= 40; offset += 8 {
		c.Visit(fmt.Sprintf("https://www.gif-vif.com/loadMore.php?offset=%d", offset))
	}
	c.Wait()
}

func parseGifHTML(htmlContent string, c *colly.Collector, req *colly.Request) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if exists && strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
			c.Visit(req.AbsoluteURL(link))
		}
	})
	doc.Find("div.gif-item a[href]").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if exists && strings.Contains(link, "/gifs/") && !strings.Contains(link, "/download/") {
			c.Visit(req.AbsoluteURL(link))
		}
	})
}

// ---------------------------------------------------------
// Chromedp 爬蟲 (Threads & Plurk)
// ---------------------------------------------------------
func runChromedpScrapers() {
	log.Println(">>> 嘗試連線 Chrome (ws://127.0.0.1:9222)...")
	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), "ws://127.0.0.1:9222/")
	if allocCtx == nil {
		log.Println("[Spider] 無法連線 Chrome，跳過。")
		return
	}
	defer cancel()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	if len(threadsUsers) > 0 {
		runGenericScraper(ctx, threadsUsers, "Threads", scrapeThreadsUser)
	}
	if len(plurkUsers) > 0 {
		runGenericScraper(ctx, plurkUsers, "Plurk", scrapePlurkUser)
	}
}

func runGenericScraper(ctx context.Context, targets []string, platform string, scrapeFunc func(context.Context, string) ([]ExportMeme, error)) {
	for i, target := range targets {
		log.Printf("--- [%s][%d/%d] 處理: %s ---", platform, i+1, len(targets), target)
		chromedp.Run(ctx, chromedp.Navigate("about:blank"))
		time.Sleep(1 * time.Second)

		memes, err := scrapeFunc(ctx, target)
		if err != nil {
			log.Printf("[錯誤] %s: %v", target, err)
			continue
		}

		count := 0
		for _, meme := range memes {
			SaveToJSON(meme)
			if err := InsertMeme(meme); err == nil {
				count++
			}
		}
		log.Printf("    -> 入庫: %d 筆", count)
		time.Sleep(time.Duration(3+rand.Intn(3)) * time.Second)
	}
}

// [Threads] 使用 Jiggle Scroll (上下震動)
func scrapeThreadsUser(parentCtx context.Context, userID string) ([]ExportMeme, error) {
	url := fmt.Sprintf("https://www.threads.net/@%s", userID)
	var allMemes []ExportMeme
	seenContent := make(map[string]bool)

	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
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

	for i := 0; i < 10; i++ {
		var currentHTML string
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)

		err := chromedp.Run(timeoutCtx,
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(1*time.Second),
			chromedp.Evaluate(`window.scrollBy(0, -300);`, nil), // Jiggle
			chromedp.Sleep(500*time.Millisecond),
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(3*time.Second),
			chromedp.OuterHTML("body", &currentHTML),
		)
		timeoutCancel()

		if err != nil {
			log.Printf("滾動逾時 (跳過): %v", err)
			continue
		}

		batch := parseThreadsHTML(userID, url, currentHTML)
		newCount := 0
		for _, m := range batch {
			if !seenContent[m.URL] {
				seenContent[m.URL] = true
				allMemes = append(allMemes, m)
				newCount++
			}
		}
		log.Printf("            -> 滾動新增 %d 篇", newCount)
	}
	return allMemes, nil
}

// [Threads] 物理淨化 + 強力 Regex
func parseThreadsHTML(author string, sourceURL string, html string) []ExportMeme {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	var results []ExportMeme

	headerRegex := regexp.MustCompile(`(?s)^追蹤.*?更多`)
	// 匹配：[翻譯(可選)]...讚...回覆...轉發...分享...
	footerRegex := regexp.MustCompile(`(?s)(?:翻譯\s*)?讚[^回]*回覆[^轉]*轉發[^分]*分享.*$`)

	doc.Find("div[data-pressable-container='true']").Each(func(i int, s *goquery.Selection) {
		s.Find("br").ReplaceWithHtml("\n")
		rawText := strings.TrimSpace(s.Text())

		// 1. 物理淨化 NBSP
		rawText = strings.ReplaceAll(rawText, "\u00A0", " ")
		// 2. 移除翻譯字眼
		rawText = strings.ReplaceAll(rawText, "翻譯", "")

		cleanText := headerRegex.ReplaceAllString(rawText, "")
		cleanText = footerRegex.ReplaceAllString(cleanText, "")
		cleanText = strings.TrimSpace(cleanText)

		if len(cleanText) > 5 && !strings.Contains(cleanText, "Log in") {
			if strings.HasPrefix(cleanText, author) {
				cleanText = strings.TrimPrefix(cleanText, author)
				cleanText = strings.TrimSpace(cleanText)
			}
			meme := ExportMeme{Title: author, URL: cleanText, Tags: "Threads", SourceURL: sourceURL}
			results = append(results, meme)
		}
	})
	return results
}

// [Plurk] 使用 Jiggle Scroll
func scrapePlurkUser(parentCtx context.Context, userID string) ([]ExportMeme, error) {
	url := fmt.Sprintf("https://www.plurk.com/m/u/%s", userID)
	var allMemes []ExportMeme
	seenContent := make(map[string]bool)

	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Minute)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.ActionFunc(func(c context.Context) error {
			ctxTO, cancelTO := context.WithTimeout(c, 2*time.Second)
			defer cancelTO()
			chromedp.Evaluate(`
				var buttons = document.querySelectorAll('a, button, input');
				for (var i = 0; i < buttons.length; i++) {
					var t = (buttons[i].innerText || buttons[i].value || "").toLowerCase();
					if (t.includes("yes") || t.includes("over 18")) { buttons[i].click(); break; }
				}
			`, nil).Do(ctxTO)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	for i := 0; i < 10; i++ {
		var currentHTML string
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 15*time.Second)
		err := chromedp.Run(timeoutCtx,
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(1*time.Second),
			chromedp.Evaluate(`window.scrollBy(0, -500);`, nil), // Jiggle
			chromedp.Sleep(500*time.Millisecond),
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(3*time.Second),
			chromedp.OuterHTML("body", &currentHTML),
		)
		timeoutCancel()
		if err != nil {
			continue
		}

		batch := parsePlurkHTML(userID, url, currentHTML)
		newCount := 0
		for _, m := range batch {
			if !seenContent[m.URL] {
				seenContent[m.URL] = true
				allMemes = append(allMemes, m)
				newCount++
			}
		}
		log.Printf("            -> 滾動新增 %d 篇", newCount)
	}
	return allMemes, nil
}

func parsePlurkHTML(author string, sourceURL string, html string) []ExportMeme {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	var results []ExportMeme
	doc.Find(".plurk").Each(func(i int, s *goquery.Selection) {
		c := s.Find(".plurk-content")
		c.Find("br").ReplaceWithHtml("\n")
		text := strings.TrimSpace(c.Text())
		if len(text) > 1 && !strings.Contains(text, "含有成人內容") {
			results = append(results, ExportMeme{Title: author, URL: text, Tags: "Plurk", SourceURL: sourceURL})
		}
	})
	return results
}

// ---------------------------------------------------------
// PTT 爬蟲
// ---------------------------------------------------------
func RunPTTSpider() {
	c := colly.NewCollector(
		colly.AllowedDomains("www.ptt.cc"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)
	c.WithTransport(&http.Transport{
		DialContext:     (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	})
	c.SetCookies("https://www.ptt.cc", []*http.Cookie{{Name: "over18", Value: "1", Domain: "www.ptt.cc", Path: "/"}})

	c.OnHTML("div.over18-notice", func(e *colly.HTMLElement) {
		e.Request.Post("/ask/over18", map[string]string{"from": "/bbs/Joke/index.html", "yes": "yes"})
	})

	c.OnHTML("div.r-ent > div.title > a[href]", func(e *colly.HTMLElement) {
		e.Request.Visit(e.Request.AbsoluteURL(e.Attr("href")))
	})

	c.OnResponse(func(r *colly.Response) {
		if !strings.Contains(r.Request.URL.String(), "/M.") {
			return
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			return
		}

		title := doc.Find(".article-metaline:nth-child(2) .article-meta-value").Text()
		if title == "" {
			title = doc.Find("title").Text()
		}

		content := ""
		doc.Find("#main-content").Each(func(i int, s *goquery.Selection) {
			s.Find("div.push, div.article-metaline, div.article-metaline-right").Remove()
			h, _ := s.Html()
			if idx := strings.Index(h, "--"); idx != -1 {
				h = h[:idx]
			}
			d, _ := goquery.NewDocumentFromReader(strings.NewReader(h))
			content = d.Text()
		})
		content = strings.TrimSpace(content)

		if len(content) > 30 {
			m := ExportMeme{Title: title, URL: content, Tags: "PTT Joke", SourceURL: r.Request.URL.String()}
			SaveToJSON(m)
			if InsertMeme(m) == nil {
				log.Printf("[PTT SAVE] %s", title)
			}
		}
	})

	count := 0
	c.OnHTML("div.btn-group-paging > a.btn.wide", func(e *colly.HTMLElement) {
		if strings.Contains(e.Text, "上頁") && count < 2 {
			count++
			e.Request.Visit(e.Request.AbsoluteURL(e.Attr("href")))
		}
	})

	c.Visit("https://www.ptt.cc/bbs/Joke/index.html")
	c.Wait()
	log.Println("[Spider] PTT 爬蟲任務完成")
}
