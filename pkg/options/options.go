package options

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/loft-sh/devpod/pkg/driver"
)

// VaultSecret represents a Vault secret path and its field mappings
type VaultSecret struct {
	Path   string            `json:"path"`   // Vault KV v2 path (e.g., "secret/data/aws/creds")
	Fields map[string]string `json:"fields"` // vault_field -> ENV_VAR_NAME mapping
}

type Options struct {
	// Resources
	DiskMB   string
	CPU      string
	MemoryMB string

	JobId     string
	Namespace string
	Region    string
	TaskName  string

	Token string

	DriverOpts *driver.RunOptions

	// Vault configuration
	VaultAddr       string
	VaultRole       string
	VaultNamespace  string
	VaultChangeMode string
	VaultPolicies   []string
	VaultSecrets    []VaultSecret

	// CSI Storage configuration
	StorageMode  string // "ephemeral" (default) or "persistent"
	CSIPluginID  string // CSI plugin ID, default "ceph-csi"
	CSIClusterID string // Ceph cluster ID (required for persistent mode)
	CSIPool      string // Ceph pool name, default "nomad"
	CSIVaultPath string // Vault path for CSI credentials (e.g., "secret/data/ceph/csi")

	// GPU configuration
	GPUEnabled           bool
	GPUCount             int
	GPUComputeCapability string
}

const (
	defaultCpu             = "200"
	defaultMemoryMB        = "512"
	defaultDiskMB          = "300"
	defaultVaultRole       = "nomad-workloads"
	defaultVaultChangeMode = "restart"

	// CSI Storage defaults
	defaultStorageMode = "ephemeral"
	defaultCSIPluginID = "ceph-csi"
	defaultCSIPool     = "nomad"

	// GPU defaults
	defaultGPUCount = 1

	// Storage mode constants
	StorageModeEphemeral  = "ephemeral"
	StorageModePersistent = "persistent"
)

// Read ENV Vars for option overrides
func FromEnv() (*Options, error) {
	newopts, err := DefaultOptions()
	if err != nil {
		return nil, err
	}

	return newopts, nil
}

func DefaultOptions() (*Options, error) {
	// Load config file (if available for local workspace sources)
	workspacePath := GetWorkspacePath()
	configFile, err := LoadConfigFile(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	var runOptions *driver.RunOptions
	runOptsEnv := os.Getenv("DEVCONTAINER_RUN_OPTIONS")
	if runOptsEnv != "" && runOptsEnv != "null" {
		runOptions = &driver.RunOptions{}
		err := json.Unmarshal([]byte(runOptsEnv), runOptions)
		if err != nil {
			return nil, fmt.Errorf("unmarshal run options: %w", err)
		}
	}

	// Parse Vault policies from env or config
	vaultPolicies := getVaultPolicies(configFile)

	// Parse Vault secrets from env or config
	vaultSecrets, err := getVaultSecrets(configFile)
	if err != nil {
		return nil, err
	}

	// Parse GPU configuration using config file as fallback
	var gpuConfigValue *bool
	var gpuCountConfigValue *int
	var gpuCapabilityConfig string
	if configFile != nil {
		gpuConfigValue = configFile.NomadGPU
		gpuCountConfigValue = configFile.NomadGPUCount
		gpuCapabilityConfig = configFile.NomadGPUComputeCapability
	}
	gpuEnabled := getEnvOrConfigBool("NOMAD_GPU", gpuConfigValue, false)
	gpuCount := getEnvOrConfigInt("NOMAD_GPU_COUNT", gpuCountConfigValue, defaultGPUCount)

	// Get config values for other fields
	var cfg ConfigFile
	if configFile != nil {
		cfg = *configFile
	}

	opts := &Options{
		DiskMB:     getEnvOrConfig("NOMAD_DISKMB", cfg.NomadDiskMB, defaultDiskMB),
		Token:      "",
		Namespace:  getEnvOrConfig("NOMAD_NAMESPACE", cfg.NomadNamespace, ""),
		Region:     getEnvOrConfig("NOMAD_REGION", cfg.NomadRegion, ""),
		TaskName:   getEnv("MACHINE_ID", "devpod"),
		CPU:        getEnvOrConfig("NOMAD_CPU", cfg.NomadCPU, defaultCpu),
		MemoryMB:   getEnvOrConfig("NOMAD_MEMORYMB", cfg.NomadMemoryMB, defaultMemoryMB),
		JobId:      getEnv("MACHINE_ID", "devpod"), // set by devpod for machine providers
		DriverOpts: runOptions,

		// Vault configuration
		VaultAddr:       getEnvOrConfig("VAULT_ADDR", cfg.VaultAddr, ""),
		VaultRole:       getEnvOrConfig("VAULT_ROLE", cfg.VaultRole, defaultVaultRole),
		VaultNamespace:  getEnvOrConfig("VAULT_NAMESPACE", cfg.VaultNamespace, ""),
		VaultChangeMode: getEnvOrConfig("VAULT_CHANGE_MODE", cfg.VaultChangeMode, defaultVaultChangeMode),
		VaultPolicies:   vaultPolicies,
		VaultSecrets:    vaultSecrets,

		// CSI Storage configuration
		StorageMode:  getEnvOrConfig("NOMAD_STORAGE_MODE", cfg.NomadStorageMode, defaultStorageMode),
		CSIPluginID:  getEnvOrConfig("NOMAD_CSI_PLUGIN_ID", cfg.NomadCSIPluginID, defaultCSIPluginID),
		CSIClusterID: getEnvOrConfig("NOMAD_CSI_CLUSTER_ID", cfg.NomadCSIClusterID, ""),
		CSIPool:      getEnvOrConfig("NOMAD_CSI_POOL", cfg.NomadCSIPool, defaultCSIPool),
		CSIVaultPath: getEnvOrConfig("NOMAD_CSI_VAULT_PATH", cfg.NomadCSIVaultPath, ""),

		// GPU configuration
		GPUEnabled:           gpuEnabled,
		GPUCount:             gpuCount,
		GPUComputeCapability: getEnvOrConfig("NOMAD_GPU_COMPUTE_CAPABILITY", gpuCapabilityConfig, ""),
	}

	// Validate Vault configuration
	if err := opts.ValidateVault(); err != nil {
		return nil, err
	}

	// Validate CSI configuration
	if err := opts.ValidateCSI(); err != nil {
		return nil, err
	}

	// Validate GPU configuration
	if err := opts.ValidateGPU(); err != nil {
		return nil, err
	}

	return opts, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getVaultPolicies returns Vault policies from env var (JSON) or config file.
// Environment variable takes precedence.
func getVaultPolicies(configFile *ConfigFile) []string {
	// Check env var first
	vaultPoliciesJSON := os.Getenv("VAULT_POLICIES_JSON")
	if vaultPoliciesJSON != "" {
		var policies []string
		if err := json.Unmarshal([]byte(vaultPoliciesJSON), &policies); err == nil {
			return policies
		}
	}

	// Fall back to config file
	if configFile != nil && len(configFile.VaultPolicies) > 0 {
		return configFile.VaultPolicies
	}

	return nil
}

// getVaultSecrets returns Vault secrets from env var (JSON) or config file.
// Environment variable takes precedence.
func getVaultSecrets(configFile *ConfigFile) ([]VaultSecret, error) {
	// Check env var first
	vaultSecretsJSON := os.Getenv("VAULT_SECRETS_JSON")
	if vaultSecretsJSON != "" {
		var secrets []VaultSecret
		if err := json.Unmarshal([]byte(vaultSecretsJSON), &secrets); err != nil {
			return nil, fmt.Errorf("unmarshal VAULT_SECRETS_JSON: %w", err)
		}
		return secrets, nil
	}

	// Fall back to config file
	if configFile != nil && len(configFile.VaultSecrets) > 0 {
		return configFile.VaultSecrets, nil
	}

	return nil, nil
}

// ValidateVault validates Vault configuration settings
func (o *Options) ValidateVault() error {
	// If no Vault secrets configured, nothing to validate
	if len(o.VaultSecrets) == 0 {
		return nil
	}

	// If Vault secrets are configured, require policies and address
	if len(o.VaultPolicies) == 0 {
		return fmt.Errorf("VAULT_POLICIES_JSON is required when VAULT_SECRETS_JSON is specified")
	}

	if o.VaultAddr == "" {
		return fmt.Errorf("VAULT_ADDR is required when VAULT_SECRETS_JSON is specified")
	}

	// Validate each secret configuration
	for i, secret := range o.VaultSecrets {
		if secret.Path == "" {
			return fmt.Errorf("vault secret at index %d has empty path", i)
		}

		if len(secret.Fields) == 0 {
			return fmt.Errorf("vault secret at index %d (%s) has no field mappings", i, secret.Path)
		}

		// Validate field mappings
		for vaultField, envVar := range secret.Fields {
			if vaultField == "" {
				return fmt.Errorf("vault secret at index %d (%s) has empty field name", i, secret.Path)
			}
			if envVar == "" {
				return fmt.Errorf("vault secret at index %d (%s) has empty environment variable name for field %s", i, secret.Path, vaultField)
			}
		}
	}

	// Validate change mode
	validChangeModes := map[string]bool{
		"restart": true,
		"noop":    true,
		"signal":  true,
	}
	if !validChangeModes[o.VaultChangeMode] {
		return fmt.Errorf("invalid VAULT_CHANGE_MODE: %s (must be restart, noop, or signal)", o.VaultChangeMode)
	}

	return nil
}

// ValidateCSI validates CSI storage configuration settings
func (o *Options) ValidateCSI() error {
	// Validate storage mode value
	if o.StorageMode != StorageModeEphemeral && o.StorageMode != StorageModePersistent {
		return fmt.Errorf("invalid NOMAD_STORAGE_MODE: %s (must be 'ephemeral' or 'persistent')", o.StorageMode)
	}

	// If persistent mode, require cluster ID and Vault path for CSI secrets
	if o.StorageMode == StorageModePersistent {
		if o.CSIClusterID == "" {
			return fmt.Errorf("NOMAD_CSI_CLUSTER_ID is required when NOMAD_STORAGE_MODE is 'persistent'")
		}
		if o.CSIPluginID == "" {
			return fmt.Errorf("NOMAD_CSI_PLUGIN_ID is required when NOMAD_STORAGE_MODE is 'persistent'")
		}
		if o.CSIVaultPath == "" {
			return fmt.Errorf("NOMAD_CSI_VAULT_PATH is required when NOMAD_STORAGE_MODE is 'persistent' (Vault path containing 'userID' and 'userKey' for Ceph CSI)")
		}
		if o.VaultAddr == "" {
			return fmt.Errorf("VAULT_ADDR is required when NOMAD_STORAGE_MODE is 'persistent' (needed to fetch CSI credentials)")
		}
	}

	return nil
}

// ValidateGPU validates GPU configuration settings
func (o *Options) ValidateGPU() error {
	if !o.GPUEnabled {
		return nil
	}

	if o.GPUCount < 1 {
		return fmt.Errorf("NOMAD_GPU_COUNT must be at least 1")
	}

	if o.GPUComputeCapability != "" {
		// Validate format X.Y
		parts := strings.Split(o.GPUComputeCapability, ".")
		if len(parts) != 2 {
			return fmt.Errorf("NOMAD_GPU_COMPUTE_CAPABILITY must be in X.Y format (e.g., '7.5')")
		}
		for _, p := range parts {
			if _, err := strconv.Atoi(p); err != nil {
				return fmt.Errorf("NOMAD_GPU_COMPUTE_CAPABILITY must be numeric (e.g., '7.5')")
			}
		}
	}

	return nil
}

// GetVolumeID returns the CSI volume ID for this workspace
func (o *Options) GetVolumeID() string {
	return "devpod-" + o.JobId
}
