package main

import (
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
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

// DB initialization
var db *sql.DB

func initDB() {

	env_err := godotenv.Load(".env")
	if env_err != nil {
		logger.Fatalf("Error loading .env file: %v", env_err)
	}

	// 获取数据库文件路径，默认使用当前目录下的 shin.db
	dbPath := os.Getenv("DB_PATH")
	logger.Print("dbPath:", dbPath)
	var err error
	db, err = sql.Open("sqlite", dbPath)
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

func main() {

	go AsyncTask()

	r := gin.Default()

	// Initialize the database
	initDB()

	// Serve static files (CSS, JS, images, etc.)
	r.Static("/static", "./static")

	r.GET("/", func(c *gin.Context) {
		c.File("./templates/post_list.html")
	})

	// Route for the post list page
	r.GET("/post_list", func(c *gin.Context) {
		c.File("./templates/post_list.html")
	})

	// Route for the post detail page
	r.GET("/post_detail", func(c *gin.Context) {
		c.File("./templates/post_detail.html")
	})

	// REST API routes
	r.POST("/create_post", createPost)
	r.POST("/mark_read", markRead)
	r.GET("/page_post", pagePost)
	r.GET("/get_detail", getDetail)

	r.Run(":8080")
}
