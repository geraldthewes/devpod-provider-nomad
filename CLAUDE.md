Testing
=======

## Basic Testing

Run the following commands:
```bash
devpod delete 'vscode-remote-try-node'
devpod provider delete nomad
RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug
```

## Testing with Vault Integration

### Prerequisites

1. Nomad cluster with Vault integration enabled
2. Vault instance accessible from Nomad
3. Test secrets stored in Vault KV v2

### Setup Test Secrets

```bash
# Store test secrets in Vault
vault kv put secret/test/devpod \
  test_key="test_value_123" \
  api_token="secret_token_456"
```

### Create Vault Policy

Create `devpod-test.hcl`:
```hcl
path "secret/data/test/devpod" {
  capabilities = ["read"]
}
```

Apply the policy:
```bash
vault policy write devpod-test devpod-test.hcl
```

### Test with Vault Secrets

```bash
# Clean up any existing workspace
devpod delete 'vscode-remote-try-node'
devpod provider delete nomad

# Build the provider
RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

# Launch workspace with Vault secrets using --provider-option flags
devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
  --provider-option VAULT_ADDR=https://vault.example.com:8200 \
  --provider-option VAULT_POLICIES_JSON='["devpod-test"]' \
  --provider-option VAULT_SECRETS_JSON='[{"path":"secret/data/test/devpod","fields":{"test_key":"TEST_KEY","api_token":"API_TOKEN"}}]'

# Verify secrets are injected
devpod ssh vscode-remote-try-node
echo $TEST_KEY
echo $API_TOKEN
```

### Test Multiple Secrets

```bash
# Store additional secrets
vault kv put secret/test/aws \
  access_key="AKIAIOSFODNN7EXAMPLE" \
  secret_key="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

# Update policy
vault policy write devpod-test - <<EOF
path "secret/data/test/devpod" {
  capabilities = ["read"]
}
path "secret/data/test/aws" {
  capabilities = ["read"]
}
EOF

# Test with multiple secrets using --provider-option
devpod delete 'vscode-remote-try-node'
devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
  --provider-option VAULT_ADDR=https://vault.example.com:8200 \
  --provider-option VAULT_POLICIES_JSON='["devpod-test"]' \
  --provider-option VAULT_SECRETS_JSON='[{"path":"secret/data/test/devpod","fields":{"test_key":"TEST_KEY","api_token":"API_TOKEN"}},{"path":"secret/data/test/aws","fields":{"access_key":"AWS_ACCESS_KEY_ID","secret_key":"AWS_SECRET_ACCESS_KEY"}}]'
```

### Test Per-Workspace Configuration

Create `.devcontainer/devcontainer.json` with Vault config:
```json
{
  "name": "Test with Vault",
  "image": "mcr.microsoft.com/devcontainers/javascript-node:latest",
  "remoteEnv": {
    "VAULT_ADDR": "https://vault.example.com:8200",
    "VAULT_POLICIES_JSON": "[\"devpod-test\"]",
    "VAULT_SECRETS_JSON": "[{\"path\":\"secret/data/test/devpod\",\"fields\":{\"test_key\":\"TEST_KEY\"}}]"
  }
}
```

### Verify Vault Integration

```bash
# Check Nomad job was created with Vault stanza
nomad job inspect vscode-remote-try-node | jq '.Job.TaskGroups[0].Tasks[0].Vault'

# Check Nomad allocation status
nomad job status vscode-remote-try-node

# View allocation logs to see template rendering
ALLOC_ID=$(nomad job status vscode-remote-try-node | grep running | awk '{print $1}' | head -1)
nomad alloc logs $ALLOC_ID

# SSH into workspace and verify secrets are accessible
devpod ssh vscode-remote-try-node

# Check if secrets file exists in workspace root (created automatically by provider)
ls -la .vault-secrets

# View the secrets file content
cat .vault-secrets

# Source the secrets and verify environment variables
source .vault-secrets
env | grep -E 'TEST_KEY|API_TOKEN|AWS_'

# Test sourcing in a script
echo 'source .vault-secrets && echo "Secrets loaded: TEST_KEY=$TEST_KEY"' | bash
```

### Troubleshooting Test Issues

**If secrets don't appear:**
```bash
# Check Nomad can reach Vault
nomad status

# Check allocation events
nomad alloc status $ALLOC_ID

# Check template rendering logs
nomad alloc logs -f $ALLOC_ID

# Verify Vault policy and secrets exist
vault policy read devpod-test
vault kv get secret/test/devpod

# Check if secrets file was created in Nomad task
nomad alloc exec $ALLOC_ID ls -la /tmp/devpod-workspaces/.vault-secrets
nomad alloc exec $ALLOC_ID cat /tmp/devpod-workspaces/.vault-secrets

# Check if secrets are in Nomad task environment
nomad alloc exec $ALLOC_ID env | grep -E 'AWS_|TEST_'

# If file exists in Nomad task but not in devcontainer, check mount
devpod ssh vscode-remote-try-node
ls -la /tmp/devpod-workspaces/
mount | grep devpod
```

**Test validation errors:**
```bash
# Test missing VAULT_ADDR (should fail)
devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
  --provider-option VAULT_SECRETS_JSON='[{"path":"secret/data/test","fields":{"key":"VAR"}}]'
# Expected: Error about VAULT_ADDR required

# Test missing policies (should fail)
devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
  --provider-option VAULT_ADDR=https://vault.example.com:8200 \
  --provider-option VAULT_SECRETS_JSON='[{"path":"secret/data/test","fields":{"key":"VAR"}}]'
# Expected: Error about VAULT_POLICIES_JSON required

# Test invalid change mode (should fail)
devpod up github.com/microsoft/vscode-remote-try-node --provider nomad --debug \
  --provider-option VAULT_ADDR=https://vault.example.com:8200 \
  --provider-option VAULT_POLICIES_JSON='["test"]' \
  --provider-option VAULT_SECRETS_JSON='[{"path":"secret/data/test","fields":{"key":"VAR"}}]' \
  --provider-option VAULT_CHANGE_MODE=invalid
# Expected: Error about invalid change mode
```

## Testing with GPU Support

### Prerequisites

1. Nomad cluster with NVIDIA GPU nodes
2. NVIDIA Docker runtime configured on GPU nodes
3. Nomad client fingerprinting GPUs (check with `nomad node status -verbose`)

### Basic GPU Test

```bash
# Clean up any existing workspace
devpod delete 'multistep-transcriber'
devpod provider delete nomad

# Build the provider
RELEASE_VERSION=0.0.1-dev ./hack/build.sh --dev

# Launch workspace with GPU support
devpod up github.com/geraldthewes/multistep-transcriber --provider nomad --debug \
  --provider-option NOMAD_GPU=true

# Verify GPU access
devpod ssh multistep-transcriber
nvidia-smi
```

### GPU with Compute Capability Requirement

```bash
# Request GPU with minimum compute capability (e.g., 7.5 for Turing)
devpod up github.com/geraldthewes/multistep-transcriber --provider nomad --debug \
  --provider-option NOMAD_GPU=true \
  --provider-option NOMAD_GPU_COMPUTE_CAPABILITY=7.5

# Request multiple GPUs
devpod up github.com/geraldthewes/multistep-transcriber --provider nomad --debug \
  --provider-option NOMAD_GPU=true \
  --provider-option NOMAD_GPU_COUNT=2
```

### GPU via devcontainer.json

Create `.devcontainer/devcontainer.json`:
```json
{
  "name": "GPU Workspace",
  "image": "registry.cluster:5000/devcontainer-python:20251106b",
  "remoteEnv": {
    "NOMAD_GPU": "true",
    "NOMAD_GPU_COMPUTE_CAPABILITY": "7.5"
  }
}
```

### Verify GPU Configuration

```bash
# Check Nomad job has GPU device request
nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Resources.Devices'

# Check Docker runtime is nvidia
nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Config.runtime'

# Check job constraints
nomad job inspect multistep-transcriber | jq '.Job.Constraints'

# Check shared memory size (should be 2GB)
nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Config.shm_size'

# Check NVIDIA environment variables
nomad job inspect multistep-transcriber | jq '.Job.TaskGroups[0].Tasks[0].Env'
```

### Troubleshooting GPU Issues

**If GPU is not detected:**
```bash
# Check if Nomad client detects GPUs
nomad node status -verbose <node-id> | grep -i gpu

# Check if nvidia-container-runtime is installed
docker info | grep -i runtime

# Verify GPU placement constraints
nomad job inspect multistep-transcriber | jq '.Job.Constraints'
```

**Test validation errors:**
```bash
# Test invalid GPU count (should fail)
devpod up github.com/geraldthewes/multistep-transcriber --provider nomad --debug \
  --provider-option NOMAD_GPU=true \
  --provider-option NOMAD_GPU_COUNT=0
# Expected: Error about GPU count must be at least 1

# Test invalid compute capability format (should fail)
devpod up github.com/geraldthewes/multistep-transcriber --provider nomad --debug \
  --provider-option NOMAD_GPU=true \
  --provider-option NOMAD_GPU_COMPUTE_CAPABILITY=invalid
# Expected: Error about compute capability format
```
