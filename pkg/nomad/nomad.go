package nomad

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/briancain/devpod-provider-nomad/pkg/options"
	"github.com/hashicorp/nomad/api"
	"github.com/loft-sh/devpod/pkg/client"
	"github.com/loft-sh/log"
	dockerterm "github.com/moby/term"
)

type Nomad struct {
	// Nomad client
	client *api.Client
}

func NewNomad(opts *options.Options) (*Nomad, error) {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return nil, err
	}

	return &Nomad{
		client: client,
	}, nil
}

func (n *Nomad) Init(
	ctx context.Context,
) error {
	// List nomad jobs to confirm we can connect
	_, _, err := n.client.Jobs().List(nil)
	if err != nil {
		return err
	}

	return nil
}

func (n *Nomad) Create(
	ctx context.Context,
	job *api.Job,
) (*api.JobRegisterResponse, error) {
	resp, _, err := n.client.Jobs().Register(job, nil)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (n *Nomad) Delete(
	ctx context.Context,
	jobID string,
) error {
	_, _, err := n.client.Jobs().Deregister(jobID, true, nil)
	if err != nil {
		return err
	}

	return nil
}

func (n *Nomad) Status(
	ctx context.Context,
	jobID string,
) (client.Status, *api.Job, error) {
	job, _, err := n.client.Jobs().Info(jobID, nil)
	if err != nil {
		return client.StatusNotFound, job, err
	}

	status := *job.Status
	// Convert to uppercase for consistent comparison
	statusUpper := strings.ToUpper(status)
	switch statusUpper {
	case "PENDING":
		return client.StatusBusy, job, nil
	case "RUNNING":
		return client.StatusRunning, job, nil
	case "COMPLETE":
		return client.StatusStopped, job, nil
	case "DEAD":
		return client.StatusStopped, job, nil
	case "":
		return client.StatusNotFound, job, nil
	default:
		return client.StatusNotFound, job, nil
	}

	return client.StatusNotFound, job, nil
}

// waitForHealthyAllocation polls until a healthy, running allocation is found for the job
func (n *Nomad) waitForHealthyAllocation(
	ctx context.Context,
	jobID string,
	taskName string,
	timeout time.Duration,
) (*api.Allocation, error) {
	logger := log.Default.ErrorStreamOnly()
	logger.Infof("Waiting for healthy allocation for job %q...", jobID)

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for healthy allocation for job %q", jobID)
			}

			// Get allocations for the job
			allocs, _, err := n.client.Jobs().Allocations(jobID, false, nil)
			if err != nil {
				logger.Debugf("Error getting allocations: %v, retrying...", err)
				continue
			}

			if len(allocs) == 0 {
				logger.Debugf("No allocations found for job %q yet, waiting...", jobID)
				continue
			}

			// Look for a running allocation with a running task
			for _, allocStub := range allocs {
				if allocStub.ClientStatus != "running" {
					continue
				}

				// Get full allocation details to check task state
				alloc, _, err := n.client.Allocations().Info(allocStub.ID, nil)
				if err != nil {
					logger.Debugf("Error getting allocation info: %v, retrying...", err)
					continue
				}

				// Check if the specific task is running
				if taskState, ok := alloc.TaskStates[taskName]; ok {
					if taskState.State == "running" {
						// Task is running, now check if it's ready (curl installed)
						// Execute a command to check for the readiness marker
						// Use strings.NewReader for stdin and io.Discard for stdout/stderr
						exitCode, err := n.client.Allocations().Exec(
							ctx,
							alloc,
							taskName,
							false, // no TTY
							[]string{"/bin/sh", "-c", "test -f /tmp/.devpod-ready"},
							strings.NewReader(""), io.Discard, io.Discard,
							nil, nil,
						)
						if err != nil {
							logger.Debugf("Error checking readiness: %v, retrying...", err)
							continue
						}
						if exitCode == 0 {
							logger.Infof("Found healthy allocation %s with running task %q", alloc.ID[:8], taskName)
							return alloc, nil
						}
						logger.Debugf("Task %q is running but not ready yet (curl still installing)...", taskName)
					} else {
						logger.Debugf("Task %q is in state %q, waiting for running state...", taskName, taskState.State)
					}
				} else {
					logger.Debugf("Task %q not found in allocation, waiting...", taskName)
				}
			}

			logger.Debugf("No healthy allocations found yet, retrying...")
		}
	}
}

// Run a command on the instance
func (n *Nomad) CommandDevContainer(
	ctx context.Context,
	jobID string,
	taskName string,
	user string,
	command string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) (int, error) {
	ctx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()

	// Wait for a healthy allocation with the task running
	// Give it up to 5 minutes to start (image pull, task startup, etc.)
	alloc, err := n.waitForHealthyAllocation(ctx, jobID, taskName, 5*time.Minute)
	if err != nil {
		return -1, err
	}
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range signalCh {
			cancelFn()
		}
	}()

	sizeCh := make(chan api.TerminalSize, 1)

	// Detect if stdin is a terminal to determine TTY allocation
	// When DevPod injects its agent, stdin is a pipe (not a terminal)
	// When running interactive commands, stdin is a real terminal
	_, isTTY := dockerterm.GetFdInfo(stdin)

	return n.client.Allocations().Exec(ctx, alloc, taskName, isTTY, []string{"/bin/sh", "-c", command},
		stdin, stdout, stderr, sizeCh, nil)
}
