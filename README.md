# Golang_FinalProject
找了些感覺好像有用的東東  
複製文   
https://www.plurk.com/m/copypasta  
https://www.plurk.com/p/on41dn  
https://copylove.app/   
結構簡單的感覺可以爬  
梗圖  
找到一個python的教學，感覺可以參考：https://ithelp.ithome.com.tw/articles/10281312  
不然就是好像有可以抓圖片的app  

# spider.go
```
# 1. 強制關閉所有 Chrome (預防萬一)
taskkill /F /IM chrome.exe

# 2. 建立一個臨時資料夾 (在 C 槽根目錄，確保路徑簡單無誤)
mkdir C:\ChromeTemp -Force

# 3. 使用這個臨時資料夾啟動 Chrome
# 注意：請確認您的 Chrome路徑是否正確，若不確定可用下方通用路徑
& "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222 --user-data-dir="C:\ChromeTemp"

# 4. 登入thread/plurk帳號
```