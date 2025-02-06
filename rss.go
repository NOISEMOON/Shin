package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"
)

var (
	DB_PATH                     = "data/shin_v3.db"
	_                           = godotenv.Load("data/.env")
	logger                      = log.New(os.Stdout, "", log.LstdFlags)
	hn_regex                    = regexp.MustCompile(`https://news\.ycombinator\.com/item\?id=\d+`)
	pollIntervalSeconds, _      = strconv.Atoi(os.Getenv("POLL_INTERVAL_SECONDS"))
	googleBaseURL               = "https://translate.googleapis.com/translate_a/single"
	freshrssAuthURL             = os.Getenv("FRESHRSS_AUTH_URL")
	freshrssListSubscriptionURL = os.Getenv("FRESHRSS_LIST_SUBSCRIPTION_URL")
	freshrssContentURLPrefix    = os.Getenv("FRESHRSS_CONTENT_URL_PREFIX")
	freshrssFilteredLabel       = os.Getenv("FRESHRSS_FILTERED_LABEL")
	defaultOT                   = os.Getenv("DEFAULT_OT")
	withContentFeeds            = os.Getenv("WITH_CONTENT_FEEDS")
	IMPORTANT_FEEDS             = os.Getenv("IMPORTANT_FEEDS")
	OT_MAP_KEY                  = "otMap"
	otMap                       map[string]string
	newOTMap                    map[string]string
)

func init() {
	// 设置全局的 http.DefaultTransport，跳过 TLS 验证
	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func AsyncTask() {
	logger.Println("Starting loop...")
	logger.Println("pollIntervalSeconds", pollIntervalSeconds)
	otMap = GetOtMap()
	newOTMap = make(map[string]string)
	if defaultOT == "" {
		// 默认从2小时前拉取
		defaultOT = strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)
	}
	logger.Printf("Start otMap: %v newOTMap: %v defaultOT: %s", otMap, newOTMap, defaultOT)

	authToken := rssAuth()

	for {
		func() { // 使用匿名函数包裹 for 循环中的主要逻辑，方便捕获 panic
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("Recovered from panic: %v", r)
				}
			}()

			postID, postItems := fetchNews(authToken)
			if len(postItems) > 0 {
				location, _ := time.LoadLocation("Asia/Shanghai")
				subject := fmt.Sprintf("RSS %s", time.Now().In(location).Format("2006-01-02 15:04:05"))
				InsertPost(postID, subject)
				otMap = newOTMap
				UpdateOtMap(otMap)
			} else {
				logger.Println("No updates.")
			}
			logger.Println("End current loop.")
		}()

		// Sleep 不需要捕获异常，放在循环外
		time.Sleep(time.Duration(pollIntervalSeconds) * time.Second)
	}
}

func rssAuth() string {
	response, err := http.Get(freshrssAuthURL)
	if err != nil {
		logger.Println("Error during RSS auth:", err)
		return ""
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Println("Error reading response body:", err)
		return ""
	}

	re := regexp.MustCompile(`SID=([^\n]+)`)
	match := re.FindStringSubmatch(string(body))
	if len(match) > 1 {
		return match[1]
	}
	logger.Println("SID not found")
	return ""
}

func fetchNews(authToken string) (string, []PostItem) {
	postID := strconv.FormatInt(time.Now().UnixNano(), 10)
	subs := fetchSub(authToken)

	var allPostItems []PostItem
	for _, sub := range subs {
		feedID := sub["id"].(string)
		feedTitle := sub["title"].(string)
		postItems := fetchFeed(postID, feedID, feedTitle, authToken)
		if len(postItems) > 0 {
			InsertPostItems(postItems)
			allPostItems = append(allPostItems, postItems...)
		} else {
			logger.Println("No updates from", feedID, feedTitle)
		}
	}
	return postID, allPostItems
}

func fetchSub(authToken string) []map[string]interface{} {
	var enSub []map[string]interface{}
	client := &http.Client{}
	req, err := http.NewRequest("GET", freshrssListSubscriptionURL, nil)
	if err != nil {
		logger.Println("Failed to create request:", err)
		return enSub
	}
	req.Header.Add("Authorization", fmt.Sprintf("GoogleLogin auth=%s", authToken))

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Error during list subscription:", err)
		return enSub
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err == nil {
			subscriptions := data["subscriptions"].([]interface{})
			for _, sub := range subscriptions {
				item := sub.(map[string]interface{})
				for _, category := range item["categories"].([]interface{}) {
					if freshrssFilteredLabel != "" && category.(map[string]interface{})["label"].(string) != freshrssFilteredLabel {
						continue
					}
					enSub = append(enSub, item)
				}
			}
		} else {
			logger.Println("Failed to parse JSON:", err)
		}
	}
	return enSub
}

func fetchFeed(postID, feedID, feedTitle, authToken string) []PostItem {
	ot := otMap[feedID]
	logger.Printf("feedID: %s feedTitle: %s ot: %s defaultOT: %s", feedID, feedTitle, ot, defaultOT)
	if ot == "" {
		ot = defaultOT
	}

	url := fmt.Sprintf("%s%s?ot=%s", freshrssContentURLPrefix, feedID, ot)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Println("Failed to create request:", err)
		return nil
	}
	req.Header.Add("Authorization", fmt.Sprintf("GoogleLogin auth=%s", authToken))

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Failed to fetch feed:", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		var data map[string]interface{}
		var postItems []PostItem
		if err := json.Unmarshal(body, &data); err == nil {
			items := data["items"].([]interface{})

			if len(items) > 0 {
				crawlTimeStr := items[0].(map[string]interface{})["crawlTimeMsec"].(string)
				crawlTimeInt, err := strconv.ParseInt(crawlTimeStr, 10, 64)
				if err != nil {
					fmt.Println("crawlTimeStr conversion err:", err)
				} else {
					newOT := strconv.FormatInt((crawlTimeInt/1000)+1, 10)
					logger.Printf("crawlTimeInt: %d newOT: %s", crawlTimeInt, newOT)
					newOTMap[feedID] = newOT
				}

				for _, item := range items {
					title := item.(map[string]interface{})["title"].(string)
					cnTitle := translate(title)
					href := item.(map[string]interface{})["canonical"].([]interface{})[0].(map[string]interface{})["href"].(string)

					// For hacker news, use comment link
					if strings.Contains(withContentFeeds, feedID) {
						summary_content := item.(map[string]interface{})["summary"].(map[string]interface{})["content"].(string)
						match := hn_regex.FindString(summary_content)
						if len(match) > 0 {
							href = match
						}
					}

					postItemContent := PostItemContent{
						CnTitle: cnTitle,
						Title:   title,
						Link:    href,
					}

					postItemContentJSON, _ := json.Marshal(postItemContent)
					postItemContentJSONString := string(postItemContentJSON)

					postItems = append(postItems, PostItem{
						ID:        strconv.FormatInt(time.Now().UnixNano(), 10),
						PostID:    postID,
						FeedTitle: feedTitle,
						Content:   postItemContentJSONString,
						MemoID:    "",
					})

					time.Sleep(time.Duration(rand.Intn(10)) * time.Second)
				}

				return postItems
			}
		}
	}
	return nil
}

func translate(text string) string {
	logger.Println("text:", text)
	encodedText := url.QueryEscape(text)
	requestURL := googleBaseURL + "?client=gtx&sl=auto&tl=zh&dt=t&q=" + encodedText

	response, err := http.Get(requestURL)
	if err != nil {
		logger.Println("Translation error:", err)
		return ""
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(response.Body)
		logger.Println("Non-200 response:", response.StatusCode, string(bodyBytes))
		return ""
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Println("Error reading translation response:", err)
		return ""
	}

	var result []interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Println("Failed to parse translation response:", err)
		return ""
	}

	if len(result) > 0 {
		firstItem, ok := result[0].([]interface{})
		if ok && len(firstItem) > 0 {
			secondItem, ok := firstItem[0].([]interface{})
			if ok && len(secondItem) > 0 {
				if translatedText, ok := secondItem[0].(string); ok {
					return translatedText
				}
			}
		}
	}

	return ""
}
