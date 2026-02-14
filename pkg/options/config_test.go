package options

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile_FileNotFound(t *testing.T) {
	config, err := LoadConfigFile("/nonexistent/path")
	if err != nil {
		t.Errorf("Expected no error for missing file, got: %v", err)
	}
	if config != nil {
		t.Error("Expected nil config for missing file")
	}
}

func TestLoadConfigFile_EmptyPath(t *testing.T) {
	config, err := LoadConfigFile("")
	if err != nil {
		t.Errorf("Expected no error for empty path, got: %v", err)
	}
	if config != nil {
		t.Error("Expected nil config for empty path")
	}
}

func TestLoadConfigFile_ValidYAML(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devpod-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devpod directory
	devpodDir := filepath.Join(tmpDir, ".devpod")
	if err := os.MkdirAll(devpodDir, 0755); err != nil {
		t.Fatalf("Failed to create .devpod dir: %v", err)
	}

	// Write config file
	configContent := `
nomad_cpu: "2000"
nomad_memorymb: "4096"
nomad_diskmb: "10240"
nomad_gpu: true
nomad_gpu_count: 2
nomad_gpu_compute_capability: "7.5"
vault_addr: "https://vault.example.com:8200"
vault_policies:
  - "policy1"
  - "policy2"
vault_secrets:
  - path: "secret/data/test"
    fields:
      api_key: "API_KEY"
      secret: "SECRET_VALUE"
`
	configPath := filepath.Join(devpodDir, "nomad.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	config, err := LoadConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	// Verify values
	if config.NomadCPU != "2000" {
		t.Errorf("Expected NomadCPU=2000, got %s", config.NomadCPU)
	}
	if config.NomadMemoryMB != "4096" {
		t.Errorf("Expected NomadMemoryMB=4096, got %s", config.NomadMemoryMB)
	}
	if config.NomadDiskMB != "10240" {
		t.Errorf("Expected NomadDiskMB=10240, got %s", config.NomadDiskMB)
	}
	if config.NomadGPU == nil || *config.NomadGPU != true {
		t.Error("Expected NomadGPU=true")
	}
	if config.NomadGPUCount == nil || *config.NomadGPUCount != 2 {
		t.Error("Expected NomadGPUCount=2")
	}
	if config.NomadGPUComputeCapability != "7.5" {
		t.Errorf("Expected NomadGPUComputeCapability=7.5, got %s", config.NomadGPUComputeCapability)
	}
	if config.VaultAddr != "https://vault.example.com:8200" {
		t.Errorf("Expected VaultAddr=https://vault.example.com:8200, got %s", config.VaultAddr)
	}
	if len(config.VaultPolicies) != 2 || config.VaultPolicies[0] != "policy1" || config.VaultPolicies[1] != "policy2" {
		t.Errorf("Expected VaultPolicies=[policy1, policy2], got %v", config.VaultPolicies)
	}
	if len(config.VaultSecrets) != 1 {
		t.Fatalf("Expected 1 VaultSecret, got %d", len(config.VaultSecrets))
	}
	if config.VaultSecrets[0].Path != "secret/data/test" {
		t.Errorf("Expected secret path=secret/data/test, got %s", config.VaultSecrets[0].Path)
	}
	if config.VaultSecrets[0].Fields["api_key"] != "API_KEY" {
		t.Errorf("Expected api_key->API_KEY, got %s", config.VaultSecrets[0].Fields["api_key"])
	}
}

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devpod-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devpod directory
	devpodDir := filepath.Join(tmpDir, ".devpod")
	if err := os.MkdirAll(devpodDir, 0755); err != nil {
		t.Fatalf("Failed to create .devpod dir: %v", err)
	}

	// Write invalid config file
	configContent := `
nomad_cpu: [invalid yaml
`
	configPath := filepath.Join(devpodDir, "nomad.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config - should return error
	_, err = LoadConfigFile(tmpDir)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadConfigFile_PartialConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devpod-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devpod directory
	devpodDir := filepath.Join(tmpDir, ".devpod")
	if err := os.MkdirAll(devpodDir, 0755); err != nil {
		t.Fatalf("Failed to create .devpod dir: %v", err)
	}

	// Write partial config file (only GPU settings)
	configContent := `
nomad_gpu: true
nomad_gpu_compute_capability: "8.0"
`
	configPath := filepath.Join(devpodDir, "nomad.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config
	config, err := LoadConfigFile(tmpDir)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	// Verify GPU values are set
	if config.NomadGPU == nil || *config.NomadGPU != true {
		t.Error("Expected NomadGPU=true")
	}
	if config.NomadGPUComputeCapability != "8.0" {
		t.Errorf("Expected NomadGPUComputeCapability=8.0, got %s", config.NomadGPUComputeCapability)
	}

	// Verify other values are empty/nil
	if config.NomadCPU != "" {
		t.Errorf("Expected NomadCPU to be empty, got %s", config.NomadCPU)
	}
	if config.NomadGPUCount != nil {
		t.Errorf("Expected NomadGPUCount to be nil, got %d", *config.NomadGPUCount)
	}
}

func TestGetWorkspacePath_LocalSource(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("WORKSPACE_SOURCE")
	defer func() {
		if orig != "" {
			os.Setenv("WORKSPACE_SOURCE", orig)
		} else {
			os.Unsetenv("WORKSPACE_SOURCE")
		}
	}()

	os.Setenv("WORKSPACE_SOURCE", "local:/home/user/my-project")
	path := GetWorkspacePath()
	expected := "/home/user/my-project"
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetWorkspacePath_GitSource(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("WORKSPACE_SOURCE")
	defer func() {
		if orig != "" {
			os.Setenv("WORKSPACE_SOURCE", orig)
		} else {
			os.Unsetenv("WORKSPACE_SOURCE")
		}
	}()

	os.Setenv("WORKSPACE_SOURCE", "git:https://github.com/example/repo.git")
	path := GetWorkspacePath()
	if path != "" {
		t.Errorf("Expected empty string for git source, got %s", path)
	}
}

func TestGetWorkspacePath_Empty(t *testing.T) {
	// Save and restore original env and cwd
	orig := os.Getenv("WORKSPACE_SOURCE")
	origCwd, _ := os.Getwd()
	defer func() {
		if orig != "" {
			os.Setenv("WORKSPACE_SOURCE", orig)
		} else {
			os.Unsetenv("WORKSPACE_SOURCE")
		}
		os.Chdir(origCwd)
	}()

	// Change to a directory without a config file
	os.Unsetenv("WORKSPACE_SOURCE")
	os.Chdir(os.TempDir())
	path := GetWorkspacePath()
	if path != "" {
		t.Errorf("Expected empty string for unset env and no config in cwd, got %s", path)
	}
}

func TestGetWorkspacePath_FallbackToCwd(t *testing.T) {
	// Save and restore original env and cwd
	orig := os.Getenv("WORKSPACE_SOURCE")
	origCwd, _ := os.Getwd()
	defer func() {
		if orig != "" {
			os.Setenv("WORKSPACE_SOURCE", orig)
		} else {
			os.Unsetenv("WORKSPACE_SOURCE")
		}
		os.Chdir(origCwd)
	}()

	// Create temp directory with config file
	tmpDir, err := os.MkdirTemp("", "devpod-cwd-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devpod/nomad.yaml
	devpodDir := filepath.Join(tmpDir, ".devpod")
	if err := os.MkdirAll(devpodDir, 0755); err != nil {
		t.Fatalf("Failed to create .devpod dir: %v", err)
	}
	configPath := filepath.Join(devpodDir, "nomad.yaml")
	if err := os.WriteFile(configPath, []byte("nomad_gpu: true\n"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set git source (not local) and change to the temp directory
	os.Setenv("WORKSPACE_SOURCE", "git:https://github.com/example/repo.git")
	os.Chdir(tmpDir)

	// Should fall back to cwd since config file exists there
	path := GetWorkspacePath()
	if path != tmpDir {
		t.Errorf("Expected %s (cwd with config), got %s", tmpDir, path)
	}
}

func TestGetEnvOrConfig_EnvTakesPrecedence(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_ENV_VAR")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_ENV_VAR", orig)
		} else {
			os.Unsetenv("TEST_ENV_VAR")
		}
	}()

	os.Setenv("TEST_ENV_VAR", "env_value")
	result := getEnvOrConfig("TEST_ENV_VAR", "config_value", "default_value")
	if result != "env_value" {
		t.Errorf("Expected env_value, got %s", result)
	}
}

func TestGetEnvOrConfig_ConfigUsedWhenNoEnv(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_ENV_VAR_UNSET")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_ENV_VAR_UNSET", orig)
		} else {
			os.Unsetenv("TEST_ENV_VAR_UNSET")
		}
	}()

	os.Unsetenv("TEST_ENV_VAR_UNSET")
	result := getEnvOrConfig("TEST_ENV_VAR_UNSET", "config_value", "default_value")
	if result != "config_value" {
		t.Errorf("Expected config_value, got %s", result)
	}
}

func TestGetEnvOrConfig_DefaultUsedWhenNothingSet(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_ENV_VAR_UNSET2")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_ENV_VAR_UNSET2", orig)
		} else {
			os.Unsetenv("TEST_ENV_VAR_UNSET2")
		}
	}()

	os.Unsetenv("TEST_ENV_VAR_UNSET2")
	result := getEnvOrConfig("TEST_ENV_VAR_UNSET2", "", "default_value")
	if result != "default_value" {
		t.Errorf("Expected default_value, got %s", result)
	}
}

func TestGetEnvOrConfigBool_EnvTakesPrecedence(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_BOOL_VAR")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_BOOL_VAR", orig)
		} else {
			os.Unsetenv("TEST_BOOL_VAR")
		}
	}()

	os.Setenv("TEST_BOOL_VAR", "true")
	configValue := false
	result := getEnvOrConfigBool("TEST_BOOL_VAR", &configValue, false)
	if result != true {
		t.Errorf("Expected true from env, got %v", result)
	}
}

func TestGetEnvOrConfigBool_ConfigUsedWhenNoEnv(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_BOOL_VAR_UNSET")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_BOOL_VAR_UNSET", orig)
		} else {
			os.Unsetenv("TEST_BOOL_VAR_UNSET")
		}
	}()

	os.Unsetenv("TEST_BOOL_VAR_UNSET")
	configValue := true
	result := getEnvOrConfigBool("TEST_BOOL_VAR_UNSET", &configValue, false)
	if result != true {
		t.Errorf("Expected true from config, got %v", result)
	}
}

func TestGetEnvOrConfigInt_EnvTakesPrecedence(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_INT_VAR")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_INT_VAR", orig)
		} else {
			os.Unsetenv("TEST_INT_VAR")
		}
	}()

	os.Setenv("TEST_INT_VAR", "42")
	configValue := 10
	result := getEnvOrConfigInt("TEST_INT_VAR", &configValue, 1)
	if result != 42 {
		t.Errorf("Expected 42 from env, got %d", result)
	}
}

func TestGetEnvOrConfigInt_ConfigUsedWhenNoEnv(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_INT_VAR_UNSET")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_INT_VAR_UNSET", orig)
		} else {
			os.Unsetenv("TEST_INT_VAR_UNSET")
		}
	}()

	os.Unsetenv("TEST_INT_VAR_UNSET")
	configValue := 25
	result := getEnvOrConfigInt("TEST_INT_VAR_UNSET", &configValue, 1)
	if result != 25 {
		t.Errorf("Expected 25 from config, got %d", result)
	}
}

func TestGetEnvOrConfigInt_InvalidEnvFallsToConfig(t *testing.T) {
	// Save and restore original env
	orig := os.Getenv("TEST_INT_INVALID")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_INT_INVALID", orig)
		} else {
			os.Unsetenv("TEST_INT_INVALID")
		}
	}()

	os.Setenv("TEST_INT_INVALID", "not-a-number")
	configValue := 15
	result := getEnvOrConfigInt("TEST_INT_INVALID", &configValue, 1)
	if result != 15 {
		t.Errorf("Expected 15 from config (invalid env), got %d", result)
	}
}

func TestGetEnvOrConfig_EmptyEnvFallsToConfig(t *testing.T) {
	orig := os.Getenv("TEST_EMPTY_ENV")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_EMPTY_ENV", orig)
		} else {
			os.Unsetenv("TEST_EMPTY_ENV")
		}
	}()

	os.Setenv("TEST_EMPTY_ENV", "")
	result := getEnvOrConfig("TEST_EMPTY_ENV", "config_value", "default_value")
	if result != "config_value" {
		t.Errorf("Expected config_value when env is empty, got %s", result)
	}
}

func TestGetEnvOrConfigBool_EmptyEnvFallsToConfig(t *testing.T) {
	orig := os.Getenv("TEST_EMPTY_BOOL_ENV")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_EMPTY_BOOL_ENV", orig)
		} else {
			os.Unsetenv("TEST_EMPTY_BOOL_ENV")
		}
	}()

	os.Setenv("TEST_EMPTY_BOOL_ENV", "")
	configValue := true
	result := getEnvOrConfigBool("TEST_EMPTY_BOOL_ENV", &configValue, false)
	if result != true {
		t.Errorf("Expected true from config when env is empty, got %v", result)
	}
}

func TestGetEnvOrConfigInt_EmptyEnvFallsToConfig(t *testing.T) {
	orig := os.Getenv("TEST_EMPTY_INT_ENV")
	defer func() {
		if orig != "" {
			os.Setenv("TEST_EMPTY_INT_ENV", orig)
		} else {
			os.Unsetenv("TEST_EMPTY_INT_ENV")
		}
	}()

	os.Setenv("TEST_EMPTY_INT_ENV", "")
	configValue := 42
	result := getEnvOrConfigInt("TEST_EMPTY_INT_ENV", &configValue, 1)
	if result != 42 {
		t.Errorf("Expected 42 from config when env is empty, got %d", result)
	}
}
