package main

import (
	"strings"
	"testing"
)

func TestParseThreadsHTML(t *testing.T) {
	// 模擬包含 NBSP 空間、翻譯按鈕、以及頁尾數據的 HTML
	// 注意：這裡的 "3.9 萬" 中間我們刻意用 \u00A0 模擬真實情況
	mockHTML := `
	<html><body><div data-pressable-container="true">
		<div>追蹤</div>
		<br>
		<div>這是一篇測試文章。翻譯</div>
		<div>讚 3.9` + "\u00A0" + `萬回覆 728轉發 1,139分享 1,538</div>
	</div></body></html>`

	results := parseThreadsHTML("test_user", "http://test", mockHTML)

	if len(results) == 0 {
		t.Fatal("解析失敗")
	}
	content := results[0].URL

	if !strings.Contains(content, "這是一篇測試文章") {
		t.Error("內文遺失")
	}
	if strings.Contains(content, "翻譯") {
		t.Error("翻譯未移除")
	}
	if strings.Contains(content, "讚") || strings.Contains(content, "回覆") {
		t.Errorf("頁尾未清除乾淨: %s", content)
	}
}
