package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"
)

func TestReadConfigDirectModeUsesLocalDataDir(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))

	cfg := ReadConfig()

	if cfg.DSN != filepath.Join("data", "data.db") {
		t.Fatalf("DSN = %q, want local data db", cfg.DSN)
	}
	if cfg.LogFile != filepath.Join("data", "service.log") {
		t.Fatalf("LogFile = %q, want local service log", cfg.LogFile)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "data", "config.yaml")); err != nil {
		t.Fatalf("local config was not written: %v", err)
	}
}

func TestReadConfigNPMModeUsesHomeDataDir(t *testing.T) {
	tempDir := t.TempDir()
	workDir := filepath.Join(tempDir, "work")
	homeDir := filepath.Join(tempDir, "home")
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(work) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, npmInstallMarkerFileName), nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	chdirForTest(t, workDir)
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), homeDir)

	cfg := ReadConfig()

	dataDir := filepath.Join(homeDir, homeDataDirName)
	if cfg.DSN != filepath.Join(dataDir, "data.db") {
		t.Fatalf("DSN = %q, want home data db", cfg.DSN)
	}
	if cfg.LogFile != filepath.Join(dataDir, "service.log") {
		t.Fatalf("LogFile = %q, want home service log", cfg.LogFile)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "config.yaml")); err != nil {
		t.Fatalf("home config was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "data", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("workdir config should not be written in npm mode, stat err = %v", err)
	}
}

func TestUpdateConfigWritesConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))

	cfg := ReadConfig()
	cfg.PrintConfig = false
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	updated, err := UpdateConfig(func(config *AppConfig) error {
		config.ServeAt = "127.0.0.1:4040"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if updated.ServeAt != "127.0.0.1:4040" {
		t.Fatalf("updated ServeAt = %q, want persisted value", updated.ServeAt)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, "data", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	if !strings.Contains(string(content), "serveAt: 127.0.0.1:4040") {
		t.Fatalf("config.yaml missing updated serveAt:\n%s", string(content))
	}
}

func TestReadConfigToleratesLegacyKeys(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))

	legacy := "serveAt: :5050\n" +
		"webUrl: /ui\n" +
		"attachmentSizeLimit: 1024\n" +
		"domain: 127.0.0.1:3020\n" +
		"registerOpen: false\n" +
		"imageCompress: false\n" +
		"attachmentConfig:\n" +
		"  useS3: true\n" +
		"  endpoint: http://example.com\n" +
		"  bucket: legacy\n" +
		"  accessKey: \"\"\n" +
		"  secretKey: \"\"\n" +
		"  token: \"\"\n"
	if err := os.MkdirAll(filepath.Join(tempDir, "data"), 0o755); err != nil {
		t.Fatalf("MkdirAll(data) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "data", "config.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy config) error = %v", err)
	}

	cfg := ReadConfig()

	if cfg.ServeAt != ":5050" {
		t.Fatalf("ServeAt = %q, want legacy value", cfg.ServeAt)
	}
	if cfg.StaticMountPath != "/ui" {
		t.Fatalf("StaticMountPath = %q, want legacy webUrl value", cfg.StaticMountPath)
	}
	if cfg.RequestBodyLimitKB != 1024 {
		t.Fatalf("RequestBodyLimitKB = %d, want legacy attachmentSizeLimit value", cfg.RequestBodyLimitKB)
	}
}

func TestReadConfigUsesEnvironmentProbeURLAsDefault(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))
	t.Setenv(proxyHealthProbeURLEnv, " https://cp.cloudflare.com/generate_204 ")

	cfg := ReadConfig()

	if cfg.ProxyHealth.ProbeURL != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("ProbeURL = %q, want trimmed environment default", cfg.ProxyHealth.ProbeURL)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, "data", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	if strings.Contains(string(content), "probeUrl:") {
		t.Fatalf("environment default should remain implicit in config.yaml:\n%s", string(content))
	}

	if _, err := UpdateConfig(func(config *AppConfig) error {
		config.ServeAt = "127.0.0.1:4040"
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	reloaded := ReadConfig()
	if reloaded.ProxyHealth.ProbeURL != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("reloaded ProbeURL = %q, want environment default after unrelated update", reloaded.ProxyHealth.ProbeURL)
	}
}

func TestReadConfigSystemProbeURLOverridesEnvironmentDefault(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))

	if err := os.MkdirAll(filepath.Join(tempDir, "data"), 0o755); err != nil {
		t.Fatalf("MkdirAll(data) error = %v", err)
	}
	config := "proxyHealth:\n  probeUrl: https://system.example.test/generate_204\n"
	if err := os.WriteFile(filepath.Join(tempDir, "data", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	t.Setenv(proxyHealthProbeURLEnv, " https://cp.cloudflare.com/generate_204 ")

	cfg := ReadConfig()

	if cfg.ProxyHealth.ProbeURL != "https://system.example.test/generate_204" {
		t.Fatalf("ProbeURL = %q, want system setting to override environment", cfg.ProxyHealth.ProbeURL)
	}
}

func TestReadConfigLegacyGeneratedDefaultAllowsEnvironmentOverride(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))

	if err := os.MkdirAll(filepath.Join(tempDir, "data"), 0o755); err != nil {
		t.Fatalf("MkdirAll(data) error = %v", err)
	}
	config := "proxyHealth:\n  probeUrl: https://www.gstatic.com/generate_204\n"
	if err := os.WriteFile(filepath.Join(tempDir, "data", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	t.Setenv(proxyHealthProbeURLEnv, "https://cp.cloudflare.com/generate_204")

	cfg := ReadConfig()
	if cfg.ProxyHealth.ProbeURL != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("ProbeURL = %q, want environment override for legacy generated default", cfg.ProxyHealth.ProbeURL)
	}
}

func TestUpdateConfigWithProxyHealthPromotesEnvironmentValueToSystemSetting(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	resetConfigForTest(t, filepath.Join(binDir, "proxy-hub"), filepath.Join(tempDir, "home"))
	t.Setenv(proxyHealthProbeURLEnv, "https://cp.cloudflare.com/generate_204")

	ReadConfig()
	if _, err := UpdateConfigWithProxyHealth(func(config *AppConfig) error {
		config.ProxyHealth.ProbeURL = "https://cp.cloudflare.com/generate_204"
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfigWithProxyHealth() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, "data", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(config.yaml) error = %v", err)
	}
	if !strings.Contains(string(content), "probeUrl: https://cp.cloudflare.com/generate_204") {
		t.Fatalf("config.yaml missing promoted probe URL:\n%s", string(content))
	}
	if !strings.Contains(string(content), "probeUrlConfigured: true") {
		t.Fatalf("config.yaml missing explicit probe URL marker:\n%s", string(content))
	}

	t.Setenv(proxyHealthProbeURLEnv, "https://example.com/generate_204")
	reloaded := ReadConfig()
	if reloaded.ProxyHealth.ProbeURL != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("reloaded ProbeURL = %q, want saved system value", reloaded.ProxyHealth.ProbeURL)
	}
}

func resetConfigForTest(t *testing.T, executablePath, homeDir string) {
	t.Helper()

	oldConfigStore := configStore
	oldConfigPath := configPath
	oldConfigHasSystemProxyHealthProbeURL := configHasSystemProxyHealthProbeURL
	configStore = koanf.New(".")
	configPath = ""
	configHasSystemProxyHealthProbeURL = false
	overrideDataDirHooks(t, executablePath, homeDir)

	t.Cleanup(func() {
		configStore = oldConfigStore
		configPath = oldConfigPath
		configHasSystemProxyHealthProbeURL = oldConfigHasSystemProxyHealthProbeURL
	})
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
}
