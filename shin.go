package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type EmailSender struct {
	smtpServer string
	smtpPort   int
	login      string
	password   string
}

var (
	logger                      = log.New(os.Stdout, "", log.LstdFlags)
	pollIntervalSeconds         = getenvInt("POLL_INTERVAL_SECONDS", 600)
	googleBaseURL               = "https://translate.googleapis.com/translate_a/single"
	freshrssAuthURL             = os.Getenv("FRESHRSS_AUTH_URL")
	freshrssListSubscriptionURL = os.Getenv("FRESHRSS_LIST_SUBSCRIPTION_URL")
	freshrssContentURLPrefix    = os.Getenv("FRESHRSS_CONTENT_URL_PREFIX")
	freshrssFilteredLabel       = os.Getenv("FRESHRSS_FILTERED_LABEL")
	senderEmail                 = os.Getenv("SENDER_EMAIL")
	senderAuthToken             = os.Getenv("SENDER_AUTH_TOKEN")
	smtpServer                  = os.Getenv("SMTP_SERVER")
	smtpPort                    = getenvInt("SMTP_PORT", 25)
	receiverEmail               = os.Getenv("RECEIVER_EMAIL")
	defaultOT                   = os.Getenv("DEFAULT_OT")
	otMapJSON                   = os.Getenv("OT_MAP_JSON")
	otMap                       map[string]string
	newOTMap                    map[string]string
)

func init() {
	// 设置全局的 http.DefaultTransport，跳过 TLS 验证
	http.DefaultTransport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func getenvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return fallback
}

func main() {
	logger.Println("Starting loop...")
	otMap = make(map[string]string)
	newOTMap = make(map[string]string)
	if defaultOT == "" {
		// 默认从6小时前拉取
		defaultOT = strconv.FormatInt(time.Now().Add(-6*time.Hour).Unix(), 10)
	}
	logger.Println("otMapJSON:", otMapJSON)
	json.Unmarshal([]byte(otMapJSON), &otMap)
	logger.Printf("Start otMap: %v newOTMap: %v defaultOT: %s", otMap, newOTMap, defaultOT)

	authToken := rssAuth()

	emailSender := &EmailSender{
		smtpServer: smtpServer,
		smtpPort:   smtpPort,
		login:      senderEmail,
		password:   senderAuthToken,
	}

	for {
		body := buildMailBody(authToken)
		if body != "" {
			location, _ := time.LoadLocation("Asia/Shanghai")
			subject := fmt.Sprintf("RSS %s", time.Now().In(location).Format("2006-01-02 15:04:05"))

			if emailSender.SendEmail(senderEmail, receiverEmail, subject, body) {
				logger.Println("Email sent successfully.")
				newOTMapJson, _ := json.MarshalIndent(newOTMap, "", " ")
				logger.Printf("Update otMap: %v newOTMap: %s", otMap, string(newOTMapJson))
				otMap = newOTMap
			}

		} else {
			logger.Println("No updates. Don't send email.")
		}
		time.Sleep(time.Duration(pollIntervalSeconds) * time.Second)
	}
}

func (e *EmailSender) SendEmail(from, to, subject, body string) bool {
	msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n\n%s", from, to, subject, body)

	// 连接到 SMTP 服务器
	client, err := smtp.Dial(fmt.Sprintf("%s:%d", e.smtpServer, e.smtpPort))
	if err != nil {
		logger.Println("Failed to connect to SMTP server:", err)
		return false
	}
	defer client.Close()

	// 跳过 TLS 证书验证
	tlsConfig := &tls.Config{
		ServerName:         e.smtpServer,
		InsecureSkipVerify: true, // 跳过 TLS 证书验证
	}

	// 启动 TLS
	if err := client.StartTLS(tlsConfig); err != nil {
		logger.Println("Failed to start TLS:", err)
		return false
	}

	// 进行身份验证
	auth := smtp.PlainAuth("", e.login, e.password, e.smtpServer)
	if err := client.Auth(auth); err != nil {
		logger.Println("Failed to authenticate:", err)
		return false
	}

	// 设置发送者和接收者
	if err := client.Mail(from); err != nil {
		logger.Println("Failed to set mail sender:", err)
		return false
	}
	if err := client.Rcpt(to); err != nil {
		logger.Println("Failed to set mail recipient:", err)
		return false
	}

	// 写入邮件内容
	writer, err := client.Data()
	if err != nil {
		logger.Println("Failed to get writer for mail content:", err)
		return false
	}
	_, err = writer.Write([]byte(msg))
	if err != nil {
		logger.Println("Failed to write email content:", err)
		return false
	}
	writer.Close()

	return true
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

func buildMailBody(authToken string) string {
	subs := rssListSub(authToken)
	body := ""
	for _, sub := range subs {
		feedID := sub["id"].(string)
		feedTitle := sub["title"].(string)
		feedContent := rssFetchFeed(feedID, feedTitle, authToken)
		if feedContent != "" {
			body += feedContent + "\n"
		} else {
			logger.Println("No updates from", feedID, feedTitle)
		}
	}
	return body
}

func rssListSub(authToken string) []map[string]interface{} {
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

func rssFetchFeed(feedID, feedTitle, authToken string) string {
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
		return ""
	}
	req.Header.Add("Authorization", fmt.Sprintf("GoogleLogin auth=%s", authToken))

	resp, err := client.Do(req)
	if err != nil {
		logger.Println("Failed to fetch feed:", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err == nil {
			items := data["items"].([]interface{})
			if len(items) > 0 {
				// 获取 crawlTimeMsec，假设它是一个字符串
				crawlTimeStr := items[0].(map[string]interface{})["crawlTimeMsec"].(string)
				crawlTimeInt, err := strconv.ParseInt(crawlTimeStr, 10, 64)
				if err != nil {
					fmt.Println("crawlTimeStr conversion err:", err)
				} else {
					newOT := strconv.FormatInt((crawlTimeInt/1000)+1, 10)
					logger.Printf("crawlTimeInt: %d newOT: %s", crawlTimeInt, newOT)
					newOTMap[feedID] = newOT
				}

				var content strings.Builder
				content.WriteString(fmt.Sprintf("<h1>%s</h1>\n", feedTitle))
				for _, item := range items {
					title := item.(map[string]interface{})["title"].(string)
					cnTitle := translate(title)
					href := item.(map[string]interface{})["canonical"].([]interface{})[0].(map[string]interface{})["href"].(string)
					content.WriteString(fmt.Sprintf("<li>%s <a href=%s>%s</a></li>", cnTitle, href, title))
					time.Sleep(time.Duration(rand.Intn(10)) * time.Second)
				}
				return content.String()
			}
		}
	}
	return ""
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

	// [[["翻译结果",...]]]
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
