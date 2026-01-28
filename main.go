package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	videoTable        = "video_metrics"
	listenPort        = ":8080"
	maxRedirects      = 10
	defaultTimeout    = 30 * time.Second
	shutdownTimeout   = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 1 << 20 // 1MB
	maxIdleConns      = 100
	maxIdleConnsPerHost = 50
	maxConnsPerHost   = 100
	idleConnTimeout   = 90 * time.Second
)

// Config Doris 配置
type Config struct {
	BEHTTP string // BE HTTP 地址（用于 Stream Load）
	DB     string
	User   string
	Passwd string
}

// VideoRequest HTTP 请求数据
type VideoRequest struct {
	Project   string `json:"project" binding:"required"`
	Event     string `json:"event" binding:"required"`
	UserAgent string `json:"userAgent"`
}

// VideoData Doris 数据格式
type VideoData struct {
	Project   string `json:"project"`
	Event     string `json:"event"`
	UserAgent string `json:"user_agent"`
	EventTime string `json:"event_time,omitempty"` // 事件时间，格式：YYYY-MM-DD HH:MM:SS
}

// DorisClient Doris 客户端封装
type DorisClient struct {
	config     *Config
	client     *http.Client
	streamURL  string
	authHeader string
	once       sync.Once
}

// App 应用主结构
type App struct {
	config      *Config
	logger      *slog.Logger
	dorisClient *DorisClient
}

// NewDorisClient 创建 Doris 客户端
func NewDorisClient(cfg *Config) *DorisClient {
	dc := &DorisClient{
		config: cfg,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        maxIdleConns,
				MaxIdleConnsPerHost: maxIdleConnsPerHost,
				MaxConnsPerHost:     maxConnsPerHost,
				IdleConnTimeout:     idleConnTimeout,
				DisableKeepAlives:   false,
				DisableCompression:  true,
			},
			Timeout: defaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("重定向次数过多")
				}
				return nil
			},
		},
	}
	// 延迟初始化 URL 和 auth header
	dc.once.Do(dc.init)
	return dc
}

// init 初始化 URL 和认证头（延迟初始化，只执行一次）
func (dc *DorisClient) init() {
	dc.streamURL = fmt.Sprintf("%s/api/%s/%s/_stream_load", dc.config.BEHTTP, dc.config.DB, videoTable)
	auth := base64.StdEncoding.EncodeToString([]byte(dc.config.User + ":" + dc.config.Passwd))
	dc.authHeader = "Basic " + auth
}

// loadConfig 加载配置
func loadConfig() (*Config, error) {
	beHTTPAddr := getEnv("DORIS_BE_HTTP", "")
	if beHTTPAddr == "" {
		return nil, fmt.Errorf("DORIS_BE_HTTP 必须设置")
	}

	// 确保有协议前缀
	if !strings.HasPrefix(beHTTPAddr, "http://") && !strings.HasPrefix(beHTTPAddr, "https://") {
		beHTTPAddr = "http://" + beHTTPAddr
	}

	cfg := &Config{
		BEHTTP: beHTTPAddr,
		DB:     getEnv("DORIS_DATABASE", "video"),
		User:   getEnv("DORIS_USER", "devops"),
		Passwd: getEnv("DORIS_PASSWORD", ""),
	}

	if cfg.Passwd == "" {
		return nil, fmt.Errorf("DORIS_PASSWORD 未设置")
	}

	return cfg, nil
}

// getEnv 获取环境变量
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

var (
	logger *slog.Logger // 全局 logger（向后兼容）
)

// initLogger 初始化日志记录器
func initLogger() *slog.Logger {
	// 获取日志级别
	levelStr := getEnv("LOG_LEVEL", "info")
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// 创建日志选项
	opts := &slog.HandlerOptions{
		Level: level,
	}

	// 根据环境变量选择日志格式
	// JSON 格式更适合生产环境和日志收集系统
	var l *slog.Logger
	if getEnv("LOG_FORMAT", "text") == "json" {
		l = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		l = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	logger = l // 设置全局 logger
	return l
}

// maskPassword 隐藏密码
func maskPassword(pwd string) string {
	if len(pwd) <= 4 {
		return "****"
	}
	return pwd[:2] + "****" + pwd[len(pwd)-2:]
}

// StreamLoadResponse Doris Stream Load 响应
type StreamLoadResponse struct {
	TxnID                  int64  `json:"TxnId"`
	Label                  string `json:"Label"`
	Status                 string `json:"Status"`
	Message                string `json:"Message"`
	NumberTotalRows        int64  `json:"NumberTotalRows"`
	NumberLoadedRows       int64  `json:"NumberLoadedRows"`
	NumberFilteredRows     int64  `json:"NumberFilteredRows"`
	NumberUnselectedRows   int64  `json:"NumberUnselectedRows"`
	LoadBytes              int64  `json:"LoadBytes"`
	LoadTimeMs             int64  `json:"LoadTimeMs"`
	BeginTxnTimeMs         int64  `json:"BeginTxnTimeMs"`
	StreamLoadPutTimeMs    int64  `json:"StreamLoadPutTimeMs"`
	ReadDataTimeMs         int64  `json:"ReadDataTimeMs"`
	WriteDataTimeMs        int64  `json:"WriteDataTimeMs"`
	CommitAndPublishTimeMs int64  `json:"CommitAndPublishTimeMs"`
	ErrorURL               string `json:"ErrorURL"`
}

// WriteToDoris 写入数据到 Doris BE
// 直接连接 BE HTTP 端口进行 Stream Load，不经过 FE
func (dc *DorisClient) WriteToDoris(ctx context.Context, data []byte, logger *slog.Logger) error {
	isDebug := getEnv("DEBUG", "false") == "true"

	if isDebug {
		logger.Debug("向 Doris BE 发送请求", "url", dc.streamURL, "data", string(data))
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", dc.streamURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置 ContentLength，这样 Go 会自动处理 100-continue
	req.ContentLength = int64(len(data))

	// 设置请求头（与 curl 脚本保持一致）
	req.Header.Set("Authorization", dc.authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Expect", "100-continue")
	req.Header.Set("label", uuid.New().String())
	req.Header.Set("format", "json")
	req.Header.Set("read_json_by_line", "true")
	req.Header.Set("columns", "project,event,user_agent,event_time")

	resp, err := dc.client.Do(req)
	if err != nil {
		return fmt.Errorf("doris 连接失败: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("读取 Doris 响应体失败: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("Doris 返回错误", "status_code", resp.StatusCode, "body", string(body))
		return fmt.Errorf("doris 返回错误 [%d]: %s", resp.StatusCode, string(body))
	}

	// 解析响应体
	var loadResp StreamLoadResponse
	if err := json.Unmarshal(body, &loadResp); err != nil {
		logger.Error("解析响应体失败", "error", err, "body", string(body))
		return fmt.Errorf("无法解析 Doris 响应: %s", string(body))
	}

	// 检查实际执行状态
	if loadResp.Status != "Success" {
		logger.Error("Doris stream load 失败",
			"status", loadResp.Status,
			"message", loadResp.Message,
			"error_url", loadResp.ErrorURL)
		return fmt.Errorf("doris stream load 失败: Status=%s, Message=%s, ErrorURL=%s",
			loadResp.Status, loadResp.Message, loadResp.ErrorURL)
	}

	if isDebug {
		logger.Debug("Doris 写入成功",
			"label", loadResp.Label,
			"loaded_rows", loadResp.NumberLoadedRows,
			"total_rows", loadResp.NumberTotalRows,
			"load_time_ms", loadResp.LoadTimeMs)
	}
	return nil
}

// setupRouter 设置路由
func (app *App) setupRouter() *gin.Engine {
	// 根据环境变量设置 Gin 模式
	ginMode := getEnv("GIN_MODE", "release")
	gin.SetMode(ginMode)

	r := gin.New()

	// 使用自定义日志中间件（使用 slog）
	r.Use(app.ginLogger())
	r.Use(gin.Recovery())

	// 配置 CORS
	corsConfig := cors.Config{
		AllowOrigins:     strings.Split(getEnv("CORS_ALLOWED_ORIGIN", "*"), ","),
		AllowMethods:     strings.Split(getEnv("CORS_ALLOWED_METHODS", "GET,POST,OPTIONS"), ","),
		AllowHeaders:     strings.Split(getEnv("CORS_ALLOWED_HEADERS", "Content-Type,Authorization"), ","),
		AllowCredentials: getEnv("CORS_ALLOW_CREDENTIALS", "false") == "true",
		MaxAge:           3600 * time.Second,
	}
	r.Use(cors.New(corsConfig))

	// 健康检查端点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "doris-webhook",
		})
	})

	// 视频数据写入端点
	r.POST("/video", app.videoHandler)

	return r
}

// ginLogger 自定义日志中间件（使用 slog）
func (app *App) ginLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算延迟
		latency := time.Since(start)

		// 构建日志字段
		fields := []interface{}{
			"status", c.Writer.Status(),
			"method", c.Request.Method,
			"path", path,
			"latency", latency,
			"ip", c.ClientIP(),
		}

		if raw != "" {
			fields = append(fields, "query", raw)
		}

		// 使用 With 添加字段
		logger := app.logger.With(fields...)

		if c.Writer.Status() >= http.StatusInternalServerError {
			logger.Error("HTTP 请求")
		} else if c.Writer.Status() >= http.StatusBadRequest {
			logger.Warn("HTTP 请求")
		} else {
			logger.Info("HTTP 请求")
		}
	}
}

// videoHandler 处理视频数据写入
func (app *App) videoHandler(c *gin.Context) {
	var req VideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		app.logger.Warn("请求验证失败", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// 转换为 Doris 数据格式并序列化
	// 使用当前时间作为事件时间
	eventTime := time.Now().Format("2006-01-02 15:04:05")
	jsonData, err := json.Marshal(VideoData{
		Project:   req.Project,
		Event:     req.Event,
		UserAgent: req.UserAgent,
		EventTime: eventTime,
	})
	if err != nil {
		app.logger.Error("序列化数据失败", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to marshal data",
		})
		return
	}
	// 添加换行符，因为 read_json_by_line=true 需要每行一个 JSON
	jsonData = append(jsonData, '\n')

	if getEnv("DEBUG", "false") == "true" {
		app.logger.Debug("处理请求", "project", req.Project, "event", req.Event)
	}

	if err := app.dorisClient.WriteToDoris(c.Request.Context(), jsonData, app.logger); err != nil {
		app.logger.Error("写入 Doris 失败", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Doris connection failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Data processed successfully.",
	})
}

func main() {
	// 初始化日志记录器
	logger := initLogger()

	// 加载配置
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("配置错误", "error", err)
		os.Exit(1)
	}

	// 创建应用实例
	app := &App{
		config:      cfg,
		logger:      logger,
		dorisClient: NewDorisClient(cfg),
	}

	// 打印配置信息
	logger.Info("Doris 配置",
		"be_http", cfg.BEHTTP,
		"database", cfg.DB,
		"user", cfg.User,
		"password", maskPassword(cfg.Passwd),
		"table", videoTable)

	// 设置路由
	router := app.setupRouter()

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:           listenPort,
		Handler:        router,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		IdleTimeout:    idleTimeout,
		MaxHeaderBytes: maxHeaderBytes,
	}

	// 在 goroutine 中启动服务器
	go func() {
		logger.Info("服务器启动", "port", listenPort, "health_check", fmt.Sprintf("http://localhost%s/health", listenPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("服务器启动失败", "error", err)
			os.Exit(1)
		}
	}()

	// 等待中断信号以优雅关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭服务器...")

	// 创建超时上下文，用于优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// 优雅关闭服务器
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("服务器强制关闭", "error", err)
		os.Exit(1)
	}

	logger.Info("服务器已优雅关闭")
}
