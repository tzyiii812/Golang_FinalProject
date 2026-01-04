package main

import (
	"os"
	"testing"
)

// TestDatabaseLifecycle 測試資料庫的完整流程：初始化 -> 插入 -> 搜尋 -> 隨機 -> 清理
func TestDatabaseLifecycle(t *testing.T) {
	// 1. 設定一個「測試專用」的資料庫檔名
	testDBFile := "./test_memes_temp.db"

	// 確保測試開始前和結束後，這個暫存檔都會被刪除
	os.Remove(testDBFile)
	defer os.Remove(testDBFile)

	// 2. 初始化資料庫 (傳入測試檔名)
	err := InitDB(testDBFile)
	if err != nil {
		t.Fatalf("初始化測試資料庫失敗: %v", err)
	}
	defer db.Close() // 確保測試結束後關閉連線，釋放檔案鎖定

	// 3. 準備一些假資料 (Mock Data)
	memes := []ExportMeme{
		{
			Title:     "測試文字梗",
			URL:       "這是一段純文字笑話，用來測試文字模式。",
			Tags:      "PTT Joke",
			SourceURL: "http://ptt.cc/test1",
		},
		{
			Title:     "測試圖片梗",
			URL:       "http://example.com/funny.gif",
			Tags:      "GIF",
			SourceURL: "http://gif-vif.com/test2",
		},
	}

	// 4. 測試插入功能 (Insert)
	for _, m := range memes {
		err := InsertMeme(m)
		if err != nil {
			t.Errorf("插入資料失敗 (%s): %v", m.Title, err)
		}
	}

	// 5. 測試資料庫筆數
	count, err := GetMemeCount()
	if err != nil {
		t.Errorf("取得筆數失敗: %v", err)
	}
	if count != 2 {
		t.Errorf("預期資料庫有 2 筆資料，但得到 %d 筆", count)
	}

	// 6. 測試搜尋功能 (Search - All Mode)
	results, err := SearchMemes("笑話", "all")
	if err != nil {
		t.Errorf("搜尋失敗: %v", err)
	}
	if len(results) == 0 {
		t.Error("應該要找到 '測試文字梗'，但結果為空")
	} else if results[0].Title != "測試文字梗" {
		t.Errorf("搜尋結果不符，預期 '測試文字梗'，得到 '%s'", results[0].Title)
	}

	// 7. 測試過濾功能 (Search - Image Mode)
	// 搜尋空白關鍵字(看全部)，但限制 mode=image，應該只回傳 GIF 那筆
	imgResults, err := SearchMemes("", "image")
	if err != nil {
		t.Errorf("圖片搜尋失敗: %v", err)
	}
	if len(imgResults) != 1 {
		t.Errorf("預期找到 1 張圖片，但找到 %d 張", len(imgResults))
	}
	if imgResults[0].URL != "http://example.com/funny.gif" {
		t.Error("圖片過濾邏輯錯誤，找到了非圖片的內容")
	}

	// 8. 測試隨機功能 (Random)
	randMeme, err := GetRandomMeme("all")
	if err != nil {
		t.Errorf("隨機抽取失敗: %v", err)
	}
	if randMeme.Title == "" {
		t.Error("隨機抽取回傳了空物件")
	}
}
