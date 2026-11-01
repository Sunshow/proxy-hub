package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	yamlv3 "gopkg.in/yaml.v3"
)

type ProxyHealthConfig struct {
	Enabled            bool          `json:"enabled" yaml:"enabled" koanf:"enabled"`
	ProbeURL           string        `json:"probeUrl" yaml:"probeUrl" koanf:"probeUrl"`
	ProbeURLConfigured bool          `json:"-" yaml:"probeUrlConfigured,omitempty" koanf:"probeUrlConfigured"`
	Interval           time.Duration `json:"interval" yaml:"interval" koanf:"interval"`
	Timeout            time.Duration `json:"timeout" yaml:"timeout" koanf:"timeout"`
	FailureThreshold   int           `json:"failureThreshold" yaml:"failureThreshold" koanf:"failureThreshold"`
	BlacklistDuration  time.Duration `json:"blacklistDuration" yaml:"blacklistDuration" koanf:"blacklistDuration"`
	MaxConcurrency     int           `json:"maxConcurrency" yaml:"maxConcurrency" koanf:"maxConcurrency"`
}

const proxyHealthProbeURLEnv = "PROXYHUB_PROXY_HEALTH_PROBE_URL"

func DefaultProxyHealthConfig() ProxyHealthConfig {
	return ProxyHealthConfig{
		Enabled:           true,
		ProbeURL:          "https://www.gstatic.com/generate_204",
		Interval:          3 * time.Minute,
		Timeout:           5 * time.Second,
		FailureThreshold:  3,
		BlacklistDuration: 30 * time.Minute,
		MaxConcurrency:    20,
	}
}

type AppConfig struct {
	ServeAt string `json:"serveAt" yaml:"serveAt" koanf:"serveAt"`
	// StaticMountPath 是静态资源的挂载路径，历史配置键名为 webUrl（旧字段名 WebUrl）。
	StaticMountPath string `json:"webUrl" yaml:"webUrl" koanf:"webUrl"`
	// RequestBodyLimitKB 是 HTTP 请求体上限（KB），历史配置键名为 attachmentSizeLimit（旧字段名 AttachmentSizeLimit）。
	RequestBodyLimitKB int64             `json:"attachmentSizeLimit" yaml:"attachmentSizeLimit" koanf:"attachmentSizeLimit"`
	LogFile            string            `json:"logFile" yaml:"logFile" koanf:"logFile"`
	LogLevel           string            `json:"logLevel" yaml:"logLevel" koanf:"logLevel"`
	DBLogLevel         int               `json:"dbLogLevel" yaml:"dbLogLevel" koanf:"dbLogLevel"`
	CorsAllowOrigins   string            `json:"corsAllowOrigins" yaml:"corsAllowOrigins" koanf:"corsAllowOrigins"`
	UIOverwrite        string            `json:"uiOverwrite" yaml:"uiOverwrite" koanf:"uiOverwrite"`
	AutoMigrate        bool              `json:"autoMigrate" yaml:"autoMigrate" koanf:"autoMigrate"`
	OpenAPIEnabled     bool              `json:"openapiEnabled" yaml:"openapiEnabled" koanf:"openapiEnabled"`
	DocsPath           string            `json:"docsPath" yaml:"docsPath" koanf:"docsPath"`
	APITitle           string            `json:"apiTitle" yaml:"apiTitle" koanf:"apiTitle"`
	APIVersion         string            `json:"apiVersion" yaml:"apiVersion" koanf:"apiVersion"`
	ProxyHealth        ProxyHealthConfig `json:"proxyHealth" yaml:"proxyHealth" koanf:"proxyHealth"`
	DSN                string            `json:"dbUrl" yaml:"dbUrl" koanf:"dbUrl"`
	PrintConfig        bool              `json:"printConfig" yaml:"printConfig" koanf:"printConfig"`
}

var configStore = koanf.New(".")
var configPath string
var configMu sync.Mutex
var configHasSystemProxyHealthProbeURL bool

// ReadConfig 会加载当前数据目录下的 config.yaml，若不存在则写入默认配置。
func ReadConfig() *AppConfig {
	dataDir := GetDataDir()
	configPath = filepath.Join(dataDir, "config.yaml")

	defaults := AppConfig{
		ServeAt:            ":3020",
		StaticMountPath:    "/",
		RequestBodyLimitKB: 65536,
		LogFile:            filepath.Join(dataDir, "service.log"),
		LogLevel:           "info",
		CorsAllowOrigins:   "*",
		AutoMigrate:        true,
		OpenAPIEnabled:     true,
		DocsPath:           "/docs",
		APITitle:           "Proxy Hub API",
		APIVersion:         "1.0.0",
		ProxyHealth:        DefaultProxyHealthConfig(),
		DSN:                filepath.Join(dataDir, "data.db"),
		PrintConfig:        true,
	}
	// The environment variable is a deployment-time default.  Loading it
	// before the file provider means an explicitly saved system setting wins.
	if probeURL := strings.TrimSpace(os.Getenv(proxyHealthProbeURLEnv)); probeURL != "" {
		defaults.ProxyHealth.ProbeURL = probeURL
	}

	configStore = koanf.New(".")
	if err := configStore.Load(structs.Provider(&defaults, "yaml"), nil); err != nil {
		panic(err)
	}

	provider := file.Provider(configPath)
	if err := configStore.Load(provider, yaml.Parser()); err != nil {
		fmt.Printf("读取配置失败: %v\n", err)
		if os.IsNotExist(err) {
			// Keep the environment value out of the newly-created file.  It should
			// remain a default so a later system setting can take precedence.
			fileDefaults := defaults
			// The built-in or environment value stays implicit until a user saves
			// the URL from system settings.
			if writeErr := writeConfigFile(configPath, &fileDefaults, true); writeErr != nil {
				fmt.Printf("%v\n", writeErr)
			}
		} else {
			os.Exit(1)
		}
	}
	configHasSystemProxyHealthProbeURL = hasSystemProxyHealthProbeURL(configPath)

	config := defaults
	if err := configStore.Unmarshal("", &config); err != nil {
		fmt.Printf("解析配置失败: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(config.ProxyHealth.ProbeURL) == "" {
		config.ProxyHealth.ProbeURL = strings.TrimSpace(defaults.ProxyHealth.ProbeURL)
	}
	if !configHasSystemProxyHealthProbeURL {
		if probeURL := strings.TrimSpace(os.Getenv(proxyHealthProbeURLEnv)); probeURL != "" {
			config.ProxyHealth.ProbeURL = probeURL
		}
	}

	if config.PrintConfig {
		configStore.Print()
	}

	return &config
}

// UpdateConfig 会基于当前已加载配置应用修改，并写回磁盘。
func UpdateConfig(update func(*AppConfig) error) (*AppConfig, error) {
	return updateConfig(update, false)
}

// UpdateConfigWithProxyHealth persists the health probe URL when the caller is
// explicitly changing that system setting, including when it matches the
// current environment-provided default.
func UpdateConfigWithProxyHealth(update func(*AppConfig) error) (*AppConfig, error) {
	return updateConfig(update, true)
}

func updateConfig(update func(*AppConfig) error, persistProxyHealthProbeURL bool) (*AppConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	var config AppConfig
	if err := configStore.Unmarshal("", &config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	previousProbeURL := config.ProxyHealth.ProbeURL

	if update != nil {
		if err := update(&config); err != nil {
			return nil, err
		}
	}
	previousPersistProbeURL := configHasSystemProxyHealthProbeURL
	if persistProxyHealthProbeURL {
		config.ProxyHealth.ProbeURLConfigured = strings.TrimSpace(config.ProxyHealth.ProbeURL) != ""
		configHasSystemProxyHealthProbeURL = strings.TrimSpace(config.ProxyHealth.ProbeURL) != ""
	} else if strings.TrimSpace(config.ProxyHealth.ProbeURL) != strings.TrimSpace(previousProbeURL) {
		config.ProxyHealth.ProbeURLConfigured = strings.TrimSpace(config.ProxyHealth.ProbeURL) != ""
		configHasSystemProxyHealthProbeURL = strings.TrimSpace(config.ProxyHealth.ProbeURL) != ""
	}

	if err := writeConfigLocked(&config); err != nil {
		configHasSystemProxyHealthProbeURL = previousPersistProbeURL
		return nil, err
	}
	return &config, nil
}

// SaveConfig 会将配置写回磁盘，并把失败原因返回给调用方。
func SaveConfig(config *AppConfig) error {
	configMu.Lock()
	defer configMu.Unlock()

	persistProbeURL := config != nil && strings.TrimSpace(config.ProxyHealth.ProbeURL) != ""
	previousPersistProbeURL := configHasSystemProxyHealthProbeURL
	configHasSystemProxyHealthProbeURL = persistProbeURL
	if config != nil {
		config.ProxyHealth.ProbeURLConfigured = persistProbeURL
	}
	if err := writeConfigLocked(config); err != nil {
		configHasSystemProxyHealthProbeURL = previousPersistProbeURL
		return err
	}
	return nil
}

// WriteConfig 会将当前配置写回磁盘，常用于初始化默认配置。
func WriteConfig(config *AppConfig) {
	if err := SaveConfig(config); err != nil {
		fmt.Printf("%v\n", err)
	}
}

func writeConfigFile(targetPath string, config *AppConfig, omitProbeURL bool) error {
	content, err := yamlv3.Marshal(config)
	if err != nil {
		return fmt.Errorf("写入配置失败: 序列化错误: %w", err)
	}
	if omitProbeURL {
		var raw map[string]any
		if err := yamlv3.Unmarshal(content, &raw); err != nil {
			return fmt.Errorf("写入配置失败: 解析序列化结果错误: %w", err)
		}
		if proxyHealth, ok := raw["proxyHealth"].(map[string]any); ok {
			delete(proxyHealth, "probeUrl")
			content, err = yamlv3.Marshal(raw)
			if err != nil {
				return fmt.Errorf("写入配置失败: 序列化结果错误: %w", err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("写入配置失败: 无法创建目录: %w", err)
	}
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return fmt.Errorf("写入配置失败: 无法写入文件: %w", err)
	}
	return nil
}

func writeConfigLocked(config *AppConfig) error {
	if config != nil {
		if err := configStore.Load(structs.Provider(config, "yaml"), nil); err != nil {
			return fmt.Errorf("写入配置失败: 加载配置错误: %w", err)
		}
	}

	raw := configStore.Raw()
	if !configHasSystemProxyHealthProbeURL {
		if proxyHealth, ok := raw["proxyHealth"].(map[string]any); ok {
			proxyHealth = cloneMap(proxyHealth)
			delete(proxyHealth, "probeUrl")
			delete(proxyHealth, "probeUrlConfigured")
			raw["proxyHealth"] = proxyHealth
		}
	}

	content, err := yaml.Parser().Marshal(raw)
	if err != nil {
		return fmt.Errorf("写入配置失败: 序列化错误: %w", err)
	}

	targetPath := configPath
	if targetPath == "" {
		targetPath = filepath.Join(GetDataDir(), "config.yaml")
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("写入配置失败: 无法创建目录: %w", err)
	}
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return fmt.Errorf("写入配置失败: 无法写入文件: %w", err)
	}
	return nil
}

func hasSystemProxyHealthProbeURL(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	fileConfig := koanf.New(".")
	if err := fileConfig.Load(file.Provider(path), yaml.Parser()); err != nil {
		return false
	}
	value, ok := fileConfig.Get("proxyHealth.probeUrl").(string)
	if !ok || strings.TrimSpace(value) == "" {
		return false
	}
	if configured, ok := fileConfig.Get("proxyHealth.probeUrlConfigured").(bool); ok && configured {
		return true
	}
	// Older versions wrote the built-in default to config.yaml on first start.
	// Treat that generated value as a default so a deployment environment can
	// still provide its URL. A value saved from system settings is marked above.
	return strings.TrimSpace(value) != DefaultProxyHealthConfig().ProbeURL
}

func cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
