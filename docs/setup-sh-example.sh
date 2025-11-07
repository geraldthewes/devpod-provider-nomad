#!/bin/bash
# Example setup.sh snippet for sourcing Vault secrets
# This script should be run from your workspace root directory

# Source Vault secrets if available
VAULT_SECRETS_FILE=".vault-secrets"

if [ -f "$VAULT_SECRETS_FILE" ]; then
    echo "Loading Vault secrets from $VAULT_SECRETS_FILE..."
    # Source the secrets file to load environment variables
    set -a  # automatically export all variables
    source "$VAULT_SECRETS_FILE"
    set +a
    echo "Vault secrets loaded successfully"

    # Verify critical secrets are present (optional)
    if [ -n "$AWS_ACCESS_KEY_ID" ] && [ -n "$AWS_SECRET_ACCESS_KEY" ]; then
        echo "✓ AWS credentials loaded"
    fi
    if [ -n "$HF_TOKEN" ]; then
        echo "✓ HuggingFace token loaded"
    fi
else
    echo "INFO: No Vault secrets file found at $VAULT_SECRETS_FILE"
    echo "This is normal if you're not using Vault integration"
fi

# Continue with the rest of your setup...
