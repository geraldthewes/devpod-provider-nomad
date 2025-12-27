package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/briancain/devpod-provider-nomad/pkg/nomad"
	opts "github.com/briancain/devpod-provider-nomad/pkg/options"
	"github.com/briancain/devpod-provider-nomad/pkg/vault"
	"github.com/hashicorp/nomad/api"
	"github.com/spf13/cobra"
)

const (
	// Use Ubuntu as default since DevPod's Docker installation script supports it
	// and we need Docker CLI for devcontainer support
	defaultImage = "ubuntu:22.04"
	defaultUser  = "root"
)

// CreateCmd holds the cmd flags
type CreateCmd struct{}

// NewCommandCmd defines a command
func NewCreateCmd() *cobra.Command {
	cmd := &CreateCmd{}
	commandCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new devpod instance on Nomad",
		RunE: func(_ *cobra.Command, args []string) error {
			options, err := opts.FromEnv()
			if err != nil {
				return err
			}

			return cmd.Run(context.Background(), options)
		},
	}

	return commandCmd
}

func (cmd *CreateCmd) Run(
	ctx context.Context,
	options *opts.Options,
) error {
	nomadClient, err := nomad.NewNomad(options)
	if err != nil {
		return err
	}

	// DevPod run option overrides for job
	image := defaultImage
	user := defaultUser
	// Use a shared path that exists at the same location on both host and container
	// This is critical for Docker-in-Docker bind mounts to work correctly
	// Use a single shared directory for all DevPod workspaces
	sharedWorkspacePath := "/tmp/devpod-workspaces"
	env := map[string]string{}
	entrypoint := ""
	// Create shared workspace dir, install dependencies, combine Vault secrets into shared location
	// Combine all vault secrets into /tmp/devpod-workspaces/.vault-secrets
	// Start background process to copy secrets to workspace content directories as they're created
	runCmd := []string{"/bin/sh", "-c", "mkdir -p " + sharedWorkspacePath + " && apt-get update -qq && apt-get install -y -qq curl git ca-certificates && update-ca-certificates && (for f in /secrets/vault-*.env; do [ -f \"$f\" ] && cat \"$f\" >> " + sharedWorkspacePath + "/.vault-secrets; done || true) && sleep 2 && touch /tmp/.devpod-ready && (while true; do find " + sharedWorkspacePath + "/agent/contexts/*/workspaces/*/content -maxdepth 0 -type d 2>/dev/null | while read wsdir; do if [ -f " + sharedWorkspacePath + "/.vault-secrets ] && [ ! -f \"$wsdir/.vault-secrets\" ]; then cp " + sharedWorkspacePath + "/.vault-secrets \"$wsdir/.vault-secrets\" && chmod 644 \"$wsdir/.vault-secrets\"; fi; done; sleep 5; done) & sleep infinity"}
	if options.DriverOpts != nil {
		if options.DriverOpts.Image != "" {
			image = options.DriverOpts.Image
		}
		if options.DriverOpts.User != "" {
			user = options.DriverOpts.User
		}
		if options.DriverOpts.Env != nil {
			// Merge user env vars with our required env vars (ours take precedence)
			for k, v := range options.DriverOpts.Env {
				if _, exists := env[k]; !exists {
					env[k] = v
				}
			}
		}
		if options.DriverOpts.Entrypoint != "" {
			entrypoint = options.DriverOpts.Entrypoint
		}
		if options.DriverOpts.Cmd != nil {
			runCmd = append([]string{entrypoint}, options.DriverOpts.Cmd...)
		}
	} // err if nil?

	cpu, err := strconv.Atoi(options.CPU)
	if err != nil {
		return err
	}
	mem, err := strconv.Atoi(options.MemoryMB)
	if err != nil {
		return err
	}
	disk, err := strconv.Atoi(options.DiskMB)
	if err != nil {
		return err
	}

	// For persistent storage, create CSI volume if it doesn't exist
	var volumeID string
	if options.StorageMode == opts.StorageModePersistent {
		volumeID = options.GetVolumeID()

		// Convert MB to bytes for CSI volume capacity
		capacityBytes := int64(disk) * 1024 * 1024

		// Check if volume already exists
		exists, err := nomadClient.VolumeExists(ctx, volumeID, options.Namespace)
		if err != nil {
			return fmt.Errorf("failed to check if volume exists: %w", err)
		}

		if !exists {
			// Fetch CSI secrets from Vault
			csiSecrets, err := fetchCSISecretsFromVault(options)
			if err != nil {
				return fmt.Errorf("failed to fetch CSI secrets from Vault: %w", err)
			}

			err = nomadClient.CreateCSIVolume(
				ctx,
				volumeID,
				capacityBytes,
				options.CSIPluginID,
				options.CSIClusterID,
				options.CSIPool,
				options.Namespace,
				csiSecrets,
			)
			if err != nil {
				return err
			}
		}
	}

	jobResources := &api.Resources{
		CPU:      &cpu,
		MemoryMB: &mem,
	}

	// Use the machine ID for job name and task group name
	jobName := options.JobId

	// Build Docker volumes list
	// Mount Docker socket from host for Docker-in-Docker support
	// Mount Docker registry certificates for Docker daemon
	// Mount CA certificate source file so update-ca-certificates includes it
	dockerVolumes := []string{
		"/var/run/docker.sock:/var/run/docker.sock",
		"/etc/docker/certs.d:/etc/docker/certs.d:ro",
		"/usr/local/share/ca-certificates/registry.cluster.crt:/usr/local/share/ca-certificates/registry.cluster.crt:ro",
	}

	// Only add the shared workspace bind mount for ephemeral mode
	// For persistent mode, the CSI volume is mounted by Nomad
	if options.StorageMode != opts.StorageModePersistent {
		dockerVolumes = append(dockerVolumes, sharedWorkspacePath+":"+sharedWorkspacePath)
	}

	// Create the base task
	task := &api.Task{
		Name: options.TaskName,
		User: user,
		Env:  env,
		Config: map[string]interface{}{
			"image":        image,
			"args":         runCmd,
			"volumes":      dockerVolumes,
			"privileged":   true,
			"network_mode": "bridge",
		},
		Resources: jobResources,
		Driver:    "docker",
	}

	// Add Vault integration if configured
	if len(options.VaultSecrets) > 0 {
		task.Vault = &api.Vault{
			Policies:   options.VaultPolicies,
			ChangeMode: &options.VaultChangeMode,
		}

		// Set optional Vault fields if provided
		if options.VaultRole != "" {
			task.Vault.Role = options.VaultRole
		}
		if options.VaultNamespace != "" {
			task.Vault.Namespace = &options.VaultNamespace
		}

		// Generate and attach Vault secret templates
		task.Templates = generateVaultTemplates(options.VaultSecrets, options.VaultChangeMode)
	}

	// Build task group with appropriate storage configuration
	taskGroup := &api.TaskGroup{
		Name:  &jobName,
		Tasks: []*api.Task{task},
	}

	if options.StorageMode == opts.StorageModePersistent {
		// Use CSI volume for persistent storage
		// Mount at the same path DevPod expects for workspaces
		volumeName := "workspace"
		persistentMountPath := sharedWorkspacePath // /tmp/devpod-workspaces
		readOnly := false

		taskGroup.Volumes = map[string]*api.VolumeRequest{
			volumeName: {
				Name:           volumeName,
				Type:           "csi",
				Source:         volumeID,
				AccessMode:     string(api.CSIVolumeAccessModeSingleNodeWriter),
				AttachmentMode: string(api.CSIVolumeAttachmentModeFilesystem),
				MountOptions: &api.CSIMountOptions{
					FSType: "ext4",
				},
			},
		}

		// Add volume mount to task
		task.VolumeMounts = []*api.VolumeMount{
			{
				Volume:      &volumeName,
				Destination: &persistentMountPath,
				ReadOnly:    &readOnly,
			},
		}
	} else {
		// Use ephemeral disk for non-persistent storage
		taskGroup.EphemeralDisk = &api.EphemeralDisk{
			SizeMB: &disk,
		}
	}

	job := &api.Job{
		ID:         &options.JobId,
		Name:       &jobName,
		Namespace:  &options.Namespace,
		Region:     &options.Region,
		TaskGroups: []*api.TaskGroup{taskGroup},
	}

	_, err = nomadClient.Create(ctx, job)
	if err != nil {
		return err
	}

	return nil
}

// generateVaultTemplates creates Nomad template stanzas for Vault secrets
func generateVaultTemplates(secrets []opts.VaultSecret, changeMode string) []*api.Template {
	if len(secrets) == 0 {
		return nil
	}

	templates := make([]*api.Template, len(secrets))
	for i, secret := range secrets {
		tmpl := generateSecretTemplate(secret)
		destPath := "secrets/vault-" + strconv.Itoa(i) + ".env"
		templates[i] = &api.Template{
			DestPath:     &destPath,
			EmbeddedTmpl: &tmpl,
			Envvars:      boolPtr(true), // Makes secrets available as environment variables
			ChangeMode:   &changeMode,
		}
	}

	return templates
}

// generateSecretTemplate creates a Nomad template string for a single Vault secret
func generateSecretTemplate(secret opts.VaultSecret) string {
	template := "{{- with secret \"" + secret.Path + "\" -}}\n"

	for vaultField, envVar := range secret.Fields {
		template += "export " + envVar + "=\"{{ .Data.data." + vaultField + " }}\"\n"
	}

	template += "{{- end }}\n"  // Don't strip trailing whitespace to preserve newlines
	return template
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// fetchCSISecretsFromVault fetches CSI credentials from Vault
func fetchCSISecretsFromVault(options *opts.Options) (*nomad.CSISecrets, error) {
	// Create Vault client
	vaultClient, err := vault.NewClient(options.VaultAddr, options.VaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Get token from environment (DEVPOD_VAULT_TOKEN)
	token := vault.GetTokenFromEnv()
	if token == "" {
		return nil, fmt.Errorf("DEVPOD_VAULT_TOKEN environment variable is required for fetching CSI secrets")
	}
	vaultClient.SetToken(token)

	// Fetch CSI secrets from Vault
	secrets, err := vaultClient.ReadCSISecrets(options.CSIVaultPath)
	if err != nil {
		return nil, err
	}

	return &nomad.CSISecrets{
		UserID:  secrets.UserID,
		UserKey: secrets.UserKey,
	}, nil
}
