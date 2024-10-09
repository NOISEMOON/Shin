package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite" // SQLite driver
)

// Post represents the shin_post table structure
type Post struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	ReadAt    string `json:"read_at"`
}

type PostItem struct {
	ID        string `json:"id"`
	PostID    string `json:"post_id"`
	FeedTitle string `json:"feed_title"`
	Content   string `json:"content"`
	MemoID    string `json:"memo_id"`
}

type PostItemContent struct {
	CnTitle string `json:"cnTitle"`
	Title   string `json:"title"`
	Link    string `json:"link"`
}

var db *sql.DB
var authToken string

func initDB() {
	authToken = os.Getenv("AUTH_TOKEN")
	var err error
	db, err = sql.Open("sqlite", DB_PATH)
	if err != nil {
		panic("failed to connect database")
	}

	// Create the table if it doesn't exist
	createPostTableSQL := `CREATE TABLE IF NOT EXISTS shin_post (
		id TEXT PRIMARY KEY,
		title TEXT,
		created_at TEXT,
		read_at TEXT
	);`
	if _, err := db.Exec(createPostTableSQL); err != nil {
		panic("failed to create shin_post")
	}

	createPostItemTableSQL := `CREATE TABLE IF NOT EXISTS shin_post_item (
		id TEXT PRIMARY KEY,
		post_id TEXT,
		feed_title TEXT,
		content TEXT,
		memo_id TEXT
	);`
	if _, err := db.Exec(createPostItemTableSQL); err != nil {
		panic("failed to create shin_post_item")
	}
}

// 鉴权中间件
func authMiddleware(c *gin.Context) {
	if c.Request.URL.Path == "/login_page" || c.Request.URL.Path == "/login" {
		c.Next() // 继续处理，不拦截
		return
	}

	// 获取 cookie 中的 token
	token, err := c.Cookie("token")
	if err != nil {
		// 如果 token 不存在或无效，重定向到登录页面
		c.Redirect(http.StatusFound, "/login_page")
		c.Abort() // 终止请求
		return
	}

	// 验证 token
	if token != authToken {
		logger.Println("no token in cookie")
		c.Redirect(http.StatusFound, "/login_page")
		c.Abort()
		return
	}

	// 继续处理请求
	c.Next()
}

func InsertPost(postID, title string) {
	post := Post{
		ID:        postID,
		Title:     title,
		CreatedAt: strconv.FormatInt(time.Now().Unix(), 10),
		ReadAt:    "0", // initially unread
	}

	_, err := db.Exec("INSERT INTO shin_post (id, title, created_at, read_at) VALUES (?, ?, ?, ?)",
		post.ID, post.Title, post.CreatedAt, post.ReadAt)
	if err != nil {
		panic(err) // Handle error appropriately in production
	}
}

func InsertPostItems(items []PostItem) error {
	// 启动事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// 准备插入SQL
	stmt, err := tx.Prepare(`INSERT INTO shin_post_item (id, post_id, feed_title, content, memo_id) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	// 批量插入
	for _, item := range items {
		_, err := stmt.Exec(item.ID, item.PostID, item.FeedTitle, item.Content, item.MemoID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute insert statement: %w", err)
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func getPostItemsGroupedByFeedTitle(postID string) (map[string][]PostItem, error) {
	// 查询 shin_post_item 表的所有记录
	rows, err := db.Query(`SELECT id, feed_title, content, memo_id FROM shin_post_item WHERE post_id = ?`, postID)
	if err != nil {
		return nil, fmt.Errorf("failed to query post items: %w", err)
	}
	defer rows.Close()

	// 用于存储分组结果
	groupedItems := make(map[string][]PostItem)

	for rows.Next() {
		var postItemID string
		var feedTitle string
		var contentStr string
		var memoID string
		if err := rows.Scan(&postItemID, &feedTitle, &contentStr, &memoID); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// 将 content 字符串解析为 JSON 对象
		var content PostItemContent
		if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
			return nil, fmt.Errorf("failed to unmarshal content: %w", err)
		}

		// 按 feed_title 分组
		groupedItems[feedTitle] = append(groupedItems[feedTitle], PostItem{
			ID:        postItemID,
			PostID:    postID,
			FeedTitle: feedTitle,
			Content:   contentStr,
			MemoID:    memoID,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error occurred during row iteration: %w", err)
	}

	return groupedItems, nil
}

func getGroupedPostItemsAsJSON(postID string) (string, error) {
	// 获取按 feed_title 分组的内容
	groupedItems, err := getPostItemsGroupedByFeedTitle(postID)
	if err != nil {
		return "", err
	}

	// 将分组后的数据转换为 JSON 字符串
	jsonBytes, err := json.Marshal(groupedItems)
	if err != nil {
		return "", fmt.Errorf("failed to marshal grouped items to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

func markRead(c *gin.Context) {
	var input struct {
		PostID string `json:"post_id"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新 read_at 字段
	readAt := strconv.FormatInt(time.Now().Unix(), 10)
	_, err := db.Exec("UPDATE shin_post SET read_at = ? WHERE id = ?", readAt, input.PostID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating post"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Post marked as read"})
}

func UpdateMemoID(postItemID, memoID string) {
	logger.Println("UpdateMemoID: ", postItemID, memoID)
	_, err := db.Exec("UPDATE shin_post_item SET memo_id = ? WHERE id = ?", memoID, postItemID)
	if err != nil {
		logger.Println("failed to update memo_id")
		panic(err)
	}
}

func getDetail(c *gin.Context) {
	postID := c.Query("id")
	logger.Println("getDetail: ", postID)
	var post Post
	err := db.QueryRow("SELECT id, title, created_at, read_at FROM shin_post WHERE id = ?", postID).Scan(&post.ID, &post.Title, &post.CreatedAt, &post.ReadAt)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching post"})
		}
		return
	}

	content, _ := getGroupedPostItemsAsJSON(postID)
	post.Content = content
	c.JSON(http.StatusOK, post)
}

// pagePost handler
func pagePost(c *gin.Context) {
	pageNumber, _ := strconv.Atoi(c.Query("page_number"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	var posts []Post
	var totalPosts int64

	err := db.QueryRow("SELECT COUNT(*) FROM shin_post").Scan(&totalPosts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error counting posts"})
		return
	}
	totalPage := (totalPosts + int64(pageSize) - 1) / int64(pageSize)

	rows, err := db.Query("SELECT id, title, created_at, read_at FROM shin_post ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, (pageNumber-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching posts"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var post Post
		if err := rows.Scan(&post.ID, &post.Title, &post.CreatedAt, &post.ReadAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning post"})
			return
		}
		posts = append(posts, post)
	}

	c.JSON(http.StatusOK, gin.H{
		"total_page": totalPage,
		"data":       posts,
	})
}

func searchPostItems(c *gin.Context) {
	keyword := c.Query("keyword")
	logger.Println("search keyword:", keyword)
	query := fmt.Sprintf("SELECT id, post_id, feed_title, content, memo_id FROM shin_post_item WHERE content LIKE '%%%s%%'", keyword)
	rows, err := db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute query"})
		return
	}
	defer rows.Close()

	var postItems []PostItem
	for rows.Next() {
		var item PostItem
		if err := rows.Scan(&item.ID, &item.PostID, &item.FeedTitle, &item.Content, &item.MemoID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan result"})
			return
		}
		postItems = append(postItems, item)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error after scanning results"})
		return
	}

	c.JSON(http.StatusOK, postItems)
}

// 处理登录请求
func processLogin(c *gin.Context) {
	var input struct {
		Token string `form:"token"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logger.Printf("clientToken: %s envToken: %s", input.Token, authToken)

	// 验证 token
	if input.Token == authToken {
		// 设置客户端的 token
		c.SetCookie("token", input.Token, 3600*24*365, "/", "", false, true)

		// 登录成功，重定向到首页
		c.Redirect(http.StatusFound, "/home")
	} else {
		// token 不匹配，返回登录页面
		c.HTML(http.StatusOK, "login.html", gin.H{
			"error": "Invalid token",
		})
	}
}

func main() {

	go AsyncTask()

	r := gin.Default()

	// Initialize the database
	initDB()

	// 应用认证中间件到所有路由
	r.Use(authMiddleware)

	// Serve static files (CSS, JS, images, etc.)
	r.Static("/static", "./static")

	r.LoadHTMLGlob("templates/*")

	r.GET("/login_page", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "home.html", gin.H{})
	})

	r.GET("/home", func(c *gin.Context) {
		c.HTML(http.StatusOK, "home.html", gin.H{})
	})

	r.GET("/detail", func(c *gin.Context) {
		c.HTML(http.StatusOK, "detail.html", gin.H{})
	})

	// REST API routes
	r.POST("/login", processLogin)
	r.POST("/mark_read", markRead)
	r.GET("/page_post", pagePost)
	r.GET("/get_detail", getDetail)
	r.POST("/createMemo", CreateMemo)
	r.GET("/search", searchPostItems)

	r.Run(":8777")
}
