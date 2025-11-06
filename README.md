# DevPod Provider for Nomad

Author: Brian Cain

[![Go](https://github.com/briancain/devpod-provider-nomad/actions/workflows/go.yml/badge.svg)](https://github.com/briancain/devpod-provider-nomad/actions/workflows/go.yml) [![Release](https://github.com/briancain/devpod-provider-nomad/actions/workflows/release.yml/badge.svg)](https://github.com/briancain/devpod-provider-nomad/actions/workflows/release.yml)

This is a provider for [DevPod](https://devpod.sh/) that allows you to create a
DevPod using [HashiCorp Nomad](https://www.nomadproject.io/).

Please report any issues or feature requests to the
[Github Issues](https://github.com/briancain/devpod-provider-nomad/issues) page.

This project is still a work in progress, excuse our mess! <3

[devpod.sh](https://devpod.sh/)

## Getting Started

1. Install the provider to your local machine

From Github:

```shell
devpod provider add geraldthewes/devpod-provider-nomad
```

2. Use the provider

```shell
devpod up <repository-url> --provider nomad
```

### Provider Configurations

Set these options through DevPod to configure them when DevPod launches the
Nomad job during a workspace creation.

- NOMAD_NAMESPACE:
  + description: The namespace for the Nomad job
  + default:
- NOMAD_REGION:
  + description: The region for the Nomad job
  + default:
- NOMAD_CPU:
  + description: The cpu in mhz to use for the Nomad Job
  + default: "200"
- NOMAD_MEMORYMB:
  + description: The memory in mb to use for the Nomad Job
  + default: "512"
- NOMAD_DISKMB:
  + description: The ephemeral disk in mb to use for the Nomad Job
  + default: "300"

#### Setting Resource Options

You can configure resources when starting a workspace using `--provider-option` flags:

```shell
devpod up <repository-url> --provider nomad \
  --provider-option NOMAD_CPU=2000 \
  --provider-option NOMAD_MEMORYMB=8192 \
  --provider-option NOMAD_DISKMB=1024
```

Or set them as persistent defaults for all workspaces:

```shell
devpod provider set-options nomad \
  --option NOMAD_CPU=2000 \
  --option NOMAD_MEMORYMB=8192 \
  --option NOMAD_DISKMB=1024
```

Verify your configuration:

```shell
devpod provider options nomad
```

## Environment Variables

DevPod supports injecting environment variables into your development containers. This is useful for secrets (API keys, tokens) and configuration (hosts, ports, etc.). There are two main approaches: **global variables** (shared across all projects) and **project-specific variables**.

### Understanding remoteEnv vs containerEnv

**IMPORTANT:** Use `remoteEnv` for runtime environment variables in your running container:

- ✅ **`remoteEnv`** - Sets environment variables in the running container (use this!)
- ❌ **`containerEnv`** - Sets environment variables during container build (not what you want for runtime vars)

### Global Environment Variables

For secrets and configuration you want available across **all your projects** (like `HF_TOKEN`, `OLLAMA_HOST`, `GITHUB_TOKEN`):

**Step 1:** Set the environment variable on your local machine:

```bash
# Add to your ~/.bashrc, ~/.zshrc, or equivalent
export HF_TOKEN="hf_xxxxxxxxxxxxxxxxxxxxx"
export OLLAMA_HOST="http://localhost:11434"
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxxx"
```

**Step 2:** Reference it in your project's `.devcontainer/devcontainer.json`:

```json
{
  "name": "My Project",
  "image": "mcr.microsoft.com/devcontainers/python:3.12",
  "remoteEnv": {
    "HF_TOKEN": "${localEnv:HF_TOKEN}",
    "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
    "GITHUB_TOKEN": "${localEnv:GITHUB_TOKEN}"
  }
}
```

The `${localEnv:VAR_NAME}` syntax tells DevPod to read the variable from your local machine and inject it into the container at startup.

### Project-Specific Environment Variables

For variables that are unique to a specific project:

#### Option 1: Hardcoded in devcontainer.json

For non-sensitive configuration:

```json
{
  "name": "My API Project",
  "image": "mcr.microsoft.com/devcontainers/python:3.12",
  "remoteEnv": {
    "API_BASE_URL": "https://api.example.com",
    "LOG_LEVEL": "debug",
    "ENVIRONMENT": "development"
  }
}
```

#### Option 2: Using an .env file

For project-specific secrets that shouldn't be committed to git:

**Step 1:** Create `.devcontainer/devcontainer.env`:

```bash
# .devcontainer/devcontainer.env
PROJECT_API_KEY=abc123def456
DATABASE_PASSWORD=secret123
```

**Step 2:** Add to your `.gitignore`:

```
.devcontainer/devcontainer.env
```

**Step 3:** Reference it in `.devcontainer/devcontainer.json`:

```json
{
  "name": "My Project",
  "image": "mcr.microsoft.com/devcontainers/python:3.12",
  "runArgs": ["--env-file", "${localWorkspaceFolder}/.devcontainer/devcontainer.env"]
}
```

#### Option 3: Combining Global and Project-Specific

You can mix both approaches:

```json
{
  "name": "ML Training Project",
  "image": "mcr.microsoft.com/devcontainers/python:3.12",
  "remoteEnv": {
    "HF_TOKEN": "${localEnv:HF_TOKEN}",
    "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
    "PROJECT_NAME": "ml-training",
    "MODEL_PATH": "/workspace/models",
    "BATCH_SIZE": "32"
  }
}
```

### Complete Example

Here's a complete `.devcontainer/devcontainer.json` with environment variables:

```json
{
  "name": "Python ML Development",
  "image": "mcr.microsoft.com/devcontainers/python:1-3.12-bookworm",
  "remoteUser": "vscode",

  "remoteEnv": {
    "HF_TOKEN": "${localEnv:HF_TOKEN}",
    "OLLAMA_HOST": "${localEnv:OLLAMA_HOST}",
    "PYTHONPATH": "${containerWorkspaceFolder}",
    "PROJECT_ENV": "development"
  },

  "postCreateCommand": "pip install -r requirements.txt",

  "features": {
    "ghcr.io/devcontainers/features/python:1": {
      "version": "3.12"
    }
  }
}
```

### Verifying Environment Variables

After your workspace starts, verify the variables are set:

```bash
# SSH into your workspace
devpod ssh your-workspace

# Check environment variables
env | grep HF_TOKEN
echo $OLLAMA_HOST
```

### Troubleshooting

**Variables not appearing in container:**
- ✅ Make sure you're using `remoteEnv`, NOT `containerEnv`
- ✅ Verify the variable is set on your local machine: `echo $HF_TOKEN`
- ✅ Check the syntax: `"${localEnv:VAR_NAME}"` (include quotes and exact casing)
- ✅ Restart your workspace: `devpod delete <workspace> && devpod up ...`

**SSH config SetEnv doesn't work:**
- The Nomad provider doesn't use SSH for initial connection, so `~/.ssh/config` `SetEnv` directives won't work
- Use `remoteEnv` in devcontainer.json instead

**Variables work locally but not in DevPod:**
- DevPod only has access to environment variables that exist when `devpod up` runs
- Make sure variables are exported in your shell profile and you've restarted your terminal

## Using Private Docker Registries

The provider supports using private Docker registries with custom TLS certificates. This is useful when working with self-hosted registries or registries with self-signed certificates.

### How It Works

The provider automatically mounts certificates from the Nomad client hosts into the DevPod containers:

- `/etc/docker/certs.d/<registry>/ca.crt` - Docker registry certificates (mounted read-only)
- `/usr/local/share/ca-certificates/registry.cluster.crt` - CA certificate source file (mounted read-only)

**Why two mounts?**
- The Docker daemon on the Nomad client uses `/etc/docker/certs.d/` when pulling images
- DevPod makes direct API calls to the registry to inspect images and needs the CA cert in its trust store
- When the container starts, `update-ca-certificates` runs and includes the mounted certificate in the container's CA bundle at `/etc/ssl/certs/ca-certificates.crt`

### Setting Up Registry Certificates

**Step 1:** Place your registry's CA certificate on **each Nomad client node** that will run DevPod workspaces.

The certificate must be placed in two locations:

1. Docker registry certificate directory (for Docker daemon):
```bash
# On each Nomad client node (example for registry.cluster:5000)
sudo mkdir -p /etc/docker/certs.d/registry.cluster:5000
sudo cp /path/to/ca.crt /etc/docker/certs.d/registry.cluster:5000/ca.crt
sudo chmod 644 /etc/docker/certs.d/registry.cluster:5000/ca.crt
```

2. System CA certificates directory (for DevPod API calls):
```bash
# On each Nomad client node
# IMPORTANT: The filename must be exactly "registry.cluster.crt"
sudo cp /path/to/ca.crt /usr/local/share/ca-certificates/registry.cluster.crt
sudo chmod 644 /usr/local/share/ca-certificates/registry.cluster.crt
sudo update-ca-certificates
```

**Note:** The filename `registry.cluster.crt` is hardcoded in the provider and must match exactly.

**Step 2:** Restart the Docker daemon on each Nomad client:
```bash
sudo systemctl restart docker
```

**Step 3:** Use your private registry in your devcontainer.json:
```json
{
  "name": "My Project",
  "image": "registry.cluster:5000/my-devcontainer:latest",
  "remoteUser": "vscode"
}
```

### Example Configuration

For a local registry with a self-signed certificate:

```json
{
  "name": "Python Development",
  "image": "registry.cluster:5000/devcontainer-python:latest",
  "remoteUser": "vscode",
  "postCreateCommand": "./setup.sh",
  "remoteEnv": {
    "PYTHONPATH": "${containerWorkspaceFolder}"
  }
}
```

Launch with:
```bash
devpod up github.com/your-org/your-project --provider nomad \
  --provider-option NOMAD_CPU=2000 \
  --provider-option NOMAD_MEMORYMB=8192
```

### Troubleshooting Registry Certificate Issues

**Error: "x509: certificate signed by unknown authority"**

This means the Docker daemon on the Nomad client cannot verify your registry's certificate.

Solutions:
1. ✅ Verify the certificate is at the correct path on **all Nomad client nodes**
2. ✅ Check the certificate filename is exactly `ca.crt` (not `ca.cert` or other variations)
3. ✅ Ensure the registry address in the path matches exactly (including port): `/etc/docker/certs.d/registry.cluster:5000/`
4. ✅ Restart the Docker daemon after adding certificates: `sudo systemctl restart docker`
5. ✅ Test Docker can pull from the registry directly on the Nomad client:
   ```bash
   # On the Nomad client
   docker pull registry.cluster:5000/your-image:tag
   ```

**Error: "Get https://registry.cluster:5000/v2/: dial tcp: lookup registry.cluster"**

This is a DNS issue, not a certificate issue:
- Ensure `registry.cluster` resolves correctly on the Nomad client nodes
- Add an entry to `/etc/hosts` if needed: `10.0.1.12 registry.cluster`

## Testing Locally

1. Build the provider locally

```shell
RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev
```

2. Delete the old provider from DevPod

```shell
devpod provider delete nomad
```

3. Install the new provider from a local build

```shell
devpod provider add --name nomad --use ./release/provider.yaml 
```

4. Test the provider

```shell
devpod up <repository-url> --provider nomad --debug 
```
