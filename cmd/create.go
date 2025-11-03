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
	// Create shared workspace dir, install curl and git, create readiness marker, then sleep
	runCmd := []string{"/bin/sh", "-c", "mkdir -p " + sharedWorkspacePath + " && apt-get update -qq && apt-get install -y -qq curl git && sleep 2 && touch /tmp/.devpod-ready && sleep infinity"}
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

	jobResources := &api.Resources{
		CPU:      &cpu,
		MemoryMB: &mem,
	}

	jobName := "devpod"
	job := &api.Job{
		ID:        &options.JobId,
		Name:      &jobName,
		Namespace: &options.Namespace,
		Region:    &options.Region,
		TaskGroups: []*api.TaskGroup{
			{
				Name: &jobName,
				Tasks: []*api.Task{
					{
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
							"volumes": []string{
								"/var/run/docker.sock:/var/run/docker.sock",
								sharedWorkspacePath + ":" + sharedWorkspacePath,
							},
							"privileged": true,
						},
						Resources: jobResources,
						Driver:    "docker",
					},
				},
			},
		},
	}

	_, err = nomad.Create(ctx, job)
	if err != nil {
		return err
	}

	return nil
}
