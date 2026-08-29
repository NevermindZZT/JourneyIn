package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	journeyin "journeyin"
	"journeyin/internal/application"
	journeymaps "journeyin/internal/maps"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
	"journeyin/internal/transport/httpapi"
	mcptransport "journeyin/internal/transport/mcp"
)

var version = "0.2.0"

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "mcp" && os.Args[2] == "stdio" {
		runStdio()
		return
	}
	listen := envOr("JOURNEYIN_LISTEN", "127.0.0.1:8080")
	dataPath := envOr("JOURNEYIN_DATA_DIR", defaultDataPath())
	flag.StringVar(&listen, "listen", listen, "HTTP listen address")
	flag.StringVar(&dataPath, "data", dataPath, "SQLite database path")
	flag.Parse()
	if dataPath != ":memory:" {
		if absolute, err := filepath.Abs(dataPath); err == nil {
			dataPath = absolute
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		log.Fatal(err)
	}
	database, err := store.Open(ctx, dataPath, migrations)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	app := application.NewTripService(database)
	webFS, err := fs.Sub(journeyin.WebFS, "web/dist")
	if err != nil {
		log.Fatal(err)
	}
	schemaFS, err := fs.Sub(journeyin.SchemaFS, "schemas")
	if err != nil {
		log.Fatal(err)
	}
	api := httpapi.NewServer(app, webFS, schemaFS, version, logger)
	baiduServerAK := os.Getenv("JOURNEYIN_BAIDU_SERVER_AK")
	if strings.TrimSpace(baiduServerAK) == "" {
		baiduServerAK = settingValue(ctx, database, "map.baidu.server_key", os.Getenv("BMAP_WEBAPI_AK"))
	}
	baiduBrowserKey := os.Getenv("JOURNEYIN_BAIDU_BROWSER_AK")
	if strings.TrimSpace(baiduBrowserKey) == "" {
		baiduBrowserKey = settingValue(ctx, database, "map.baidu.browser_key", "")
	}
	amapServerKey := settingValue(ctx, database, "map.amap.server_key", "")
	if strings.TrimSpace(amapServerKey) == "" {
		amapServerKey = os.Getenv("JOURNEYIN_AMAP_SERVER_KEY")
	}
	mapRegistry := journeymaps.NewRegistry(
		journeymaps.NewBaiduProvider(journeymaps.BaiduConfig{ServerAK: baiduServerAK}),
		journeymaps.NewAMapProviderWithConfig("journeyin", journeymaps.AMapConfig{ServerKey: amapServerKey}),
	)
	mapService := application.NewMapService(database, mapRegistry, intEnv("JOURNEYIN_MAP_MAX_CONCURRENCY", 2), intEnv("JOURNEYIN_MAP_DAILY_LIMIT", 0))
	app.SetMapService(mapService)
	api.SetMapRegistry(mapRegistry, baiduBrowserKey)
	api.SetMapService(mapService)
	api.SetSettingsStore(database)
	api.SetShareService(journeyshare.NewService(journeyshare.NewSQLiteStore(database)), envOr("JOURNEYIN_PUBLIC_URL", "http://"+listen))
	api.SetSyncStore(database)
	mcpToken := strings.TrimSpace(os.Getenv("JOURNEYIN_MCP_TOKEN"))
	if !isLoopback(listen) && mcpToken == "" {
		logger.Error("remote listen address requires JOURNEYIN_MCP_TOKEN")
		os.Exit(2)
	}
	mcpServer := mcptransport.NewServer(app, version, schemaFS)
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcptransport.RequireBearer(mcpServer.HTTPHandler(), mcpToken))
	apiHandler := httpapi.RequireAPIAuth(api.Handler(), os.Getenv("JOURNEYIN_API_TOKEN"))
	mux.Handle("/", apiHandler)
	server := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	logger.Info("JourneyIn listening", "addr", listen, "data", dataPath, "version", version)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func runStdio() {
	dataPath := envOr("JOURNEYIN_DATA_DIR", defaultDataPath())
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	migrations, err := fs.Sub(journeyin.MigrationFS, "migrations")
	if err != nil {
		log.New(os.Stderr, "", log.LstdFlags).Fatal(err)
	}
	database, err := store.Open(context.Background(), dataPath, migrations)
	if err != nil {
		log.New(os.Stderr, "", log.LstdFlags).Fatal(err)
	}
	defer database.Close()
	schemaFS, err := fs.Sub(journeyin.SchemaFS, "schemas")
	if err != nil {
		log.New(os.Stderr, "", log.LstdFlags).Fatal(err)
	}
	if err := mcptransport.NewServer(application.NewTripService(database), version, schemaFS).RunStdio(context.Background()); err != nil {
		logger.Error("MCP stdio stopped", "error", err)
		os.Exit(1)
	}
}

func settingValue(ctx context.Context, database *store.Store, key, fallback string) string {
	if value, ok, err := database.GetSetting(ctx, key); err == nil && ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func defaultDataPath() string {
	candidates := []string{filepath.Join("data", "journeyin.db")}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(executableDir, "data", "journeyin.db"), filepath.Join(filepath.Dir(executableDir), "data", "journeyin.db"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "JourneyIn", "journeyin.db")
	}
	return candidates[0]
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
