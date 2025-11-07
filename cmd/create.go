package cmd

import (
	"context"
	"strconv"

	"github.com/briancain/devpod-provider-nomad/pkg/nomad"
	"github.com/briancain/devpod-provider-nomad/pkg/options"
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
			options, err := options.FromEnv()
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
	options *options.Options,
) error {
	nomad, err := nomad.NewNomad(options)
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
	// Create shared workspace dir, install curl and git, update CA certificates, create readiness marker, then sleep
	runCmd := []string{"/bin/sh", "-c", "mkdir -p " + sharedWorkspacePath + " && apt-get update -qq && apt-get install -y -qq curl git ca-certificates && update-ca-certificates && sleep 2 && touch /tmp/.devpod-ready && sleep infinity"}
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

	jobResources := &api.Resources{
		CPU:      &cpu,
		MemoryMB: &mem,
	}

	jobName := "devpod"

	// Create the base task
	task := &api.Task{
		Name: options.TaskName,
		User: user,
		Env:  env,
		Config: map[string]interface{}{
			"image": image,
			"args":  runCmd,
			// Mount Docker socket from host for Docker-in-Docker support
			// Mount workspace directory at the SAME path on host and container
			// This is critical: when DevPod tells Docker to bind mount paths,
			// Docker needs to find them on the host at the same path
			// Mount Docker registry certificates for Docker daemon
			// Mount CA certificate source file so update-ca-certificates includes it
			"volumes": []string{
				"/var/run/docker.sock:/var/run/docker.sock",
				sharedWorkspacePath + ":" + sharedWorkspacePath,
				"/etc/docker/certs.d:/etc/docker/certs.d:ro",
				"/usr/local/share/ca-certificates/registry.cluster.crt:/usr/local/share/ca-certificates/registry.cluster.crt:ro",
			},
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

	job := &api.Job{
		ID:        &options.JobId,
		Name:      &jobName,
		Namespace: &options.Namespace,
		Region:    &options.Region,
		TaskGroups: []*api.TaskGroup{
			{
				Name: &jobName,
				EphemeralDisk: &api.EphemeralDisk{
					SizeMB: &disk,
				},
				Tasks: []*api.Task{task},
			},
		},
	}

	_, err = nomad.Create(ctx, job)
	if err != nil {
		return err
	}

	return nil
}

// generateVaultTemplates creates Nomad template stanzas for Vault secrets
func generateVaultTemplates(secrets []options.VaultSecret, changeMode string) []*api.Template {
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
func generateSecretTemplate(secret options.VaultSecret) string {
	template := "{{- with secret \"" + secret.Path + "\" -}}\n"

	for vaultField, envVar := range secret.Fields {
		template += "export " + envVar + "=\"{{ .Data.data." + vaultField + " }}\"\n"
	}

	template += "{{- end -}}\n"
	return template
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}
