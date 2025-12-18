package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	videoTable = "video_metrics"
	listenPort = ":8080"
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
	Project   string `json:"project"`
	Event     string `json:"event"`
	UserAgent string `json:"userAgent"`
}

// VideoData Doris 数据格式
type VideoData struct {
	Project   string `json:"project"`
	Event     string `json:"event"`
	UserAgent string `json:"user_agent"`
}

var (
	config      *Config
	logger      *slog.Logger
	dorisClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,              // 增大总连接池（默认 100）
			MaxIdleConnsPerHost: 50,               // 增大每个主机的连接池（默认 2）
			MaxConnsPerHost:     100,              // 限制每个主机的最大连接数
			IdleConnTimeout:     90 * time.Second, // 增加空闲连接超时（默认 90s）
			DisableKeepAlives:   false,            // 启用连接复用
			DisableCompression:  true,             // 禁用压缩（Doris 不需要）
		},
		Timeout: 30 * time.Second, // 增加超时时间以适应 Doris 处理时间
		// 自动跟随重定向
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 允许最多 10 次重定向
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}
)

// loadConfig 加载配置
func loadConfig() (*Config, error) {
	beHTTPAddr := getEnv("DORIS_BE_HTTP", "")

	cfg := &Config{
		DB:     getEnv("DORIS_DATABASE", "video"),
		User:   getEnv("DORIS_USER", "devops"),
		Passwd: getEnv("DORIS_PASSWORD", ""),
	}

	// 验证必需配置
	if beHTTPAddr == "" {
		return nil, fmt.Errorf("DORIS_BE_HTTP 必须设置")
	}

	// 确保有协议前缀
	if !strings.HasPrefix(beHTTPAddr, "http://") && !strings.HasPrefix(beHTTPAddr, "https://") {
		beHTTPAddr = "http://" + beHTTPAddr
	}
	cfg.BEHTTP = beHTTPAddr

	// 验证 HTTP 地址格式
	if !strings.HasPrefix(cfg.BEHTTP, "http://") && !strings.HasPrefix(cfg.BEHTTP, "https://") {
		return nil, fmt.Errorf("BE HTTP 地址格式错误: %s", cfg.BEHTTP)
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

// initLogger 初始化日志记录器
func initLogger() {
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
	if getEnv("LOG_FORMAT", "text") == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
}

// maskPassword 隐藏密码
func maskPassword(pwd string) string {
	if len(pwd) <= 4 {
		return "****"
	}
	return pwd[:2] + "****" + pwd[len(pwd)-2:]
}

// toVideoData 将 VideoRequest 转换为 VideoData
func toVideoData(req VideoRequest) VideoData {
	return VideoData{
		Project:   req.Project,
		Event:     req.Event,
		UserAgent: req.UserAgent,
	}
}

// setCORSHeaders 设置 CORS 响应头
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowCredentials := getEnv("CORS_ALLOW_CREDENTIALS", "false") == "true"
	allowedOrigin := getEnv("CORS_ALLOWED_ORIGIN", "*")

	// 设置允许的源
	// 注意：如果允许凭证，则不能使用通配符 "*"
	if allowCredentials {
		// 允许凭证时，必须指定具体的源
		if allowedOrigin == "*" {
			// 如果配置为 "*" 但需要凭证，则使用请求的 Origin
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		} else if origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
	} else {
		// 不允许凭证时，可以使用通配符
		if allowedOrigin == "*" || origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
	}

	// 允许的方法
	allowedMethods := getEnv("CORS_ALLOWED_METHODS", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Methods", allowedMethods)

	// 允许的头部
	allowedHeaders := getEnv("CORS_ALLOWED_HEADERS", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)

	// 允许携带凭证
	if allowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// 预检请求的缓存时间
	maxAge := getEnv("CORS_MAX_AGE", "3600")
	w.Header().Set("Access-Control-Max-Age", maxAge)
}

// corsMiddleware CORS 中间件
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 设置 CORS 头
		setCORSHeaders(w, r)

		// 处理 OPTIONS 预检请求
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// validateRequest 验证 HTTP 请求
func validateRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// OPTIONS 请求已经在 CORS 中间件中处理，这里直接放行
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if r.Method != http.MethodPost {
			// 确保错误响应也包含 CORS 头
			setCORSHeaders(w, r)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("Content-Type") != "application/json" {
			// 确保错误响应也包含 CORS 头
			setCORSHeaders(w, r)
			http.Error(w, "Invalid content type", http.StatusUnsupportedMediaType)
			return
		}

		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		if !json.Valid(body) {
			// 确保错误响应也包含 CORS 头
			setCORSHeaders(w, r)
			http.Error(w, "Invalid JSON format", http.StatusBadRequest)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	}
}

// videoHandler 处理视频数据写入
func videoHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("处理请求时发生 panic", "error", err)
			// 确保错误响应也包含 CORS 头
			setCORSHeaders(w, r)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	var req VideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("解析请求体失败", "error", err)
		// 确保错误响应也包含 CORS 头
		setCORSHeaders(w, r)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 验证必需字段
	if req.Project == "" || req.Event == "" {
		logger.Warn("缺少必需字段", "project", req.Project, "event", req.Event)
		// 确保错误响应也包含 CORS 头
		setCORSHeaders(w, r)
		http.Error(w, "Missing required fields: project and event", http.StatusBadRequest)
		return
	}

	// 转换为 Doris 数据格式
	data := toVideoData(req)

	// 使用 read_json_by_line=true 时，需要每行一个 JSON 对象
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("序列化数据失败", "error", err)
		// 确保错误响应也包含 CORS 头
		setCORSHeaders(w, r)
		http.Error(w, "Failed to marshal data", http.StatusInternalServerError)
		return
	}
	// 添加换行符，因为 read_json_by_line=true 需要每行一个 JSON
	jsonData = append(jsonData, '\n')

	// 仅在调试模式输出处理日志
	if getEnv("DEBUG", "false") == "true" {
		logger.Debug("处理请求", "project", req.Project, "event", req.Event)
	}
	if err := writeToDoris(jsonData); err != nil {
		logger.Error("写入 Doris 失败", "error", err)
		// 确保错误响应也包含 CORS 头
		setCORSHeaders(w, r)
		http.Error(w, fmt.Sprintf("Doris connection failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	if _, err := w.Write([]byte("Data processed successfully.")); err != nil {
		logger.Error("写入响应失败", "error", err)
	}
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

// writeToDoris 写入数据到 Doris BE
// 直接连接 BE HTTP 端口进行 Stream Load，不经过 FE
func writeToDoris(data []byte) error {
	url := fmt.Sprintf("%s/api/%s/%s/_stream_load", config.BEHTTP, config.DB, videoTable)
	// 减少日志输出以提高性能（仅在调试时启用）
	if getEnv("DEBUG", "false") == "true" {
		logger.Debug("向 Doris BE 发送请求", "url", url, "data", string(data))
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置 ContentLength，这样 Go 会自动处理 100-continue
	req.ContentLength = int64(len(data))

	// 设置请求头（与 curl 脚本保持一致）
	auth := base64.StdEncoding.EncodeToString([]byte(config.User + ":" + config.Passwd))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Expect", "100-continue")
	label := uuid.New().String()
	req.Header.Set("label", label)
	req.Header.Set("format", "json")
	req.Header.Set("read_json_by_line", "true")
	req.Header.Set("columns", "project,event,user_agent")

	resp, err := dorisClient.Do(req)
	if err != nil {
		return fmt.Errorf("doris 连接失败: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("读取 Doris 响应体失败: %w", readErr)
	}

	// 仅在调试模式或错误时输出详细日志
	if getEnv("DEBUG", "false") == "true" || resp.StatusCode != http.StatusOK {
		logger.Debug("Doris 响应", "status_code", resp.StatusCode, "body", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("Doris 返回错误", "status_code", resp.StatusCode, "body", string(body))
		return fmt.Errorf("doris 返回错误 [%d]: %s", resp.StatusCode, string(body))
	}

	// 解析响应体
	var loadResp StreamLoadResponse
	if err := json.Unmarshal(body, &loadResp); err != nil {
		logger.Error("解析响应体失败", "error", err, "body", string(body))
		// 如果无法解析，但状态码是 200，仍然返回错误以便检查
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

	// 仅在调试模式输出成功日志
	if getEnv("DEBUG", "false") == "true" {
		logger.Debug("Doris 写入成功",
			"label", loadResp.Label,
			"loaded_rows", loadResp.NumberLoadedRows,
			"total_rows", loadResp.NumberTotalRows,
			"load_time_ms", loadResp.LoadTimeMs)
	}
	return nil
}

func main() {
	// 初始化日志记录器
	initLogger()

	var err error
	config, err = loadConfig()
	if err != nil {
		logger.Error("配置错误", "error", err)
		os.Exit(1)
	}

	// 打印配置信息
	logger.Info("Doris 配置",
		"be_http", config.BEHTTP,
		"database", config.DB,
		"user", config.User,
		"password", maskPassword(config.Passwd),
		"table", videoTable)

	// 健康检查端点
	http.HandleFunc("/health", corsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "doris-webhook",
		})
	}))

	// 启动服务器
	http.HandleFunc("/video", corsMiddleware(validateRequest(videoHandler)))
	server := &http.Server{
		Addr:           listenPort,
		ReadTimeout:    10 * time.Second,  // 增加读取超时
		WriteTimeout:   30 * time.Second,  // 增加写入超时以匹配 Doris 超时
		IdleTimeout:    120 * time.Second, // 增加空闲连接超时
		MaxHeaderBytes: 1 << 20,           // 1MB 最大请求头
	}

	logger.Info("服务器启动", "port", listenPort, "health_check", fmt.Sprintf("http://localhost%s/health", listenPort))
	if err = server.ListenAndServe(); err != nil {
		logger.Error("服务器启动失败", "error", err)
		os.Exit(1)
	}
}
