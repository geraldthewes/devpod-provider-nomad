ixcellent! I've fixed the TTY issue that was causing the inject loop. Here's what was done:

  Root Cause

  The problem was on pkg/nomad/nomad.go:159 where TTY was hardcoded to true:
  return n.client.Allocations().Exec(ctx, alloc, task, true, ...)

  When TTY is enabled, the terminal echoes input and adds control characters, which corrupts DevPod's inject protocol that transfers the agent binary via
  stdin/stdout.

  The Fix

  1. Added import for terminal detection (pkg/nomad/nomad.go:16)

  dockerterm "github.com/moby/term"

  2. Dynamic TTY detection (pkg/nomad/nomad.go:160-163)

  // Detect if stdin is a terminal to determine TTY allocation
  // When DevPod injects its agent, stdin is a pipe (not a terminal)
  // When running interactive commands, stdin is a real terminal
  _, isTTY := dockerterm.GetFdInfo(stdin)

  3. Use detected value instead of hardcoded true (pkg/nomad/nomad.go:165)

  return n.client.Allocations().Exec(ctx, alloc, task, isTTY, ...)

  How This Works

  - During inject: stdin is a pipe → isTTY = false → clean binary transfer works
  - Interactive shells: stdin is a terminal → isTTY = true → proper terminal emulation

  Rebuild and Test

  Now rebuild the provider:
  ./hack/build.sh

  Then reinstall:
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  And test:
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  The "unexpected message during inject: pong" error should now be resolved! 🎉

Summary of Changes

  The issue was that the command operation was trying to execute commands on the Nomad allocation before it was fully healthy and ready. The previous code only
   checked if:
  1. The job status was "running"
  2. An allocation had ClientStatus == "running"

  However, in Nomad, an allocation can be "running" but the tasks within it might still be starting up (pulling images, initializing, etc.).

  The Fix

  I added proper polling logic in pkg/nomad/nomad.go:103-166:

  1. New waitForHealthyAllocation function: This function polls every 2 seconds (up to 5 minutes) to find a healthy allocation where:
    - The allocation's ClientStatus is "running"
    - The specific task (named "devpod") within that allocation has State == "running"
  2. Updated CommandDevContainer function: Replaced the simple status checks with a call to waitForHealthyAllocation, which ensures the task is fully running
  before attempting to execute commands.
  3. Informative logging: Added debug and info logging so you can see the polling progress in the debug output.

  The provider has been successfully rebuilt at release/devpod-provider-nomad-linux-amd64.

  Next Steps

  You can now test the fix with:
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  The command should now wait properly for the Nomad job and task to be fully running before attempting to inject the DevPod agent and execute commands. You
  should see log messages like "Waiting for healthy allocation for job..." during the startup process.


  Issue 1: Command Execution Timing (FIXED ✓)

  Problem: The provider was trying to execute commands before the Nomad allocation was fully healthy and running.

  Solution (pkg/nomad/nomad.go:103-189):
  - Added waitForHealthyAllocation() function that polls every 2 seconds (up to 5 minutes timeout)
  - Checks that the allocation's ClientStatus is "running" AND the task state is "running"
  - Updated CommandDevContainer() to use this polling logic before executing commands
  - Added informative logging so you can see the progress

  Issue 2: Docker Availability (FIXED ✓)

  Problem: DevPod requires Docker to build devcontainers, but Alpine doesn't have Docker and the installation script doesn't support Alpine.

  Solution (cmd/create.go:14-16, 101-109):
  1. Mounted Docker socket: Added /var/run/docker.sock:/var/run/docker.sock volume mount so the container can use the Nomad client's Docker daemon
  2. Set privileged mode: Added privileged: true to allow Docker operations
  3. Changed default image: Switched from alpine to ubuntu:22.04 which is supported by DevPod's Docker installation script

  Testing

  The provider has been rebuilt. Follow the testing instructions in CLAUDE.md:

  devpod delete 'vscode-remote-try-node'
  devpod provider delete nomad
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  You should now see:
  1. "Waiting for healthy allocation..." message
  2. "Found healthy allocation..." once the job is ready
  3. Successful Docker installation in Ubuntu container
  4. The devcontainer workspace starting up properly


Fix for Missing curl/wget

  Problem: The Ubuntu container starts without curl or wget, which DevPod needs to download its agent binary. The provider was reporting the task as "healthy"
  before curl was installed, causing injection failures.

  Solution:

  1. Modified startup command (cmd/create.go:57):
    - Changed from: sleep infinity
    - To: apt-get update -qq && apt-get install -y -qq curl && touch /tmp/.devpod-ready && sleep infinity
    - This installs curl and creates a readiness marker file
  2. Added readiness check (pkg/nomad/nomad.go:152-171):
    - After detecting the task is "running", the provider now executes a test command to check if /tmp/.devpod-ready exists
    - Only returns the allocation as healthy once curl is fully installed
    - Logs: "Task is running but not ready yet (curl still installing)..." during the wait

  Testing Both Your Use Cases

  The provider should now work for both:
  - vscode-remote-try-node (the test repo from Microsoft)
  - Your Python project (github.com/geraldthewes/multistep-transcriber.git)

  Both will use the same base Ubuntu container with Docker socket mounted and curl pre-installed.

  Test with:
  devpod delete 'vscode-remote-try-node'  # or 'multistep-transcriber'
  devpod provider delete nomad
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod up github.com/geraldthewes/multistep-transcriber.git --provider nomad --debug

  You should now see the provider wait for both the task to start AND for curl to be installed before attempting agent injection.

Perfect! I've fixed the nil pointer panic. Here's what was wrong and what I fixed:

  Fix for Panic in Readiness Check

  Problem: The Nomad API client's Exec() function doesn't handle nil values for stdin, stdout, and stderr. When I passed nil for these parameters, it caused a
  segmentation fault (panic) when the client tried to use them.

  Solution (pkg/nomad/nomad.go:162):
  - Changed from: nil, nil, nil, nil, nil
  - To: strings.NewReader(""), io.Discard, io.Discard, nil, nil
  - This provides valid io.Reader/io.Writer objects that satisfy the API requirements
  - strings.NewReader("") provides an empty stdin
  - io.Discard throws away stdout/stderr output from the readiness check

  Current State

  The provider now:
  1. ✅ Creates Ubuntu 22.04 container (instead of Alpine)
  2. ✅ Mounts Docker socket from host (/var/run/docker.sock)
  3. ✅ Installs curl during container startup
  4. ✅ Waits for allocation task to be running
  5. ✅ Polls for readiness marker (/tmp/.devpod-ready) to ensure curl is installed
  6. ✅ Only then allows DevPod agent injection

  Testing

  Try again with the commands from CLAUDE.md:
  devpod delete 'vscode-remote-try-node'  # or 'multistep-transcriber'
  devpod provider delete nomad
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod up github.com/geraldthewes/multistep-transcriber.git --provider nomad --debug

  This should now successfully wait for the container to be ready and inject the DevPod agent without errors.


 Fix for apt Lock Conflict

  Problem: The container startup command was running apt-get install curl, and when DevPod agent tried to install git, it couldn't acquire the apt lock because
   our startup process was still holding it.

  Error was: E: Could not get lock /var/lib/apt/lists/lock. It is held by process 3069 (apt-get)

  Solution (cmd/create.go:57):
  - Changed from: apt-get update -qq && apt-get install -y -qq curl && touch /tmp/.devpod-ready && sleep infinity
  - To: apt-get update -qq && apt-get install -y -qq curl git && sleep 2 && touch /tmp/.devpod-ready && sleep infinity

  Key improvements:
  1. Pre-install git: Install both curl and git together during container startup
  2. Wait for lock release: Added sleep 2 after installation to ensure apt fully releases all locks
  3. Prevent DevPod from using apt: Since git is already installed, DevPod won't need to run apt-get to install it

  Current State

  The provider now:
  - ✅ Waits for Nomad allocation to be healthy
  - ✅ Checks that curl/git installation is complete
  - ✅ Pre-installs all tools DevPod needs (curl, git)
  - ✅ Ensures apt locks are released before marking ready
  - ✅ Mounts Docker socket for devcontainer builds

  Testing

  Run the test again:
  devpod delete 'vscode-remote-try-node'
  devpod provider delete nomad
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  This should now successfully:
  1. Clone the repository (git is already installed)
  2. Install Docker CLI (no apt conflict)
  3. Build and start the devcontainer



Excellent! You're making great progress. I've fixed the Docker-in-Docker bind mount issue. Here's what was wrong and what I fixed:

  The Docker-in-Docker Bind Mount Problem

  What was happening:
  1. DevPod agent (inside Nomad container) created files at /home/devpod/.devpod/agent/contexts/default/workspaces/vscode-remote-try-node/content
  2. DevPod tried to run: docker run --mount type=bind,src=/home/devpod/.devpod/agent/...
  3. Since we mounted the Docker socket, Docker ran on the host, not in the container
  4. Docker looked for /home/devpod/.devpod/... on the host filesystem (not inside the container)
  5. The path didn't exist on the host → exit code 125

  The Core Issue: Path inside the container ≠ Path on the host

  The Fix (cmd/create.go:56-64, 118-120)

  I implemented a "shared path" strategy:

  1. Created shared workspace path: /opt/devpod-workspaces/{jobId}
  2. Mounted at same path on both sides: host:/opt/devpod-workspaces/{jobId} → container:/opt/devpod-workspaces/{jobId}
  3. Configured DevPod to use this path via environment variables:
    - AGENT_PATH=/opt/devpod-workspaces/{jobId}/devpod
    - AGENT_DATA_PATH=/opt/devpod-workspaces/{jobId}/agent
  4. Fixed env merge logic: User-provided env vars are now merged properly without overriding our required settings

  Now when DevPod runs docker run --mount type=bind,src=/opt/devpod-workspaces/{jobId}/..., Docker can find the path on the host!

  Testing

  Important: Since we changed the workspace path, you'll need Nomad to create the directory on the host. Make sure your Nomad client has volumes.enabled = true
   in the Docker plugin config (it usually is by default).

  Run the test:
  devpod delete 'vscode-remote-try-node'
  devpod provider delete nomad
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  This should now successfully start the devcontainer! 🎉


Perfect! I've fixed the Docker-in-Docker bind mount issue. Here's what was wrong and what I fixed:

  The Root Cause

  The AGENT_PATH and AGENT_DATA_PATH environment variables I set in the Nomad container weren't being used. These are provider options read on the client side
  (where devpod up runs), not from the remote container's environment.

  DevPod was still using its defaults:
  - /home/devpod/.devpod/devpod (agent path)
  - /home/devpod/.devpod/agent (data path)

  When DevPod told Docker to bind mount /home/devpod/.devpod/..., Docker looked for that path on the host (not in the container) and couldn't find it → exit
  code 125.

  The Fix

  I implemented a shared path strategy where the same path exists on both host and container:

  1. Updated Nomad Job Config (cmd/create.go:57, 120)

  - Shared path: /tmp/devpod-workspaces (same on both host and container)
  - Volume mount: host:/tmp/devpod-workspaces → container:/tmp/devpod-workspaces
  - Files created at this path inside the container are visible to the host Docker daemon at the same path

  2. Updated Provider Configuration

  - hack/provider/provider.yaml: Changed AGENT_PATH default to /tmp/devpod-workspaces/devpod
  - hack/provider/provider-dev.yaml: Changed AGENT_DATA_PATH default to /tmp/devpod-workspaces/agent
  - Rebuilt with: RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

  Testing

  Now when you run the test, DevPod will:
  1. Use /tmp/devpod-workspaces/... for all workspace files
  2. This path exists at the same location on both host and container
  3. Docker can successfully bind mount these paths

  Test with:
  devpod delete 'vscode-remote-try-node'
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  This should now successfully start the devcontainer! 🎉

Publishing a Release

  The release workflow in .github/workflows/release.yml:1 is already set up. Here's what you need to do:

  Steps to Create a Release:

  1. Create and push a git tag (version must start with v):
  git tag v0.1.0
  git push origin v0.1.0

  2. Create a GitHub Release:
    - Go to your GitHub repo: https://github.com/YOUR_USERNAME/devpod-provider-nomad/releases
    - Click "Draft a new release"
    - Select the tag you just pushed (v0.1.0)
    - Add a title and description for the release
    - Click "Publish release"
  3. The workflow will automatically:
    - Build binaries for all platforms (Linux, macOS, Windows)
    - Build for both amd64 and arm64 architectures
    - Generate SHA256 checksums
    - Create the provider.yaml file
    - Upload all files to your GitHub release

  What Gets Built:

  The workflow builds:
  - devpod-provider-nomad-linux-amd64 (+ .sha256)
  - devpod-provider-nomad-linux-arm64 (+ .sha256)
  - devpod-provider-nomad-darwin-amd64 (+ .sha256)
  - devpod-provider-nomad-darwin-arm64 (+ .sha256)
  - devpod-provider-nomad-windows-amd64.exe (+ .sha256)
  - provider.yaml

  Note:

  Make sure you have Actions enabled in your fork's repository settings:
  - Go to Settings → Actions → General
  - Ensure "Allow all actions and reusable workflows" is selected

  That's it! Once you publish the release, GitHub Actions will automatically build and attach the binaries within a few minutes.

To SSH into a DevPod container, you typically use the DevPod CLI directly:

  Standard Method

  devpod ssh vscode-remote-try-node

  This is the recommended way - DevPod handles all the SSH connection details for you automatically.

  Finding Connection Details Manually

  If you need the actual IP and port for direct SSH access:

  # Get workspace status and details
  devpod status vscode-remote-try-node

  # List workspaces with details
  devpod list

  # Get SSH config that DevPod uses
  devpod ssh vscode-remote-try-node --command "echo \$SSH_CONNECTION"

  For Nomad Provider Specifically

  Since you're using the Nomad provider, the container is running as a Nomad job. You can also:

  # Find the Nomad allocation
  nomad job status devpod-vscode-remote-try-node

  # Get allocation details (this will show the actual node and ports)
  nomad alloc status <allocation-id>

  # Check port mappings
  nomad alloc status <allocation-id> | grep -A 10 "Port"

  The SSH port is typically exposed through Nomad's dynamic port allocation. DevPod's Nomad provider should handle this mapping automatically.

  Direct SSH (if needed)

  If you really need to SSH directly:
  1. The IP would be the Nomad client node's IP where the allocation is running
  2. The port would be the dynamically allocated port mapped to the container's SSH port (usually 22)

  But again, devpod ssh <workspace-name> is the intended way - it abstracts away all these details and handles authentication automatically.

  Would you like me to help you find the specific connection details for your running workspace?

 Based on the notes, I can see that Ubuntu 22.04 LTS was chosen when switching from Alpine specifically because "which is supported by DevPod's Docker
  installation script" (notes/notes.md:95).

  The choice of 22.04 over 24.04 likely came down to these factors:

  1. Maturity and stability: Ubuntu 22.04 LTS (released April 2022) is more mature and battle-tested than 24.04 LTS (released April 2024), with ~2 years more
  time to work out bugs and edge cases
  2. DevPod compatibility: DevPod's installation scripts and tooling are likely tested and verified against 22.04. The 22.04 base ensures broader compatibility
   with DevPod's agent and tooling
  3. Docker support: Docker packages and socket integration are well-established on 22.04, whereas 24.04 may have different package versions or configurations
  4. Conservative approach: When fixing critical issues, it made sense to use a known-stable base rather than introducing potential new issues with a newer OS
  version

  That said, Ubuntu 24.04 LTS would likely work fine for this use case. If you'd like to test it or switch to it, the change would be simple:

  // In cmd/create.go:16
  defaultImage = "ubuntu:24.04"

  Would you like me to update it to Ubuntu 24.04 LTS, or test whether 24.04 works properly with the current provider implementation?

Great question! No, the Ubuntu 22.04 base is NOT replaced - it serves as the "machine" that hosts the devcontainer. Here's the architecture:

  The Architecture

  Nomad Job (Ubuntu 22.04 base container)
  ├── DevPod agent runs here
  ├── Docker CLI runs here (via mounted socket from host)
  ├── curl, git, and other tools installed here
  └── Workspace directory (/tmp/devpod-workspaces)
      └── Devcontainer runs INSIDE via Docker-in-Docker

  What the Ubuntu 22.04 Base Does

  Looking at cmd/create.go:114-124, the base container:

  1. Stays running persistently (sleep infinity at cmd/create.go:61)
  2. Hosts the DevPod agent - the agent binary runs in this container
  3. Provides Docker access - mounts /var/run/docker.sock from the Nomad host (line 121)
  4. Has essential tools - curl and git are pre-installed (line 61)
  5. Provides workspace storage - /tmp/devpod-workspaces shared with host (line 122)

  What Happens When You Run a Devcontainer

  When you do devpod up github.com/microsoft/vscode-remote-try-node:

  1. DevPod agent (running in Ubuntu 22.04 base) clones the repo
  2. DevPod agent reads the .devcontainer/devcontainer.json
  3. DevPod agent uses the Docker CLI (connected via socket) to build/run the devcontainer image
  4. The devcontainer runs as a separate Docker container alongside the Ubuntu base
  5. VS Code connects to the devcontainer (not the Ubuntu base)

  Why This Design?

  This is similar to how DevPod's "docker" provider works - you need a stable "machine" container that:
  - Doesn't get torn down between workspace sessions
  - Can orchestrate Docker operations
  - Provides a consistent environment for the DevPod agent

  So Ubuntu 22.04 is the persistent host environment, and your actual development happens in the ephemeral devcontainer that runs inside it via
  Docker-in-Docker.

Great! The provider has been rebuilt successfully. Now you need to delete your existing workspace and recreate it to test the fix:

  devpod delete multistep-transcriber
  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad

  The network configuration I added should allow the container to access the Ubuntu package repositories and successfully install curl and git.

  What I changed:
  - Added a Networks configuration to the Nomad task group with bridge mode
  - This gives the container proper network access to reach external services like archive.ubuntu.com and security.ubuntu.com

  The bridge network mode is the standard way to provide internet connectivity to Docker containers in Nomad. If this doesn't work, your Nomad cluster might
  have additional network restrictions that need to be configured at the cluster level (like CNI plugins or firewall rules).

Built successfully! The fix now uses Docker's native bridge networking instead of Nomad's CNI-based networking. Now try again:

  devpod delete multistep-transcriber
  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad

  What changed:
  - Removed the Nomad TaskGroup-level Networks configuration (which requires CNI plugins)
  - Added "network_mode": "bridge" to the Docker driver config instead
  - This uses Docker's native bridge networking, which should be available on any Docker installation

  This approach avoids the CNI plugin requirement and should allow your containers to access the internet for package installation.



erfect! I've created manual-release.sh in your repository. Here's how to use it:

  Usage

  # For your existing release:
  ./manual-release.sh v0.1.0

  # For future releases:
  ./manual-release.sh v0.2.0

  The script will:
  1. Validate the version format
  2. Clean and rebuild all binaries (Linux, macOS, Windows for amd64 and arm64)
  3. Generate SHA256 checksums
  4. Create the provider.yaml file
  5. Show you instructions for uploading

  After building, upload with:

  Option 1: GitHub CLI (easiest)
  gh release upload v0.1.0 release/*

  Option 2: Manual upload
  - Go to your release page
  - Click "Edit release"
  - Drag and drop all files from the release/ directory
  - Click "Update release"

  Now you have both options:
  - Enable GitHub Actions for automated releases
  - Use manual-release.sh for manual releases whenever needed

Now I understand the setup! DevPod for the Nomad provider doesn't expose SSH via traditional host:port - instead it uses ProxyCommand to tunnel through nomad
   alloc exec.

  Here's how to connect with Emacs TRAMP:

  For Emacs TRAMP

  Use this connection string in Emacs:

  /ssh:multistep-transcriber.devpod:/path/to/file

  Or if you need to specify the user explicitly:

  /ssh:vscode@multistep-transcriber.devpod:/path/to/file

  How This Works

  Looking at your SSH config at ~/.ssh/config, DevPod created this entry:

  Host multistep-transcriber.devpod
    ProxyCommand "/home/gerald/.local/bin/devpod" ssh --stdio --context default --user vscode multistep-transcriber
    User vscode

  The ProxyCommand tells SSH to use devpod ssh --stdio as a tunnel, which in turn uses nomad alloc exec to connect to the container. TRAMP will automatically
  use this SSH config entry.

  Testing

  Try this in Emacs:

  1. Open a file: C-x C-f /ssh:multistep-transcriber.devpod:/tmp/test.txt
  2. Or open dired: C-x d /ssh:multistep-transcriber.devpod:/workspace/

  Alternative: If you need a direct connection

  If TRAMP doesn't work with ProxyCommand (some older versions have issues), you could instead:

  1. Start an SSH tunnel manually:
  ssh -N -L 2222:localhost:22 multistep-transcriber.devpod
  2. Then connect via localhost in TRAMP:
  /ssh:vscode@localhost#2222:/path/to/file

  Does the first method (/ssh:multistep-transcriber.devpod:) work for you in Emacs?


  1. Configuring Nomad Resources

  Resources are configured via provider options in hack/provider/provider.yaml:21-26. You can set:

  - NOMAD_CPU: CPU in MHz (default: "200")
  - NOMAD_MEMORYMB: Memory in MB (default: "512")

  You can configure these when setting up the provider:

  devpod provider set-options nomad \
    --option NOMAD_CPU=1000 \
    --option NOMAD_MEMORYMB=2048

  Or set them as environment variables before running devpod up:
  export NOMAD_CPU=1000
  export NOMAD_MEMORYMB=2048
  devpod up ...

  The defaults you're seeing (200 MHz CPU, 512 MiB Memory) match what's defined in pkg/options/options.go:28-29.

  2. Multiple Devcontainers

  No, each devcontainer runs in its own separate Nomad allocation.

  In cmd/create.go:101, each devcontainer gets its own unique Job ID (from the DEVCONTAINER_ID environment variable). So if you create multiple devcontainers,
  you'll see multiple Nomad jobs/allocations, each with their own resource reservations.

  For example, 3 devcontainers = 3 Nomad jobs = 3 separate allocations (each reserving their own CPU/memory/disk).


Perfect! I've implemented disk resource configuration support. Here's what I changed:

  Changes Made:

  1. hack/provider/provider.yaml:27-29 - Added NOMAD_DISKMB option with default of "300" MB
  2. hack/provider/provider-dev.yaml:27-29 - Added NOMAD_DISKMB option to dev provider
  3. pkg/options/options.go:13 - Renamed DiskSize to DiskMB
  4. pkg/options/options.go:30 - Added defaultDiskMB constant
  5. pkg/options/options.go:55 - Now reads NOMAD_DISKMB from environment
  6. cmd/create.go:93-96 - Parse disk MB from options
  7. cmd/create.go:112-114 - Set EphemeralDisk on the TaskGroup

  Now you can rebuild and test:

  # Rebuild the provider
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

  # Delete existing workspace and provider
  devpod delete 'multistep-transcriber'
  devpod provider delete nomad

  # Use with custom disk size
  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \
    --option NOMAD_CPU=2000 \
    --option NOMAD_MEMORYMB=8192 \
    --option NOMAD_DISKMB=1024

  Or set as defaults:

  devpod provider set-options nomad \
    --option NOMAD_CPU=2000 \
    --option NOMAD_MEMORYMB=8192 \
    --option NOMAD_DISKMB=1024

  Then verify with nomad alloc status - you should see your configured disk size!


Add Disk Resource Configuration Support
========================================

Added support for configuring ephemeral disk resources for Nomad workspaces.

Problem
-------
The provider previously had a DiskSize field in options but it was never used when creating Nomad jobs.
Disk was always set to Nomad's default (300 MiB) regardless of user configuration.

Solution
--------
1. Added NOMAD_DISKMB provider option (default: "300")
2. Updated Options struct to use DiskMB instead of DiskSize
3. Parse disk value and configure EphemeralDisk on TaskGroup
4. Updated README.md with resource configuration documentation

Files Changed
-------------
- hack/provider/provider.yaml - Added NOMAD_DISKMB option
- hack/provider/provider-dev.yaml - Added NOMAD_DISKMB option
- pkg/options/options.go - Changed DiskSize to DiskMB, read from NOMAD_DISKMB env var
- cmd/create.go - Parse DiskMB and set EphemeralDisk.SizeMB on TaskGroup
- README.md - Added documentation for setting resource options

Usage
-----
Set disk when creating workspace:
  devpod up <repo> --provider nomad --option NOMAD_DISKMB=1024

Or set as persistent default:
  devpod provider set-options nomad --option NOMAD_DISKMB=1024

Key Insight
-----------
In Nomad, disk is configured on the TaskGroup via EphemeralDisk.SizeMB, not in the Task's Resources.
This is different from CPU and Memory which are set on the Task's Resources.

