#!/bin/bash
# Helper script to set Vault provider options for Nomad provider
# This saves the options so you don't need to specify them every time with devpod up

echo "Setting Vault provider options for Nomad..."

# Set Vault server address
devpod provider set-options nomad \
  --option VAULT_ADDR=https://vault.service.consul:8200

# Set Vault role (optional, default is nomad-workloads)
devpod provider set-options nomad \
  --option VAULT_ROLE=nomad-workloads

# Set Vault policies (JSON array) - wrap in single quotes
devpod provider set-options nomad \
  --option 'VAULT_POLICIES_JSON=["transcription-policy"]'

# Set Vault secrets (complex nested JSON) - wrap in single quotes
devpod provider set-options nomad \
  --option 'VAULT_SECRETS_JSON=[{"path":"secret/data/aws/transcription","fields":{"access_key":"AWS_ACCESS_KEY_ID","secret_key":"AWS_SECRET_ACCESS_KEY"}},{"path":"secret/data/hf/transcription","fields":{"token":"HF_TOKEN"}}]'

echo ""
echo "✓ Vault options saved!"
echo ""
echo "You can now run devpod up without specifying --provider-option flags:"
echo "  devpod up /path/to/your/repo --provider nomad"
echo ""
echo "To view saved options:"
echo "  devpod provider options nomad"
