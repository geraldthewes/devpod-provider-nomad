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

# SSH into workspace and verify environment variables
devpod ssh vscode-remote-try-node
env | grep -E 'TEST_KEY|API_TOKEN|AWS_'
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
