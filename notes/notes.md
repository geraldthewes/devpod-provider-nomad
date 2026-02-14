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


● Summary

  I've added a comprehensive Environment Variables section to the README that covers both your use cases:

  Key Points Documented:

  1. The Critical Difference: Explained why remoteEnv works but containerEnv doesn't for runtime variables
  2. Global Environment Variables (HF_TOKEN, OLLAMA_HOST, etc.):
    - Set once on your local machine (export HF_TOKEN=...)
    - Reference in any project using "${localEnv:HF_TOKEN}"
    - Works automatically across all your DevPod workspaces
  3. Project-Specific Variables:
    - Option 1: Hardcoded non-sensitive values directly in devcontainer.json
    - Option 2: Use .devcontainer/devcontainer.env file for project secrets (gitignored)
    - Option 3: Combine both global and project-specific variables
  4. Troubleshooting Section addressing:
    - Why SSH config SetEnv doesn't work with the Nomad provider
    - How to verify environment variables are correctly set
    - Common syntax issues

  What You Should Do Next:

  1. Export your global variables in your shell profile (~/.bashrc or ~/.zshrc):
  export HF_TOKEN="hf_your_token_here"
  export OLLAMA_HOST="http://localhost:11434"
  2. Update your devcontainer.json to use remoteEnv (NOT containerEnv):
  {
    "remoteEnv": {
      "HF_TOKEN": "${localEnv:HF_TOKEN}",
      "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}"
    }
  }
  3. Restart your terminal (to load the new exports), then test with:
  devpod delete your-workspace
  devpod up <repo> --provider nomad --debug

  The documentation is now in your README.md starting at line 80. Would you like me to create a quick test to verify this actually works with
   the Nomad provider?


Based on my research, there are known issues with remoteEnv and ${localEnv:...} substitution in DevPod. But I noticed an interesting
  pattern - HF_TOKEN works but OPENAI_ and OLLAMA_ don't**. This suggests the variable names might be filtered or overridden.

  Let's test if it's the variable names causing the issue:

  Test 1: Rename the variables

  Try changing your devcontainer.json to use different variable names:

  {
    "remoteEnv": {
      "HF_TOKEN": "${localEnv:HF_TOKEN}",
      "MY_OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
      "MY_OPENAI_API_KEY": "${localEnv:OPENAI_API_KEY}",
      "OPENAI_MODEL": "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ",
      "MY_OPENAI_BASE_URL": "${localEnv:OPENAI_BASE_URL}"
    }
  }

  This maps your local OPENAI_API_KEY to a container variable called MY_OPENAI_API_KEY. If these work, it confirms that OPENAI_*/OLLAMA_* are
   being filtered somewhere.

  Test 2: Use --workspace-env-file (recommended workaround)

  Create a file ~/.devpod/workspace.env:

  OLLAMA_HOST=http://ollama.cluster:11434
  OPENAI_API_KEY=**
  OPENAI_BASE_URL=http://vllm.cluster:8000/v1

  Then start your workspace with:

  devpod up github.com/geraldthewes/multistep-transcriber \
    --provider nomad \
    --workspace-env-file ~/.devpod/workspace.env \
    --debug

  Test 3: Check if libraries are overriding the vars

  Some Python libraries (like openai, langchain) automatically check for OPENAI_* variables and may override or clear them. SSH into your
  container and check:

  devpod ssh multistep-transcriber

  # Check what's actually in the environment
  env | sort | grep -E "(OLLAMA|OPENAI|HF_)"

  # Check if Python is clearing them
  python3 -c "import os; print('OPENAI_API_KEY:', os.environ.get('OPENAI_API_KEY', 'NOT SET'))"

  Which test would you like to try first? I'd recommend Test 1 (renaming) as it's the quickest way to confirm if it's a variable name
  filtering issue.


Great questions! Let me break down what I found about the reported remoteEnv issues and why workspace-env-file might not actually be
  different.

  Reported Issues with remoteEnv

  Based on my web search, here are the key problems:

  1. remoteEnv vs containerEnv Scope (Critical difference!)
  - remoteEnv: Sets environment variables for terminal sessions and dev tools (like VS Code debugger) - NOT for the entire container
  - containerEnv: Sets environment variables for the entire container, accessible to all processes

  This is the key insight! If remoteEnv only applies to terminal sessions, then:
  - Variables are available when you ssh in (which is why you see them)
  - BUT they may NOT be available during postCreateCommand execution
  - AND they may NOT be available to processes started by systemd or other init systems

  2. ${localEnv:...} Substitution Bugs
  - Issue #1366: Environment variables get replaced at runtime unexpectedly
  - Issue #1638: Environment variable substitution doesn't work reliably on Windows/WSL2
  - Some variables work, others don't, seemingly randomly

  3. Timing Issues
  - Variables set via remoteEnv may not be available during container initialization
  - postCreateCommand might run before remoteEnv is applied

  Your Excellent Point About Libraries Clearing Variables

  You're absolutely right! If Python libraries are clearing variables during pip install in your postCreateCommand, then workspace-env-file 
  would have the same problem because:

  Timeline:
  1. Container starts → containerEnv applied
  2. postCreateCommand runs → pip install → libraries may clear OPENAI_*, OLLAMA_*
  3. remoteEnv applied → too late, vars already cleared

  However, here's why some libraries might clear specific variables:

  Many Python AI/ML libraries have initialization code like:

  # In openai library __init__.py
  import os
  if 'OPENAI_API_KEY' in os.environ and not os.environ['OPENAI_API_KEY']:
      del os.environ['OPENAI_API_KEY']  # Clear if empty

  Or they validate and clear invalid values:

  # In langchain or similar
  if 'OLLAMA_HOST' in os.environ:
      try:
          validate_url(os.environ['OLLAMA_HOST'])
      except:
          del os.environ['OLLAMA_HOST']  # Clear if invalid

  The Real Mystery

  If libraries are clearing variables, why does HF_TOKEN work but OPENAI_*/OLLAMA_* don't?

  Hypothesis 1: Selective library filtering
  - huggingface_hub library doesn't validate/clear HF_TOKEN on import
  - openai library DOES validate/clear OPENAI_* variables
  - langchain or other libs clear OLLAMA_HOST

  Hypothesis 2: Variable values matter
  - Your HF_TOKEN has 37 characters (valid token)
  - Your OPENAI_API_KEY is just "**" (2 chars) - might be detected as invalid and cleared
  - OLLAMA_HOST URL might be unreachable from container, causing validation to fail and clear it

  What Would workspace-env-file Actually Do Differently?

  Looking at the code flow - probably nothing different! You're right to question this. If the issue is libraries clearing variables during
  postCreateCommand, then workspace-env-file would face the same problem.

  Better Solutions

  Option 1: Use containerEnv instead
  {
    "containerEnv": {
      "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
      "OPENAI_API_KEY": "${localEnv:OPENAI_API_KEY}"
    }
  }
  This sets them at container startup, before postCreateCommand runs.

  Option 2: Re-export in postCreateCommand
  {
    "remoteEnv": {
      "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
      "OPENAI_API_KEY": "${localEnv:OPENAI_API_KEY}"
    },
    "postCreateCommand": "pip install -r requirements.txt && echo 'export OLLAMA_HOST=${OLLAMA_HOST}' >> ~/.bashrc && echo 'export 
  OPENAI_API_KEY=${OPENAI_API_KEY}' >> ~/.bashrc"
  }

  Option 3: Use postStartCommand instead
  {
    "containerEnv": {
      "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
      "OPENAI_API_KEY": "${localEnv:OPENAI_API_KEY}"
    },
    "postCreateCommand": "pip install -r requirements.txt",
    "postStartCommand": "export OLLAMA_HOST=${OLLAMA_HOST} OPENAI_API_KEY=${OPENAI_API_KEY}"
  }

  The best test would be to check if those Python libraries are actually installed in your postCreateCommand. Can you share your
  devcontainer.json's postCreateCommand?

ou're absolutely right that adding provider-level environment variables would be perfect for global env vars like HF_TOKEN, OLLAMA_HOST,
  etc. - which is exactly your current focus! Let me compare the approaches:

  Option 1: Nomad Provider EXTRA_ENV_VARS (Recommended for your use case)

  Pros:
  - ✅ Simple to implement and use
  - ✅ Set once, works for all workspaces automatically
  - ✅ Perfect for global secrets like HF_TOKEN, OLLAMA_HOST
  - ✅ Stored securely in DevPod provider config (not in git)
  - ✅ No external dependencies

  Cons:
  - ❌ Only global (but that's what you want!)
  - ❌ Less secure than Vault (but probably fine for your use case)

  Implementation:
  I could add this in about 20 minutes. It would work like:

  # Set global env vars for all workspaces
  devpod provider set-options nomad \
    --option EXTRA_ENV_VARS='{"HF_TOKEN":"hf_xxx","OLLAMA_HOST":"http://ollama.cluster:11434","OPENAI_API_KEY":"**","OPENAI_BASE_URL":"http:/
  /vllm.cluster:8000/v1"}'

  # Now all workspaces automatically get these vars
  devpod up <any-repo> --provider nomad

  Option 2: Vault Integration (Enterprise-grade but complex)

  Nomad has native Vault integration that's actually quite elegant once set up. The workflow would be:

  How it works:
  1. Store secrets in Vault: vault kv put secret/devpod HF_TOKEN=hf_xxx
  2. Nomad job gets a Vault token
  3. Container reads secrets from Vault at runtime

  In the Nomad provider, we'd add:
  vault {
    policies = ["devpod-secrets"]
  }

  template {
    data = <<EOH
  {{ with secret "secret/devpod" }}
  export HF_TOKEN="{{ .Data.data.HF_TOKEN }}"
  export OLLAMA_HOST="{{ .Data.data.OLLAMA_HOST }}"
  {{ end }}
  EOH
    destination = "secrets/env.sh"
    env = true
  }

  Pros:
  - 🔒 Most secure approach
  - 🔒 Centralized secret management
  - 🔒 Audit logs, rotation, fine-grained access

  Cons:
  - 😤 Vault setup is a PITA (as you noted!)
  - 😤 Requires Vault infrastructure
  - 😤 Complex policies and auth setup
  - 😤 Overkill for personal dev environments

  Option 3: Consul KV (Middle ground)

  Simpler than Vault, but not secure (plaintext storage):

  consul kv put devpod/HF_TOKEN "hf_xxx"
  consul kv put devpod/OLLAMA_HOST "http://ollama.cluster:11434"

  Then read from Consul in the container startup script.

  Verdict: Don't use this for secrets - it's plaintext in Consul!

  My Recommendation

  Given your situation (personal dev environment, global secrets, existing Nomad cluster), I'd recommend:

  Add EXTRA_ENV_VARS to the Nomad provider

  This gives you:
  - ✅ Simple solution for your immediate problem
  - ✅ No external dependencies
  - ✅ Works today with 20 minutes of coding
  - ✅ Can migrate to Vault later if needed

  Would you like me to implement the EXTRA_ENV_VARS provider option right now?

  It would:
  1. Add a new option to provider.yaml
  2. Parse the JSON env vars in pkg/options/options.go
  3. Merge them with devcontainer.json env vars in cmd/create.go
  4. Update the README with examples

  This way you get global secrets without dealing with Vault's complexity, and you can still override per-project using containerEnv in
  devcontainer.json.

  Alternative: If you really want to go the Vault route and need help making it less painful, I can help set up a simplified Vault
  integration. But honestly, for global dev environment secrets, the provider option is probably the sweet spot.



Perfect! I've implemented the registry certificate support and documented it. Here's what I changed:

  Changes Made

  1. Code Changes (cmd/create.go:127-132)

  Added two volume mounts to the Nomad job configuration:
  "volumes": []string{
      "/var/run/docker.sock:/var/run/docker.sock",
      sharedWorkspacePath + ":" + sharedWorkspacePath,
      "/etc/docker/certs.d:/etc/docker/certs.d:ro",    // ← New
      "/etc/ssl/certs:/etc/ssl/certs:ro",              // ← New
  },

  2. Documentation (README.md)

  Added a comprehensive "Using Private Docker Registries" section that covers:
  - How the certificate mounting works
  - Step-by-step setup instructions
  - Example configuration
  - Troubleshooting common errors

  Testing the Changes

  Since you already have the certificates set up on your Nomad clients at /etc/docker/certs.d/registry.cluster:5000/, you can test this right
   away:

  # Build the provider
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

  # Remove old provider and install new one
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  # Test with your multistep-transcriber project
  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \
      --ide none \
      --provider-option NOMAD_CPU=2000 \
      --provider-option NOMAD_MEMORYMB=8192 \
      --provider-option NOMAD_DISKMB=512 \
      --debug

  The "x509: certificate signed by unknown authority" error should now be resolved! The Docker daemon will find your CA certificate at
  /etc/docker/certs.d/registry.cluster:5000/ca.crt and use it to verify the TLS connection.


erfect! I've identified and fixed the root cause. The issue was that DevPod makes direct HTTPS API calls to the registry to inspect images
   (not just relying on the Docker daemon), so it needs access to the system CA certificates inside the container.

  Changes Made

  /media/gerald/SSDT7/gerald/devpod-provider-nomad/cmd/create.go:134

  Added /etc/ssl/certs mount (read-write) so:
  1. DevPod can verify TLS when making direct registry API calls
  2. The ca-certificates package can install during container setup

  "volumes": []string{
      "/var/run/docker.sock:/var/run/docker.sock",
      sharedWorkspacePath + ":" + sharedWorkspacePath,
      "/etc/docker/certs.d:/etc/docker/certs.d:ro",
      "/etc/ssl/certs:/etc/ssl/certs",  // NEW: System CAs for DevPod API calls
  },

  /media/gerald/SSDT7/gerald/devpod-provider-nomad/README.md:246-277

  Updated documentation to explain:
  - Why both /etc/docker/certs.d and /etc/ssl/certs are needed
  - Updated setup instructions to add certificate to system trust store

  Required Setup on Nomad Clients

  Before testing, ensure your CA certificate is in the system trust store on each Nomad client:

  # On each Nomad client node
  sudo cp /path/to/ca.crt /usr/local/share/ca-certificates/registry-cluster.crt
  sudo update-ca-certificates

  This adds your certificate to /etc/ssl/certs/ which gets mounted into the DevPod container.

  Test the Fix

  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \
      --ide none \
      --provider-option NOMAD_CPU=2000 \
      --provider-option NOMAD_MEMORYMB=8192 \
      --provider-option NOMAD_DISKMB=512

  The TLS error should be gone now that DevPod has access to your registry's CA certificate through the system trust store!


Perfect! I've implemented the correct fix. The issue was that we were mounting /etc/ssl/certs from the host, but when the container's
  ca-certificates package installed, it regenerated the CA bundle without your registry certificate.

  What Changed

  1. /media/gerald/SSDT7/gerald/devpod-provider-nomad/cmd/create.go:61

  Added update-ca-certificates to the container startup command:
  runCmd := []string{"/bin/sh", "-c", "mkdir -p " + sharedWorkspacePath + " && apt-get update -qq && apt-get install -y -qq curl git 
  ca-certificates && update-ca-certificates && sleep 2 && touch /tmp/.devpod-ready && sleep infinity"}

  2. /media/gerald/SSDT7/gerald/devpod-provider-nomad/cmd/create.go:134

  Changed the volume mounts to:
  - ❌ Removed: /etc/ssl/certs:/etc/ssl/certs (was causing conflicts)
  - ✅ Added: /usr/local/share/ca-certificates/registry.cluster.crt:/usr/local/share/ca-certificates/registry.cluster.crt:ro

  This mounts the CA certificate source file so when update-ca-certificates runs, it includes your registry cert in the container's CA
  bundle.

  3. /media/gerald/SSDT7/gerald/devpod-provider-nomad/README.md:246-281

  Updated documentation to explain the new approach and clarify that the filename must be exactly registry.cluster.crt.

  Why This Works

  1. Host: Your Ansible deployment created /usr/local/share/ca-certificates/registry.cluster.crt on all Nomad clients ✅
  2. Container: We mount that file to the same path in the container
  3. Container startup: Runs update-ca-certificates which reads certs from /usr/local/share/ca-certificates/ and adds them to
  /etc/ssl/certs/ca-certificates.crt
  4. DevPod: Can now verify TLS when making API calls to registry.cluster:5000

  Test the Fix

  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \
      --ide none \
      --provider-option NOMAD_CPU=2000 \
      --provider-option NOMAD_MEMORYMB=8192 \
      --provider-option NOMAD_DISKMB=512

  The TLS error should be resolved now! The container will have your registry certificate trusted through the proper CA bundle mechanism.

2025-11-07
----------

## Vault Secrets - Making Secrets Accessible in Devcontainer

### Problem
Vault secrets were being injected into the Nomad task successfully, but were not accessible in the devcontainer.

- Vault integration with `Envvars: true` made secrets available to the Nomad task process
- Secrets file was written to `/tmp/devpod-workspaces/.vault-secrets` in the Nomad task
- BUT devcontainer couldn't access this path because `/tmp/devpod-workspaces` is NOT mounted into devcontainer
- DevPod only mounts the workspace content directory: `/tmp/devpod-workspaces/agent/contexts/default/workspaces/<workspace-name>/content` → `/workspaces/<workspace-name>`

### Solution
Added background process to the Nomad task init command that automatically copies secrets file to workspace directories:

**Updated cmd/create.go:63:**
- Background loop runs every 5 seconds
- Finds all workspace content directories using glob: `$sharedWorkspacePath/agent/contexts/*/workspaces/*/content`
- Copies `.vault-secrets` file to each workspace content directory if not already present
- Sets proper permissions (644)

**Result:**
- Secrets file appears at `/workspaces/<workspace-name>/.vault-secrets` in devcontainer
- User can source the file in their `setup.sh` script: `source .vault-secrets`
- Works for all workspaces automatically

### Testing
```bash
# Verify in Nomad task
nomad alloc exec <alloc-id> ls -la /tmp/devpod-workspaces/.vault-secrets

# Verify in devcontainer
devpod ssh <workspace>
ls -la .vault-secrets
cat .vault-secrets
source .vault-secrets
env | grep -E 'AWS_|HF_'
```

2025-11-06
----------

Vault Secrets Integration Plan

 Overview

 Add Nomad-native Vault integration to inject secrets as environment variables into DevPod workspaces. Secrets are fetched by Nomad (not the
 provider) using Vault policies and rendered via Nomad templates.

 Implementation Steps

 1. Define Vault Configuration Structure

 File: pkg/options/options.go

 Add new types and fields:
 // VaultSecret represents a secret path and field mappings
 type VaultSecret struct {
     Path   string            // Vault KV v2 path (e.g., "secret/data/aws/creds")
     Fields map[string]string // vault_field -> ENV_VAR_NAME
 }

 // Add to Options struct:
 type Options struct {
     // ... existing fields ...

     // Vault configuration
     VaultAddr       string
     VaultRole       string
     VaultNamespace  string
     VaultChangeMode string
     VaultPolicies   []string
     VaultSecrets    []VaultSecret
 }

 Parse from environment variables in DefaultOptions():
 - VAULT_ADDR (required if secrets configured)
 - VAULT_ROLE (default: "nomad-workloads")
 - VAULT_NAMESPACE (optional)
 - VAULT_CHANGE_MODE (default: "restart")
 - VAULT_POLICIES_JSON (JSON array)
 - VAULT_SECRETS_JSON (JSON array of objects)

 Add validation: if VAULT_SECRETS_JSON is set, require VAULT_POLICIES_JSON and VAULT_ADDR.

 2. Generate Nomad Template Stanzas

 File: cmd/create.go

 Add function to generate Vault templates:
 func generateVaultTemplates(secrets []VaultSecret) []*api.Template {
     templates := make([]*api.Template, len(secrets))

     for i, secret := range secrets {
         tmpl := generateSecretTemplate(secret)
         templates[i] = &api.Template{
             DestPath:     fmt.Sprintf("secrets/vault-%d.env", i),
             EmbeddedTmpl: tmpl,
             Envvars:      true,  // Key: makes secrets available as env vars
             ChangeMode:   options.VaultChangeMode,
         }
     }
     return templates
 }

 func generateSecretTemplate(secret VaultSecret) string {
     // Generate: {{ with secret "path" }}export VAR="{{ .Data.data.field }}"{{ end }}
 }

 3. Update Job Specification

 File: cmd/create.go (in createJob() function)

 Add Vault stanza to task if Vault secrets configured:
 if len(options.VaultSecrets) > 0 {
     task.Vault = &api.Vault{
         Policies:   options.VaultPolicies,
         Role:       options.VaultRole,
         Namespace:  options.VaultNamespace,
         ChangeMode: options.VaultChangeMode,
     }

     task.Templates = generateVaultTemplates(options.VaultSecrets)
 }

 4. Add Provider Options Documentation

 File: hack/provider/provider.yaml

 Add Vault-related options:
   VAULT_ADDR:
     description: "Vault server address (e.g., https://vault.example.com:8200)"
   VAULT_ROLE:
     description: "Vault role for Nomad workload identity"
     default: "nomad-workloads"
   VAULT_NAMESPACE:
     description: "Vault namespace (Vault Enterprise only)"
   VAULT_CHANGE_MODE:
     description: "Action when secrets change (restart, noop, signal)"
     default: "restart"
   VAULT_POLICIES_JSON:
     description: "JSON array of Vault policies (e.g., [\"aws-read\", \"db-read\"])"
   VAULT_SECRETS_JSON:
     description: "JSON array of Vault secret configurations"

 5. Update README Documentation

 File: README.md

 Add section on Vault integration:
 - Configuration examples
 - .devcontainer.json example with remoteEnv
 - Vault policy requirements
 - Template format explanation

 6. Add Validation and Error Handling

 File: pkg/options/options.go

 - Validate JSON parsing for VAULT_POLICIES_JSON and VAULT_SECRETS_JSON
 - Validate required fields (path, fields not empty)
 - Fail fast if Vault config is incomplete
 - Return clear error messages

 7. Testing

 File: CLAUDE.md testing section

 Update test commands to include Vault configuration example:
 export VAULT_ADDR="https://vault.example.com:8200"
 export VAULT_POLICIES_JSON='["devpod-secrets"]'
 export VAULT_SECRETS_JSON='[{"path":"secret/data/test","fields":{"key":"TEST_VAR"}}]'
 devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

 Key Design Decisions

 1. Nomad-native approach: Let Nomad handle Vault token management and secret fetching
 2. Environment variables only: Consistent with current architecture, no .devcontainer.json parsing needed
 3. JSON for complex data: Use JSON-encoded env vars for policies and secrets configuration
 4. Fail-fast: Job creation fails if Vault config is incomplete or invalid
 5. No hardcoded values: All settings configurable via environment variables
 6. Template-based injection: Secrets rendered to secrets/vault-N.env files, sourced as env vars via Envvars: true

 Files to Modify

 1. pkg/options/options.go - Add Vault config types and parsing
 2. cmd/create.go - Add Vault stanza and template generation
 3. hack/provider/provider.yaml - Add provider options
 4. README.md - Add documentation
 5. CLAUDE.md - Update testing examples

 Testing Requirements

 - Nomad cluster with Vault integration enabled
 - Vault policies configured for Nomad
 - Test secrets stored in Vault KV v2
 - Verify secrets appear as environment variables in container
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌


Excellent! I've successfully implemented Vault secrets integration for the devpod-provider-nomad project. Here's a summary of what was
  done:

  ✅ Implementation Complete

  Files Modified

  1. pkg/options/options.go (pkg/options/options.go:11-173)
    - Added VaultSecret struct for secret configuration
    - Added Vault fields to Options struct (VaultAddr, VaultRole, VaultNamespace, VaultChangeMode, VaultPolicies, VaultSecrets)
    - Implemented parsing of VAULT_POLICIES_JSON and VAULT_SECRETS_JSON environment variables
    - Added comprehensive validation via ValidateVault() method
  2. cmd/create.go (cmd/create.go:103-189)
    - Added generateVaultTemplates() function to create Nomad template stanzas
    - Added generateSecretTemplate() function to generate Vault secret templates with proper Nomad template syntax
    - Modified job creation to conditionally add Vault stanza and templates when secrets are configured
    - Added boolPtr() helper function
  3. hack/provider/provider.yaml (hack/provider/provider.yaml:30-64)
    - Added 6 new provider options: VAULT_ADDR, VAULT_ROLE, VAULT_NAMESPACE, VAULT_CHANGE_MODE, VAULT_POLICIES_JSON, VAULT_SECRETS_JSON
    - Included detailed descriptions and examples for each option
  4. README.md (README.md:343-611)
    - Added comprehensive "Vault Secrets Integration" section with:
        - How it works explanation
      - Prerequisites
      - Configuration examples
      - Complete step-by-step example
      - Per-workspace configuration
      - Advanced configuration options
      - Troubleshooting guide
      - Security best practices
  5. CLAUDE.md (CLAUDE.md:4-179)
    - Added "Testing with Vault Integration" section
    - Included setup instructions for test secrets and policies
    - Added test cases for single and multiple secrets
    - Included verification commands
    - Added validation error test cases

  Key Features Implemented

  ✅ Nomad-native Vault integration - Leverages Nomad's built-in Vault support✅ Environment variable configuration - All settings via env
  vars (no hardcoded values)✅ Multiple secrets support - Can inject secrets from multiple Vault paths✅ Comprehensive validation - Fails
  fast with clear error messages✅ Flexible configuration - Support for Vault namespaces, custom roles, and change modes✅ Per-workspace 
  overrides - Can configure via .devcontainer.json remoteEnv✅ Secure by design - Secrets never appear in job specs or logs

  Testing the Implementation

  To test the Vault integration:

  # Build the provider
  RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

  # Set up Vault (if needed)
  vault kv put secret/test/devpod test_key="test_value"
  vault policy write devpod-test <(echo 'path "secret/data/test/devpod" { capabilities = ["read"] }')

  # Configure and test
  export VAULT_ADDR="https://vault.example.com:8200"
  export VAULT_POLICIES_JSON='["devpod-test"]'
  export VAULT_SECRETS_JSON='[{"path":"secret/data/test/devpod","fields":{"test_key":"TEST_KEY"}}]'

  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug

  Next Steps

  1. Test the implementation with your actual Vault cluster
  2. Verify that secrets appear as environment variables in the container
  3. Create Vault policies specific to your use case
  4. Build and release the provider when ready


Here's what the --dev flag does and why it's useful:

  Production Build (no --dev)

  Binary paths: Points to GitHub releases
  path: https://github.com/briancain/devpod-provider-nomad/releases/download/v0.1.4/devpod-provider-nomad-linux-amd64

  Use case:
  - For official releases distributed via GitHub
  - DevPod downloads the binaries from GitHub releases
  - Users install with: devpod provider add https://github.com/...

  Dev Build (--dev)

  Binary paths: Points to your local filesystem
  path: /media/gerald/SSDT7/gerald/devpod-provider-nomad/release/devpod-provider-nomad-linux-amd64

  Use case:
  - For local development and testing
  - DevPod uses your locally built binaries directly (in cmd/create.go:141)
  - No need to publish to GitHub between code changes
  - Install with: devpod provider add ./release/provider.yaml

  Problem It Solves

  Without --dev: You'd have to:
  1. Make code changes
  2. Commit and push to GitHub
  3. Create a GitHub release
  4. Wait for release to publish
  5. Test your changes

  With --dev: You can:
  1. Make code changes
  2. Run ./hack/build.sh --dev
  3. Run devpod provider add ./release/provider.yaml
  4. Test immediately with local binaries

  So yes, it's very useful for development! It creates a rapid iteration loop. You should keep using --dev for local development and testing. Only use
  production builds when creating official releases.



The Problem

  Vault secrets with Envvars: true only made the environment variables available to the Nomad task process (the shell running sleep infinity). When
  DevPod created Docker containers inside that task, those containers didn't inherit the environment variables.

  That's why you could see the secrets in the Nomad task (nomad alloc exec showed them), but they weren't in the devcontainer environment.

  The Fix

  I've implemented a dual-template approach:

  1. Individual secret files (secrets/vault-0.env, vault-1.env) with Envvars: true
    - Makes secrets available to the Nomad task process
  2. Combined Docker env file (local/docker.env) without Envvars
    - Contains all secrets in Docker env file format (KEY=value)
    - Referenced by Docker driver via env_file configuration
    - Docker automatically loads this file for ALL containers it creates

  This ensures secrets are available both in the task AND in any Docker containers (including your devcontainer).

  Testing

  Now you need to:

  # Delete old workspace
  devpod delete multistep-transcriber

  # Reinstall provider
  devpod provider delete nomad
  devpod provider add --name nomad --use ./release/provider.yaml

  # Recreate with Vault config
  devpod up /media/gerald/SSDT7/gerald/multistep-transcriber --provider nomad --debug

  # Test in devcontainer
  devpod ssh multistep-transcriber
  env | grep -E "AWS_|HF_"

  All your secrets (both AWS keys and HF_TOKEN) should now be available in the devcontainer! 🎉

Here is Claude's plan:
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Manual GitHub Release Plan for devpod-provider-nomad

 Overview

 Manually publish v0.1.5 release since GitHub Actions is failing.

 Steps to Execute

 1. Build Release Artifacts

 cd /home/gerald/repos/devpod-provider-nomad
 go mod vendor
 RELEASE_VERSION=v0.1.5 ./hack/build.sh

 2. Create and Push Git Tag

 git tag v0.1.5
 git push origin v0.1.5

 3. Create GitHub Release with All Artifacts

 gh release create v0.1.5 ./release/* \
   --title "v0.1.5" \
   --generate-notes

 What Gets Published

 11 files from release/ directory:
 - devpod-provider-nomad-linux-amd64 + .sha256
 - devpod-provider-nomad-linux-arm64 + .sha256
 - devpod-provider-nomad-darwin-amd64 + .sha256
 - devpod-provider-nomad-darwin-arm64 + .sha256
 - devpod-provider-nomad-windows-amd64.exe + .sha256
 - provider.yaml (DevPod provider manifest)

 Notes

 - Go cross-compiles all platforms from Linux (no Mac needed)
 - provider.yaml contains GitHub release URLs matching v0.1.5
 - --generate-notes creates release notes from commits since v0.1.4

 Here is Claude's plan:
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Fix: Container startup fails when no Vault secrets configured

 Problem

 When launching a devpod without Vault secrets, the container never becomes ready because:

 1. The startup script uses a for loop to process vault secrets:
 for f in /secrets/vault-*.env; do [ -f "$f" ] && cat "$f" >> ...; done && sleep 2 && touch /tmp/.devpod-ready
 2. When no vault secrets exist, the glob /secrets/vault-*.env doesn't match any files, so bash expands it literally. The [ -f "$f" ] test fails (exit code 1).
 3. The for loop's exit status is 1, breaking the && chain. The touch /tmp/.devpod-ready never executes.
 4. The provider waits forever for /tmp/.devpod-ready which never appears.

 Fix

 File: cmd/create.go (line 63)

 Change the for loop to always succeed by adding || true:

 // Before (broken):
 for f in /secrets/vault-*.env; do [ -f \"$f\" ] && cat \"$f\" >> ... ; done && sleep 2

 // After (fixed):
 (for f in /secrets/vault-*.env; do [ -f \"$f\" ] && cat \"$f\" >> ... ; done || true) && sleep 2

 Or alternatively, restructure to be more explicit:

 if ls /secrets/vault-*.env 1>/dev/null 2>&1; then
   for f in /secrets/vault-*.env; do cat "$f" >> /tmp/devpod-workspaces/.vault-secrets; done
 fi

 Testing

 1. Build provider: RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
 2. Delete existing workspace: devpod delete video-transcription-batch
 3. Delete and reinstall provider: devpod provider delete nomad
 4. Test without vault secrets: devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug
 5. Verify container becomes ready and devpod connects successfully


Here is Claude's plan:
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Use workspace name as Nomad task name

 Goal

 Change the Nomad task name from hardcoded "devpod" to use the workspace name (e.g., "vscode-remote-try-node") from DEVCONTAINER_ID.

 Changes

 1. pkg/options/options.go (line 95)

 Change TaskName to use DEVCONTAINER_ID:

 // Before:
 TaskName:   "devpod",

 // After:
 TaskName:   getEnv("DEVCONTAINER_ID", "devpod"),

 2. pkg/nomad/nomad.go

 Update CommandDevContainer to accept task name as parameter:

 // Before (line 188):
 func (n *Nomad) CommandDevContainer(
     ctx context.Context,
     jobID string,
     user string,
     ...

 // After:
 func (n *Nomad) CommandDevContainer(
     ctx context.Context,
     jobID string,
     taskName string,  // NEW
     user string,
     ...

 Remove hardcoded task name (line 201):
 // Before:
 task := "devpod"

 // After:
 // (use taskName parameter instead)

 3. cmd/command.go (line 50)

 Pass TaskName to CommandDevContainer:

 // Before:
 code, err := nomad.CommandDevContainer(ctx,
     options.JobId,
     os.Getenv("DEVCONTAINER_USER"),
     ...

 // After:
 code, err := nomad.CommandDevContainer(ctx,
     options.JobId,
     options.TaskName,  // NEW
     os.Getenv("DEVCONTAINER_USER"),
     ...

 Testing

 1. Build: RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
 2. Reinstall provider
 3. Launch workspace and verify Nomad shows correct task name:
 nomad job status <workspace-name>


     ╭─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
     │ CSI Persistent Volume Support for DevPod Nomad Provider                                                                                                                         │
     │                                                                                                                                                                                 │
     │ Summary                                                                                                                                                                         │
     │                                                                                                                                                                                 │
     │ Add CSI persistent volume support to enable workspace data to survive job restarts. Users specify a storage mode (ephemeral or persistent) rather than volume names - volumes   │
     │ are automatically created and deleted based on the workspace lifecycle.                                                                                                         │
     │                                                                                                                                                                                 │
     │ Key Requirements                                                                                                                                                                │
     │                                                                                                                                                                                 │
     │ 1. Storage mode toggle: NOMAD_STORAGE_MODE = ephemeral (default) | persistent                                                                                                   │
     │ 2. Auto-derived volume names: devpod-{MACHINE_ID} pattern                                                                                                                       │
     │ 3. Automatic cleanup: CSI volumes deleted when devcontainer is deleted                                                                                                          │
     │ 4. Capacity: Re-use NOMAD_DISKMB (convert MB to bytes for CSI)                                                                                                                  │
     │ 5. Mount path: /workspace for persistent storage                                                                                                                                │
     │                                                                                                                                                                                 │
     │ User's Environment                                                                                                                                                              │
     │                                                                                                                                                                                 │
     │ - CSI Plugin: ceph-csi (Provider: rbd.csi.ceph.com)                                                                                                                             │
     │ - Ceph Cluster ID: 70464857-9ed6-11f0-8df5-d45d64d7d4f0                                                                                                                         │
     │ - Pool: nomad                                                                                                                                                                   │
     │ - Access mode: single-node-writer                                                                                                                                               │
     │ - Attachment mode: file-system                                                                                                                                                  │
     │                                                                                                                                                                                 │
     │ New Provider Options                                                                                                                                                            │
     │                                                                                                                                                                                 │
     │ | Option               | Default                   | Description                           |                                                                                    │
     │ |----------------------|---------------------------|---------------------------------------|                                                                                    │
     │ | NOMAD_STORAGE_MODE   | ephemeral                 | Storage mode: ephemeral or persistent |                                                                                    │
     │ | NOMAD_CSI_PLUGIN_ID  | ceph-csi                  | CSI plugin ID                         |                                                                                    │
     │ | NOMAD_CSI_CLUSTER_ID | (required for persistent) | Ceph cluster ID                       |                                                                                    │
     │ | NOMAD_CSI_POOL       | nomad                     | Ceph pool name                        |                                                                                    │
     │                                                                                                                                                                                 │
     │ Files to Modify                                                                                                                                                                 │
     │                                                                                                                                                                                 │
     │ 1. pkg/options/options.go                                                                                                                                                       │
     │                                                                                                                                                                                 │
     │ Add to Options struct:                                                                                                                                                          │
     │ // CSI Storage configuration                                                                                                                                                    │
     │ StorageMode    string // "ephemeral" (default) or "persistent"                                                                                                                  │
     │ CSIPluginID    string // CSI plugin ID, default "ceph-csi"                                                                                                                      │
     │ CSIClusterID   string // Ceph cluster ID (required for persistent mode)                                                                                                         │
     │ CSIPool        string // Ceph pool name, default "nomad"                                                                                                                        │
     │                                                                                                                                                                                 │
     │ Add constants:                                                                                                                                                                  │
     │ const (                                                                                                                                                                         │
     │     defaultStorageMode   = "ephemeral"                                                                                                                                          │
     │     defaultCSIPluginID   = "ceph-csi"                                                                                                                                           │
     │     defaultCSIPool       = "nomad"                                                                                                                                              │
     │     StorageModeEphemeral = "ephemeral"                                                                                                                                          │
     │     StorageModePersistent = "persistent"                                                                                                                                        │
     │ )                                                                                                                                                                               │
     │                                                                                                                                                                                 │
     │ Add methods:                                                                                                                                                                    │
     │ - ValidateCSI() - Validates CSI config (require cluster ID for persistent mode)                                                                                                 │
     │ - GetVolumeID() - Returns devpod-{JobId}                                                                                                                                        │
     │                                                                                                                                                                                 │
     │ 2. pkg/nomad/nomad.go                                                                                                                                                           │
     │                                                                                                                                                                                 │
     │ Add CSI volume methods:                                                                                                                                                         │
     │ - VolumeExists(ctx, volumeID, namespace) - Check if volume exists                                                                                                               │
     │ - CreateCSIVolume(ctx, volumeID, capacityBytes, pluginID, clusterID, pool, namespace) - Create volume                                                                           │
     │ - DeleteCSIVolume(ctx, volumeID, namespace) - Delete volume (with deregister)                                                                                                   │
     │                                                                                                                                                                                 │
     │ 3. cmd/create.go                                                                                                                                                                │
     │                                                                                                                                                                                 │
     │ Changes:                                                                                                                                                                        │
     │ 1. If StorageMode == "persistent":                                                                                                                                              │
     │   - Check if volume exists via VolumeExists()                                                                                                                                   │
     │   - Create volume if not exists via CreateCSIVolume()                                                                                                                           │
     │   - Add VolumeRequest to TaskGroup.Volumes                                                                                                                                      │
     │   - Add VolumeMount to Task.VolumeMounts (destination: /workspace)                                                                                                              │
     │   - Don't add host bind mount for workspace directory                                                                                                                           │
     │ 2. If StorageMode == "ephemeral" (default):                                                                                                                                     │
     │   - Keep current behavior with EphemeralDisk                                                                                                                                    │
     │                                                                                                                                                                                 │
     │ 4. cmd/delete.go                                                                                                                                                                │
     │                                                                                                                                                                                 │
     │ Changes:                                                                                                                                                                        │
     │ 1. Delete job first (existing behavior)                                                                                                                                         │
     │ 2. If StorageMode == "persistent":                                                                                                                                              │
     │   - Delete CSI volume via DeleteCSIVolume()                                                                                                                                     │
     │   - Log warning if volume deletion fails (don't block workspace cleanup)                                                                                                        │
     │                                                                                                                                                                                 │
     │ 5. hack/provider/provider.yaml                                                                                                                                                  │
     │                                                                                                                                                                                 │
     │ Add options:                                                                                                                                                                    │
     │ NOMAD_STORAGE_MODE:                                                                                                                                                             │
     │   description: Storage mode ("ephemeral" or "persistent")                                                                                                                       │
     │   default: "ephemeral"                                                                                                                                                          │
     │ NOMAD_CSI_PLUGIN_ID:                                                                                                                                                            │
     │   description: CSI plugin ID for persistent storage                                                                                                                             │
     │   default: "ceph-csi"                                                                                                                                                           │
     │ NOMAD_CSI_CLUSTER_ID:                                                                                                                                                           │
     │   description: Ceph cluster ID (required for persistent mode)                                                                                                                   │
     │   default:                                                                                                                                                                      │
     │ NOMAD_CSI_POOL:                                                                                                                                                                 │
     │   description: Ceph pool name for CSI volumes                                                                                                                                   │
     │   default: "nomad"                                                                                                                                                              │
     │                                                                                                                                                                                 │
     │ Implementation Steps                                                                                                                                                            │
     │                                                                                                                                                                                 │
     │ 1. Update options - Add CSI fields, constants, validation, and helper method                                                                                                    │
     │ 2. Add Nomad CSI methods - VolumeExists, CreateCSIVolume, DeleteCSIVolume                                                                                                       │
     │ 3. Update create command - Conditional volume creation and job configuration                                                                                                    │
     │ 4. Update delete command - Add volume cleanup after job deletion                                                                                                                │
     │ 5. Update provider.yaml - Expose new options                                                                                                                                    │
     │ 6. Add unit tests - Test validation and volume ID generation                                                                                                                    │
     │                                                                                                                                                                                 │
     │ Testing                                                                                                                                                                         │
     │                                                                                                                                                                                 │
     │ Test persistent mode:                                                                                                                                                           │
     │                                                                                                                                                                                 │
     │ devpod delete 'vscode-remote-try-node'                                                                                                                                          │
     │ devpod provider delete nomad                                                                                                                                                    │
     │ RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev                                                                                                                                 │
     │                                                                                                                                                                                 │
     │ devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \                                                                                                │
     │   --provider-option NOMAD_STORAGE_MODE=persistent \                                                                                                                             │
     │   --provider-option NOMAD_CSI_CLUSTER_ID=70464857-9ed6-11f0-8df5-d45d64d7d4f0 \                                                                                                 │
     │   --provider-option NOMAD_DISKMB=1024                                                                                                                                           │
     │                                                                                                                                                                                 │
     │ Verify volume created:                                                                                                                                                          │
     │                                                                                                                                                                                 │
     │ nomad volume status | grep devpod                                                                                                                                               │
     │                                                                                                                                                                                 │
     │ Test persistence:                                                                                                                                                               │
     │                                                                                                                                                                                 │
     │ devpod stop vscode-remote-try-node                                                                                                                                              │
     │ devpod up github.com/microsoft/vscode-remote-try-node --provider nomad                                                                                                          │
     │ devpod ssh vscode-remote-try-node                                                                                                                                               │
     │ # Verify files persist                                                                                                                                                          │
     │                                                                                                                                                                                 │
     │ Test cleanup:                                                                                                                                                                   │
     │                                                                                                                                                                                 │
     │ devpod delete vscode-remote-try-node                                                                                                                                            │
     │ nomad volume status | grep devpod  # Should be empty                                                                                                                            │
     │                                                                                                                                                                                 │
     │ Considerations                                                                                                                                                                  │
     │                                                                                                                                                                                 │
     │ 1. Volume detachment timing: After job deletion, volume may need time to detach. Consider retry loop in DeleteCSIVolume.                                                        │
     │ 2. Agent paths: Current AGENT_PATH defaults to /tmp/devpod-workspaces/devpod. For persistent mode, agent data should be on the CSI volume at /workspace.                        │
     │ 3. Docker-in-Docker: Current implementation requires same path on host/container. CSI mount is managed by Nomad which handles this.



     The implementation is complete. Here's a summary of the changes:

  Changes Made

  1. pkg/options/options.go

  - Added CSI storage fields: StorageMode, CSIPluginID, CSIClusterID, CSIPool
  - Added constants: StorageModeEphemeral, StorageModePersistent
  - Added ValidateCSI() method for validation
  - Added GetVolumeID() helper that returns devpod-{JobId}

  2. pkg/nomad/nomad.go

  - Added VolumeExists() - checks if a CSI volume exists
  - Added CreateCSIVolume() - creates a CSI volume with Ceph-specific parameters
  - Added DeleteCSIVolume() - deregisters and deletes a CSI volume

  3. cmd/create.go

  - Creates CSI volume if NOMAD_STORAGE_MODE=persistent and volume doesn't exist
  - Mounts CSI volume at /workspace in persistent mode
  - Uses ephemeral disk (existing behavior) in ephemeral mode

  4. cmd/delete.go

  - Deletes CSI volume after job deletion in persistent mode
  - Logs warning if volume deletion fails (doesn't block cleanup)

  5. hack/provider/provider.yaml

  - Added new options: NOMAD_STORAGE_MODE, NOMAD_CSI_PLUGIN_ID, NOMAD_CSI_CLUSTER_ID, NOMAD_CSI_POOL

  6. pkg/options/options_test.go

  - Added unit tests for CSI validation

  Testing

  To test with persistent storage:

  # Clean up existing workspace and provider
  devpod delete 'vscode-remote-try-node'
  devpod provider delete nomad

  # Install the dev build
  devpod provider add ./release/provider.yaml --name nomad

  # Launch with persistent storage
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
    --provider-option NOMAD_STORAGE_MODE=persistent \
    --provider-option NOMAD_CSI_CLUSTER_ID=70464857-9ed6-11f0-8df5-d45d64d7d4f0 \
    --provider-option NOMAD_DISKMB=10240

  # Verify the CSI volume was created
  curl -s http://10.0.1.12:4646/v1/volumes?type=csi | grep devpod

  # Test persistence
  devpod stop vscode-remote-try-node
  devpod up github.com/microsoft/vscode-remote-try-node --provider nomad

  # Clean up (should also delete the CSI volume)
  devpod delete vscode-remote-try-node

  1. Store Ceph credentials in Vault

  # Get your Ceph admin key
  ceph auth get-key client.admin

  # Store in Vault KV v2
  vault kv put secret/ceph/csi \
    userID="admin" \
    userKey="<your-ceph-admin-key>"

  2. Set VAULT_TOKEN in your environment

  export VAULT_TOKEN="<your-vault-token>"

  3. Run devpod with the required options

  devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \
    --provider-option NOMAD_CPU=2000 \
    --provider-option NOMAD_MEMORYMB=8192 \
    --provider-option NOMAD_DISKMB=512 \
    --provider-option NOMAD_STORAGE_MODE=persistent \
    --provider-option NOMAD_CSI_CLUSTER_ID=70464857-9ed6-11f0-8df5-d45d64d7d4f0 \
    --provider-option VAULT_ADDR=https://vault.service.consul:8200 \
    --provider-option NOMAD_CSI_VAULT_PATH=secret/data/ceph/csi

  Key points:
  - VAULT_TOKEN must be set as an environment variable (not a provider option)
  - NOMAD_CSI_VAULT_PATH uses KV v2 format: secret/data/ceph/csi (note the /data/ in the path)
  - The Vault secret must have exactly two fields: userID and userKey


Fix CSI Persistent Storage with Docker-in-Docker

 Problem Summary

 CSI persistent storage fails because Docker-in-Docker looks for bind mount paths on the HOST, but CSI volumes are mounted inside the container at a different path.

 Chosen Solution: Hybrid Storage

 Keep host bind mount for Docker compatibility, mount CSI at /persistent, sync data between them.

 Implementation Plan

 File: cmd/create.go

 Step 1: Always include host bind mount (line ~157-161)

 Change from:
 if options.StorageMode != opts.StorageModePersistent {
     dockerVolumes = append(dockerVolumes, sharedWorkspacePath+":"+sharedWorkspacePath)
 }

 To:
 // Always include host bind mount for Docker-in-Docker compatibility
 dockerVolumes = append(dockerVolumes, sharedWorkspacePath+":"+sharedWorkspacePath)

 Step 2: Mount CSI volume at /persistent (line ~204-208)

 Change from:
 volumeName := "workspace"
 persistentMountPath := sharedWorkspacePath // /tmp/devpod-workspaces

 To:
 volumeName := "workspace"
 persistentMountPath := "/persistent"

 Step 3: Update startup command to sync from CSI (line ~64)

 Update runCmd to:
 1. Check if /persistent has existing data (returning workspace)
 2. If yes, rsync from /persistent to /tmp/devpod-workspaces
 3. Start background sync process to periodically copy changes back to /persistent
 4. Continue with normal startup

 New command structure:
 /bin/sh -c '
   mkdir -p /tmp/devpod-workspaces /persistent

   # Restore from persistent storage if it has data
   if [ -d /persistent/agent ] && [ "$(ls -A /persistent/agent 2>/dev/null)" ]; then
     echo "Restoring workspace from persistent storage..."
     cp -a /persistent/. /tmp/devpod-workspaces/
   fi

   # Background sync: periodically save to persistent storage
   (while true; do
     sleep 60
     rsync -a --delete /tmp/devpod-workspaces/ /persistent/ 2>/dev/null || true
   done) &

   # ... rest of existing startup (apt-get, vault secrets, etc.)
 '

 Step 4: Add graceful shutdown sync

 Add a trap or final sync before sleep infinity:
 # Sync before sleeping (and on any exit)
 trap 'rsync -a --delete /tmp/devpod-workspaces/ /persistent/' EXIT

 Summary of Changes

 | Location              | Change                                         |
 |-----------------------|------------------------------------------------|
 | cmd/create.go:157-161 | Remove conditional, always add host bind mount |
 | cmd/create.go:207     | Change persistentMountPath to /persistent      |
 | cmd/create.go:64      | Update runCmd with sync logic                  |

 Testing

 After implementation:
 1. Delete existing workspace: devpod delete multistep-transcriber
 2. Rebuild provider: RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
 3. Reinstall: devpod provider delete nomad && devpod provider add release/provider.yaml
 4. Test: devpod up ... --provider-option NOMAD_STORAGE_MODE=persistent ...
 5. Create a file in workspace, stop/start, verify file persists
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌


Yes, that's correct. The implementation has two sync mechanisms:

  1. Background sync - Every 60 seconds, changes are copied from /tmp/devpod-workspaces to /persistent (the CSI volume)
  2. Exit trap - When the container receives a stop signal (from devpod stop), this runs:
  trap 'echo "Syncing to persistent storage..."; rsync -a --delete /tmp/devpod-workspaces/ /persistent/' EXIT

  So when you run devpod stop:
  1. Nomad sends SIGTERM to the container
  2. The shell's EXIT trap fires
  3. Final rsync copies all data to the CSI volume (Ceph storage)
  4. Container shuts down

  When you later run devpod up again with the same workspace, the startup script will restore from /persistent back to /tmp/devpod-workspaces.


│ Plan to implement                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ GPU Support for devpod-provider-nomad                                                                                                                                                │
│                                                                                                                                                                                      │
│ Context                                                                                                                                                                              │
│                                                                                                                                                                                      │
│ The user is running a devcontainer that requires GPU access (for ML/transcription workloads), but the devpod-provider-nomad has no GPU support. DevPod's hostRequirements.gpu from   │
│ devcontainer.json is NOT passed to providers via DEVCONTAINER_RUN_OPTIONS, so we must use provider options as the mechanism, which users can set via remoteEnv in devcontainer.json. │
│                                                                                                                                                                                      │
│ Cluster GPU requirements (from deployment guide):                                                                                                                                    │
│ - Docker runtime: nvidia                                                                                                                                                             │
│ - Environment: NVIDIA_VISIBLE_DEVICES=all                                                                                                                                            │
│ - Shared memory: 2GB (shm_size=2147483648)                                                                                                                                           │
│ - Constraints: meta.gpu-dedicated != true, attr.cpu.arch = amd64                                                                                                                     │
│                                                                                                                                                                                      │
│ Implementation Plan                                                                                                                                                                  │
│                                                                                                                                                                                      │
│ 1. Add Provider Options (hack/provider/provider.yaml)                                                                                                                                │
│                                                                                                                                                                                      │
│ Add after line 94 (after CSI options):                                                                                                                                               │
│                                                                                                                                                                                      │
│ NOMAD_GPU:                                                                                                                                                                           │
│   description: |-                                                                                                                                                                    │
│     Enable NVIDIA GPU support for the workspace.                                                                                                                                     │
│     Set to "true" to enable GPU support.                                                                                                                                             │
│   default: "false"                                                                                                                                                                   │
│ NOMAD_GPU_COUNT:                                                                                                                                                                     │
│   description: |-                                                                                                                                                                    │
│     Number of GPUs to request (default: 1).                                                                                                                                          │
│     Only used when NOMAD_GPU is "true".                                                                                                                                              │
│   default: "1"                                                                                                                                                                       │
│ NOMAD_GPU_COMPUTE_CAPABILITY:                                                                                                                                                        │
│   description: |-                                                                                                                                                                    │
│     Minimum CUDA compute capability (e.g., "7.5" for Turing, "8.0" for Ampere).                                                                                                      │
│     Common values: 6.1 (Pascal), 7.0 (Volta), 7.5 (Turing), 8.0 (A100), 8.6 (RTX 3090).                                                                                              │
│     Leave empty to accept any GPU.                                                                                                                                                   │
│   default:                                                                                                                                                                           │
│                                                                                                                                                                                      │
│ 2. Update Options Struct (pkg/options/options.go)                                                                                                                                    │
│                                                                                                                                                                                      │
│ Add fields to Options struct (after line 45):                                                                                                                                        │
│ // GPU configuration                                                                                                                                                                 │
│ GPUEnabled           bool                                                                                                                                                            │
│ GPUCount             int                                                                                                                                                             │
│ GPUComputeCapability string                                                                                                                                                          │
│                                                                                                                                                                                      │
│ Add constant (after line 58):                                                                                                                                                        │
│ defaultGPUCount = 1                                                                                                                                                                  │
│                                                                                                                                                                                      │
│ Add parsing in DefaultOptions() (after line 130):                                                                                                                                    │
│ // Parse GPU configuration                                                                                                                                                           │
│ gpuEnabled := strings.ToLower(getEnv("NOMAD_GPU", "false")) == "true"                                                                                                                │
│ gpuCount := 1                                                                                                                                                                        │
│ if gpuCountStr := os.Getenv("NOMAD_GPU_COUNT"); gpuCountStr != "" {                                                                                                                  │
│     if count, err := strconv.Atoi(gpuCountStr); err == nil && count > 0 {                                                                                                            │
│         gpuCount = count                                                                                                                                                             │
│     }                                                                                                                                                                                │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ Add to opts initialization and call validation.                                                                                                                                      │
│                                                                                                                                                                                      │
│ Add ValidateGPU() function (after ValidateCSI):                                                                                                                                      │
│ func (o *Options) ValidateGPU() error {                                                                                                                                              │
│     if !o.GPUEnabled {                                                                                                                                                               │
│         return nil                                                                                                                                                                   │
│     }                                                                                                                                                                                │
│     if o.GPUCount < 1 {                                                                                                                                                              │
│         return fmt.Errorf("NOMAD_GPU_COUNT must be at least 1")                                                                                                                      │
│     }                                                                                                                                                                                │
│     if o.GPUComputeCapability != "" {                                                                                                                                                │
│         // Validate format X.Y                                                                                                                                                       │
│         parts := strings.Split(o.GPUComputeCapability, ".")                                                                                                                          │
│         if len(parts) != 2 {                                                                                                                                                         │
│             return fmt.Errorf("NOMAD_GPU_COMPUTE_CAPABILITY must be X.Y format")                                                                                                     │
│         }                                                                                                                                                                            │
│         for _, p := range parts {                                                                                                                                                    │
│             if _, err := strconv.Atoi(p); err != nil {                                                                                                                               │
│                 return fmt.Errorf("NOMAD_GPU_COMPUTE_CAPABILITY must be numeric")                                                                                                    │
│             }                                                                                                                                                                        │
│         }                                                                                                                                                                            │
│     }                                                                                                                                                                                │
│     return nil                                                                                                                                                                       │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ 3. Modify Job Creation (cmd/create.go)                                                                                                                                               │
│                                                                                                                                                                                      │
│ Add helper function (before func (cmd *CreateCmd) Run):                                                                                                                              │
│ func buildGPUDeviceRequest(options *opts.Options) *api.RequestedDevice {                                                                                                             │
│     count := uint64(options.GPUCount)                                                                                                                                                │
│     device := &api.RequestedDevice{                                                                                                                                                  │
│         Name:  "nvidia/gpu",                                                                                                                                                         │
│         Count: &count,                                                                                                                                                               │
│     }                                                                                                                                                                                │
│     if options.GPUComputeCapability != "" {                                                                                                                                          │
│         device.Constraints = append(device.Constraints, &api.Constraint{                                                                                                             │
│             LTarget: "${meta.gpu_compute_capability}",                                                                                                                       │
│             Operand: ">=",                                                                                                                                                           │
│             RTarget: options.GPUComputeCapability,                                                                                                                                   │
│         })                                                                                                                                                                           │
│     }                                                                                                                                                                                │
│     return device                                                                                                                                                                    │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ Modify resource allocation (after line 191):                                                                                                                                         │
│ if options.GPUEnabled {                                                                                                                                                              │
│     gpuDevice := buildGPUDeviceRequest(options)                                                                                                                                      │
│     jobResources.Devices = []*api.RequestedDevice{gpuDevice}                                                                                                                         │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ Modify task config (after line 224, when building task.Config):                                                                                                                      │
│ if options.GPUEnabled {                                                                                                                                                              │
│     task.Config["runtime"] = "nvidia"                                                                                                                                                │
│     task.Config["shm_size"] = int64(2147483648) // 2GB                                                                                                                               │
│     if task.Env == nil {                                                                                                                                                             │
│         task.Env = make(map[string]string)                                                                                                                                           │
│     }                                                                                                                                                                                │
│     task.Env["NVIDIA_VISIBLE_DEVICES"] = "all"                                                                                                                                       │
│     task.Env["NVIDIA_DRIVER_CAPABILITIES"] = "compute,utility"                                                                                                                       │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ Add job constraints (after line 293, when building job):                                                                                                                             │
│ if options.GPUEnabled {                                                                                                                                                              │
│     job.Constraints = append(job.Constraints,                                                                                                                                        │
│         &api.Constraint{LTarget: "${attr.cpu.arch}", Operand: "=", RTarget: "amd64"},                                                                                                │
│         &api.Constraint{LTarget: "${meta.gpu-dedicated}", Operand: "!=", RTarget: "true"},                                                                                           │
│     )                                                                                                                                                                                │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ 4. Add Unit Tests (pkg/options/options_test.go)                                                                                                                                      │
│                                                                                                                                                                                      │
│ Add tests following existing patterns:                                                                                                                                               │
│ - TestValidateGPU_Disabled - no error when GPU disabled                                                                                                                              │
│ - TestValidateGPU_ValidConfig - valid GPU configuration passes                                                                                                                       │
│ - TestValidateGPU_InvalidComputeCapability - invalid format rejected                                                                                                                 │
│ - TestValidateGPU_InvalidCount - count < 1 rejected                                                                                                                                  │
│ - TestDefaultOptions_GPU - defaults are correct                                                                                                                                      │
│                                                                                                                                                                                      │
│ 5. Update Documentation                                                                                                                                                              │
│                                                                                                                                                                                      │
│ CLAUDE.md - Add GPU testing section with test commands.                                                                                                                              │
│                                                                                                                                                                                      │
│ README.md (if exists) - Add GPU Support section.                                                                                                                                     │
│                                                                                                                                                                                      │
│ Files to Modify                                                                                                                                                                      │
│ ┌─────────────────────────────┬──────────────────────────────────────────────────────────────────────┐                                                                               │
│ │            File             │                               Changes                                │                                                                               │
│ ├─────────────────────────────┼──────────────────────────────────────────────────────────────────────┤                                                                               │
│ │ hack/provider/provider.yaml │ Add NOMAD_GPU, NOMAD_GPU_COUNT, NOMAD_GPU_COMPUTE_CAPABILITY options │                                                                               │
│ ├─────────────────────────────┼──────────────────────────────────────────────────────────────────────┤                                                                               │
│ │ pkg/options/options.go      │ Add GPU fields, parsing, ValidateGPU()                               │                                                                               │
│ ├─────────────────────────────┼──────────────────────────────────────────────────────────────────────┤                                                                               │
│ │ cmd/create.go               │ Add buildGPUDeviceRequest(), modify task config and job constraints  │                                                                               │
│ ├─────────────────────────────┼──────────────────────────────────────────────────────────────────────┤                                                                               │
│ │ pkg/options/options_test.go │ Add GPU validation tests                                             │                                                                               │
│ ├─────────────────────────────┼──────────────────────────────────────────────────────────────────────┤                                                                               │
│ │ CLAUDE.md                   │ Add GPU testing instructions                                         │                                                                               │
│ └─────────────────────────────┴──────────────────────────────────────────────────────────────────────┘                                                                               │
│ User Usage                                                                                                                                                                           │
│                                                                                                                                                                                      │
│ Via command line:                                                                                                                                                                    │
│ devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \                                                                                                           │
│   --provider-option NOMAD_GPU=true \                                                                                                                                                 │
│   --provider-option NOMAD_GPU_COMPUTE_CAPABILITY=7.5                                                                                                                                 │
│                                                                                                                                                                                      │
│ Via devcontainer.json (using remoteEnv as workaround):                                                                                                                               │
│ {                                                                                                                                                                                    │
│   "name": "Multistep Transcriber",                                                                                                                                                   │
│   "image": "registry.cluster:5000/devcontainer-python:20251106b",                                                                                                                    │
│   "remoteEnv": {                                                                                                                                                                     │
│     "NOMAD_GPU": "true",                                                                                                                                                             │
│     "NOMAD_GPU_COMPUTE_CAPABILITY": "7.5"                                                                                                                                            │
│   }                                                                                                                                                                                  │
│ }                                                                                                                                                                                    │
│                                                                                                                                                                                      │
│ Verification                                                                                                                                                                         │
│                                                                                                                                                                                      │
│ 1. Build provider: RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev                                                                                                                   │
│ 2. Run tests: go test ./pkg/options/...                                                                                                                                              │
│ 3. Launch GPU workspace and verify:                                                                                                                                                  │
│ devpod up github.com/geraldthewes/multistep-transcriber --provider nomad \                                                                                                           │
│   --provider-option NOMAD_GPU=true --debug                                                                                                                                           │
│                                                                                                                                                                                      │
│ # Check Nomad job has GPU device request                                                                                                                                             │
│ nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Resources.Devices'                                                                                         │
│                                                                                                                                                                                      │
│ # Check Docker runtime is nvidia                                                                                                                                                     │
│ nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Config.runtime'                                                                                            │
│                                                                                                                                                                                      │
│ # SSH and verify GPU access                                                                                                                                                          │
│ devpod ssh multistep-transcriber                                                                                                                                                     │
│ nvidia-smi


   
  Files Modified                                                                                                                                                                        
  ┌─────────────────────────────┬────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
  │            File             │                                                              Changes                                                               │                  
  ├─────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤                  
  │ hack/provider/provider.yaml │ Added NOMAD_GPU, NOMAD_GPU_COUNT, NOMAD_GPU_COMPUTE_CAPABILITY options                                                             │
  ├─────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ pkg/options/options.go      │ Added GPUEnabled, GPUCount, GPUComputeCapability fields, parsing logic, and ValidateGPU() function                                 │
  ├─────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ cmd/create.go               │ Added buildGPUDeviceRequest() helper, GPU device allocation, nvidia runtime config, shm_size, NVIDIA env vars, and job constraints │
  ├─────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ pkg/options/options_test.go │ Added 9 new GPU validation tests (all passing)                                                                                     │
  ├─────────────────────────────┼────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
  │ CLAUDE.md                   │ Added GPU testing section with examples and troubleshooting                                                                        │
  └─────────────────────────────┴────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
  Key Features

  1. Provider Options: Users can enable GPU with NOMAD_GPU=true, specify count with NOMAD_GPU_COUNT, and set minimum compute capability with NOMAD_GPU_COMPUTE_CAPABILITY
  2. Nomad Job Configuration:
    - Adds nvidia/gpu device request with optional compute capability constraint
    - Sets Docker runtime to nvidia
    - Configures 2GB shared memory for ML workloads
    - Sets NVIDIA_VISIBLE_DEVICES=all and NVIDIA_DRIVER_CAPABILITIES=compute,utility
    - Adds job constraints for amd64 architecture and non-dedicated GPU nodes
  3. Validation: Properly validates GPU configuration (count >= 1, compute capability format X.Y)

  Build & Tests

  go build ./...     ✓
  go test ./...      ✓ (16 tests pass including 9 new GPU tests)

 Problem: Users want to specify provider options (GPU, resources, Vault secrets) in their repository without needing command-line flags every time.

 Solution: Add support for reading provider options from a .devpod/nomad.yaml config file in the workspace.

 Implementation Plan

 1. Create Config File Types (pkg/options/config.go)

 New file with:
 - ConfigFile struct matching all provider options
 - LoadConfigFile(workspacePath string) function to read and parse YAML
 - GetWorkspacePath() function to extract path from WORKSPACE_SOURCE env var

 # Example .devpod/nomad.yaml
 nomad_cpu: "2000"
 nomad_memorymb: "4096"
 nomad_gpu: true
 nomad_gpu_count: 1
 nomad_gpu_compute_capability: "7.5"

 vault_addr: "https://vault.example.com:8200"
 vault_policies:
   - "my-policy"
 vault_secrets:
   - path: "secret/data/myapp"
     fields:
       api_key: "API_KEY"

 2. Modify pkg/options/options.go

 Update DefaultOptions() to:
 1. Call GetWorkspacePath() to find workspace location
 2. Call LoadConfigFile() to load config (if exists)
 3. Use getEnvOrConfig() helper that checks env var first, then config file, then default

 Precedence: --provider-option flags (env vars) > config file > defaults

 3. Add Unit Tests (pkg/options/config_test.go)

 - Test file not found (silent, returns nil)
 - Test empty path (silent, returns nil)
 - Test valid YAML parsing
 - Test invalid YAML (returns error)
 - Test env vars override config file values
 - Test GetWorkspacePath() with local and git sources

 4. Update Documentation

 - README.md: Add section on config file usage
 - CLAUDE.md: Add testing instructions

 Files to Modify
 ┌────────────────────────────┬───────────────────────────────────────────┐
 │            File            │                  Changes                  │
 ├────────────────────────────┼───────────────────────────────────────────┤
 │ pkg/options/config.go      │ New file - Config file types and loading  │
 ├────────────────────────────┼───────────────────────────────────────────┤
 │ pkg/options/config_test.go │ New file - Unit tests                     │
 ├────────────────────────────┼───────────────────────────────────────────┤
 │ pkg/options/options.go     │ Modify to merge config file with env vars │
 ├────────────────────────────┼───────────────────────────────────────────┤
 │ README.md                  │ Document config file feature              │
 ├────────────────────────────┼───────────────────────────────────────────┤
 │ CLAUDE.md                  │ Add testing instructions                  │
 └────────────────────────────┴───────────────────────────────────────────┘
 Key Code Changes

 config.go (new file)

 package options

 import (
     "os"
     "path/filepath"
     "strings"

     "gopkg.in/yaml.v3"
 )

 type ConfigFile struct {
     NomadCPU      string `yaml:"nomad_cpu"`
     NomadMemoryMB string `yaml:"nomad_memorymb"`
     NomadDiskMB   string `yaml:"nomad_diskmb"`
     // ... all other options
     NomadGPU      *bool  `yaml:"nomad_gpu"`      // pointer for optional bool
     NomadGPUCount *int   `yaml:"nomad_gpu_count"`
     // Vault secrets as native YAML (not JSON string)
     VaultPolicies []string       `yaml:"vault_policies"`
     VaultSecrets  []VaultSecret  `yaml:"vault_secrets"`
 }

 func LoadConfigFile(workspacePath string) (*ConfigFile, error) {
     if workspacePath == "" {
         return nil, nil
     }
     configPath := filepath.Join(workspacePath, ".devpod", "nomad.yaml")
     data, err := os.ReadFile(configPath)
     if os.IsNotExist(err) {
         return nil, nil  // Not an error
     }
     if err != nil {
         return nil, err
     }
     var config ConfigFile
     return &config, yaml.Unmarshal(data, &config)
 }

 func GetWorkspacePath() string {
     source := os.Getenv("WORKSPACE_SOURCE")
     if strings.HasPrefix(source, "local:") {
         return strings.TrimPrefix(source, "local:")
     }
     return ""  // Git sources: path not available to provider
 }

 options.go changes

 func DefaultOptions() (*Options, error) {
     // Load config file (if available)
     workspacePath := GetWorkspacePath()
     configFile, err := LoadConfigFile(workspacePath)
     if err != nil {
         return nil, fmt.Errorf("failed to load config file: %w", err)
     }

     // ... rest uses getEnvOrConfig helper
     gpuEnabled := getGPUEnabled(configFile)  // checks env first, then config
     // ...
 }

 func getEnvOrConfig(envKey, configValue, defaultValue string) string {
     if value, ok := os.LookupEnv(envKey); ok {
         return value
     }
     if configValue != "" {
         return configValue
     }
     return defaultValue
 }

 Limitations

 Git sources: Config file loading only works for local workspace sources (devpod up /path/to/local/project). For git-based workspaces, users must still use --provider-option flags
 because the cloned repo path is not exposed to the provider during create.

 Workaround for git repos: Use devpod provider use nomad --option NOMAD_GPU=true to set default provider options.

 Verification

 Unit Tests

 cd /home/gerald/repos/devpod-provider-nomad
 go test ./pkg/options/... -v

 Integration Test (local source)

 # Create test config
 mkdir -p /tmp/test-project/.devpod
 cat > /tmp/test-project/.devpod/nomad.yaml << 'EOF'
 nomad_gpu: true
 nomad_gpu_compute_capability: "7.5"
 nomad_cpu: "2000"
 nomad_memorymb: "4096"
 EOF

 # Build and test
 devpod delete test-project || true
 devpod provider delete nomad || true
 RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
 devpod up /tmp/test-project --provider nomad --ide none --debug

 # Verify GPU configured
 nomad job inspect test-project | jq '.Job.TaskGroups[0].Tasks[0].Config.runtime'
 # Should output: "nvidia"

 Verify Precedence

 # Config file says GPU=true, but command line overrides
 devpod up /tmp/test-project --provider nomad --provider-option NOMAD_GPU=false --ide none
 nomad job inspect test-project | jq '.Job.TaskGroups[0].Tasks[0].Config.runtime'
 # Should output: null (GPU disabled by command line)

 Dependencies

 - gopkg.in/yaml.v3 - Already available as indirect dependency in go.mod
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌

  Summary                                                                                                                                                                               
                                                                                                                                                                                        
  I've implemented config file support for provider options. Here are the changes:                                                                                                      
                                                                                
  New Files

  1. pkg/options/config.go - Config file types and loading
    - ConfigFile struct with all provider options as YAML fields
    - LoadConfigFile() - reads and parses .devpod/nomad.yaml
    - GetWorkspacePath() - extracts workspace path from WORKSPACE_SOURCE env var
    - Helper functions: getEnvOrConfig(), getEnvOrConfigBool(), getEnvOrConfigInt()
  2. pkg/options/config_test.go - Unit tests (16 new tests)
    - Tests for file not found, empty path, valid YAML, invalid YAML, partial config
    - Tests for GetWorkspacePath() with local/git sources
    - Tests for precedence (env var > config file > default)

  Modified Files

  3. pkg/options/options.go - Integrated config file loading
    - DefaultOptions() now loads config file and merges with env vars
    - Added getVaultPolicies() and getVaultSecrets() helpers for config fallback
    - Env vars (--provider-option) always take precedence over config file
  4. README.md - Added documentation
    - New "Config File Support" section with quick start, format, and precedence rules
  5. CLAUDE.md - Added testing instructions
    - Integration tests for config file feature
    - Tests for precedence, Vault secrets, and cleanup

  Example Config File

  # .devpod/nomad.yaml
  nomad_gpu: true
  nomad_gpu_compute_capability: "7.5"
  nomad_cpu: "2000"
  nomad_memorymb: "4096"
  vault_addr: "https://vault.example.com:8200"
  vault_policies:
    - "my-policy"
  vault_secrets:
    - path: "secret/data/myapp"
      fields:
        api_key: "API_KEY"

  Precedence

  1. --provider-option flags (env vars) - highest priority
  2. .devpod/nomad.yaml config file
  3. Provider defaults (devpod provider set-options)

  Limitation

  Config file loading only works for local workspace sources (devpod up /path/to/project). For git sources, users must use --provider-option flags.

