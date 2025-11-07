package options

import (
	"encoding/json"
	"fmt"
	"os"

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
}

const (
	defaultCpu             = "200"
	defaultMemoryMB        = "512"
	defaultDiskMB          = "300"
	defaultVaultRole       = "nomad-workloads"
	defaultVaultChangeMode = "restart"
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
	var runOptions *driver.RunOptions
	runOptsEnv := os.Getenv("DEVCONTAINER_RUN_OPTIONS")
	if runOptsEnv != "" && runOptsEnv != "null" {
		runOptions = &driver.RunOptions{}
		err := json.Unmarshal([]byte(runOptsEnv), runOptions)
		if err != nil {
			return nil, fmt.Errorf("unmarshal run options: %w", err)
		}
	}

	// Parse Vault policies
	var vaultPolicies []string
	vaultPoliciesJSON := os.Getenv("VAULT_POLICIES_JSON")
	if vaultPoliciesJSON != "" {
		err := json.Unmarshal([]byte(vaultPoliciesJSON), &vaultPolicies)
		if err != nil {
			return nil, fmt.Errorf("unmarshal VAULT_POLICIES_JSON: %w", err)
		}
	}

	// Parse Vault secrets
	var vaultSecrets []VaultSecret
	vaultSecretsJSON := os.Getenv("VAULT_SECRETS_JSON")
	if vaultSecretsJSON != "" {
		err := json.Unmarshal([]byte(vaultSecretsJSON), &vaultSecrets)
		if err != nil {
			return nil, fmt.Errorf("unmarshal VAULT_SECRETS_JSON: %w", err)
		}
	}

	opts := &Options{
		DiskMB:     getEnv("NOMAD_DISKMB", defaultDiskMB),
		Token:      "",
		Namespace:  getEnv("NOMAD_NAMESPACE", ""),
		Region:     getEnv("NOMAD_REGION", ""),
		TaskName:   "devpod",
		CPU:        getEnv("NOMAD_CPU", defaultCpu),
		MemoryMB:   getEnv("NOMAD_MEMORYMB", defaultMemoryMB),
		JobId:      getEnv("DEVCONTAINER_ID", "devpod"), // set by devpod
		DriverOpts: runOptions,

		// Vault configuration
		VaultAddr:       os.Getenv("VAULT_ADDR"),
		VaultRole:       getEnv("VAULT_ROLE", defaultVaultRole),
		VaultNamespace:  os.Getenv("VAULT_NAMESPACE"),
		VaultChangeMode: getEnv("VAULT_CHANGE_MODE", defaultVaultChangeMode),
		VaultPolicies:   vaultPolicies,
		VaultSecrets:    vaultSecrets,
	}

	// Validate Vault configuration
	if err := opts.ValidateVault(); err != nil {
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
