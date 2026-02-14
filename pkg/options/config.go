package options

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFile represents the .devpod/nomad.yaml configuration file structure.
// All fields are pointers or strings to distinguish between "not set" and "set to empty/zero".
type ConfigFile struct {
	// Resources
	NomadCPU      string `yaml:"nomad_cpu"`
	NomadMemoryMB string `yaml:"nomad_memorymb"`
	NomadDiskMB   string `yaml:"nomad_diskmb"`

	// Nomad job options
	NomadNamespace string `yaml:"nomad_namespace"`
	NomadRegion    string `yaml:"nomad_region"`

	// GPU configuration
	NomadGPU                  *bool  `yaml:"nomad_gpu"`
	NomadGPUCount             *int   `yaml:"nomad_gpu_count"`
	NomadGPUComputeCapability string `yaml:"nomad_gpu_compute_capability"`

	// CSI Storage configuration
	NomadStorageMode  string `yaml:"nomad_storage_mode"`
	NomadCSIPluginID  string `yaml:"nomad_csi_plugin_id"`
	NomadCSIClusterID string `yaml:"nomad_csi_cluster_id"`
	NomadCSIPool      string `yaml:"nomad_csi_pool"`
	NomadCSIVaultPath string `yaml:"nomad_csi_vault_path"`

	// Vault configuration
	VaultAddr       string   `yaml:"vault_addr"`
	VaultRole       string   `yaml:"vault_role"`
	VaultNamespace  string   `yaml:"vault_namespace"`
	VaultChangeMode string   `yaml:"vault_change_mode"`
	VaultPolicies   []string `yaml:"vault_policies"`

	// VaultSecrets allows defining secrets in native YAML format instead of JSON string
	VaultSecrets []VaultSecret `yaml:"vault_secrets"`
}

// LoadConfigFile reads and parses the .devpod/nomad.yaml file from the workspace path.
// Returns nil, nil if the file does not exist (this is not an error).
// Returns nil, error if the file exists but cannot be parsed.
func LoadConfigFile(workspacePath string) (*ConfigFile, error) {
	if workspacePath == "" {
		return nil, nil
	}

	configPath := filepath.Join(workspacePath, ".devpod", "nomad.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not an error - file simply doesn't exist
		}
		return nil, err
	}

	var config ConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// GetWorkspacePath extracts the workspace path from the WORKSPACE_SOURCE environment variable,
// or falls back to checking the current working directory for a config file.
// This allows users to run "devpod up github.com/..." from inside a local clone
// and still have the config file loaded.
func GetWorkspacePath() string {
	// First check WORKSPACE_SOURCE for local sources
	source := os.Getenv("WORKSPACE_SOURCE")
	if strings.HasPrefix(source, "local:") {
		return strings.TrimPrefix(source, "local:")
	}

	// Fallback: check current working directory for config file
	if cwd, err := os.Getwd(); err == nil {
		configPath := filepath.Join(cwd, ".devpod", "nomad.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return cwd
		}
	}

	return ""
}

// getEnvOrConfig returns the value from environment variable if set,
// otherwise returns the config file value if non-empty,
// otherwise returns the default value.
func getEnvOrConfig(envKey, configValue, defaultValue string) string {
	if value, ok := os.LookupEnv(envKey); ok && value != "" {
		return value
	}
	if configValue != "" {
		return configValue
	}
	return defaultValue
}

// getEnvOrConfigBool returns the boolean value from environment variable if set,
// otherwise returns the config file value if non-nil,
// otherwise returns the default value.
func getEnvOrConfigBool(envKey string, configValue *bool, defaultValue bool) bool {
	if value, ok := os.LookupEnv(envKey); ok && value != "" {
		return strings.ToLower(value) == "true"
	}
	if configValue != nil {
		return *configValue
	}
	return defaultValue
}

// getEnvOrConfigInt returns the int value from environment variable if set,
// otherwise returns the config file value if non-nil,
// otherwise returns the default value.
func getEnvOrConfigInt(envKey string, configValue *int, defaultValue int) int {
	if value, ok := os.LookupEnv(envKey); ok {
		if intVal, err := strconv.Atoi(value); err == nil && intVal > 0 {
			return intVal
		}
	}
	if configValue != nil {
		return *configValue
	}
	return defaultValue
}
