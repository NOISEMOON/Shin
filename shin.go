package main

import (
	"database/sql"
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
	createTableSQL := `CREATE TABLE IF NOT EXISTS shin_post (
		id TEXT PRIMARY KEY,
		title TEXT,
		content TEXT,
		created_at TEXT,
		read_at TEXT
	);`
	if _, err := db.Exec(createTableSQL); err != nil {
		panic("failed to create table")
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

// createPost handler
func createPost(c *gin.Context) {
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	post := CreatePostDB(input.Title, input.Content)

	c.JSON(http.StatusOK, post)
}

func CreatePostDB(title, content string) Post {
	post := Post{
		ID:        strconv.FormatInt(time.Now().UnixNano(), 10),
		Title:     title,
		Content:   content,
		CreatedAt: strconv.FormatInt(time.Now().Unix(), 10),
		ReadAt:    "0", // initially unread
	}

	_, err := db.Exec("INSERT INTO shin_post (id, title, content, created_at, read_at) VALUES (?, ?, ?, ?, ?)",
		post.ID, post.Title, post.Content, post.CreatedAt, post.ReadAt)
	if err != nil {
		panic(err) // Handle error appropriately in production
	}

	return post
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

func getDetail(c *gin.Context) {
	postID := c.Query("id")
	logger.Println("getDetail: ", postID)
	var post Post
	err := db.QueryRow("SELECT id, title, content, created_at, read_at FROM shin_post WHERE id = ?", postID).Scan(&post.ID, &post.Title, &post.Content, &post.CreatedAt, &post.ReadAt)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching post"})
		}
		return
	}

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

	rows, err := db.Query("SELECT id, title, content, created_at, read_at FROM shin_post ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, (pageNumber-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching posts"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var post Post
		if err := rows.Scan(&post.ID, &post.Title, &post.Content, &post.CreatedAt, &post.ReadAt); err != nil {
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
		c.SetCookie("token", input.Token, 3600, "/", "", false, true)

		// 登录成功，重定向到首页
		c.Redirect(http.StatusFound, "/post_list")
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
		c.HTML(http.StatusOK, "post_list.html", gin.H{})
	})

	r.GET("/post_list", func(c *gin.Context) {
		c.HTML(http.StatusOK, "post_list.html", gin.H{})
	})

	r.GET("/post_detail", func(c *gin.Context) {
		c.HTML(http.StatusOK, "post_detail.html", gin.H{})
	})

	// REST API routes
	r.POST("/login", processLogin)
	r.POST("/create_post", createPost)
	r.POST("/mark_read", markRead)
	r.GET("/page_post", pagePost)
	r.GET("/get_detail", getDetail)
	r.POST("/createMemo", CreateMemo)

	r.Run(":8777")
}
