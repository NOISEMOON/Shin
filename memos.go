package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type MemoRequest struct {
	Content    string `json:"content"`
	Visibility string `json:"visibility"`
}

type ClientMemoRequest struct {
	PostItemID  string `json:"postItemID"`
	MemoContent string `json:"memoContent"`
}

func CreateMemo(c *gin.Context) {
	var input ClientMemoRequest

	// 从请求体中获取 memoContent
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构造要发送给外部 Memos API 的请求体
	memo := MemoRequest{
		Content:    input.MemoContent,
		Visibility: "PRIVATE",
	}

	// 将 MemoRequest 转换为 JSON
	memoData, err := json.Marshal(memo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode memo"})
		return
	}

	// 发起 HTTP 请求到 Memos API
	apiURL := os.Getenv("MEMOS_CREATE_API")
	authToken := os.Getenv("MEMO_API_TOKEN")
	logger.Printf("Memos apiURL: %s authToken: %s", apiURL, authToken)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(memoData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send request to Memos API"})
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{"error": string(body)})
		return
	}

	var respData map[string]interface{}
	json.Unmarshal(body, &respData)

	uid := respData["uid"].(string)

	UpdateMemoID(input.PostItemID, uid)

	// 返回成功消息
	c.JSON(http.StatusOK, gin.H{"message": "Memo created successfully!"})
}
