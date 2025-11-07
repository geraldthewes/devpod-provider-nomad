#!/bin/bash
# Enhanced setup.sh that configures Vault secrets for both current and future sessions

# Source secrets for this session (postCreateCommand)
if [ -f ".vault-secrets" ]; then
    echo "Loading Vault secrets for current session..."
    set -a
    source .vault-secrets
    set +a
    echo "✓ Vault secrets loaded"
fi

# Add to .bashrc for future SSH sessions (if not already added)
if [ -f ".vault-secrets" ] && ! grep -q ".vault-secrets" ~/.bashrc 2>/dev/null; then
    echo "" >> ~/.bashrc
    echo "# Auto-source Vault secrets on shell startup" >> ~/.bashrc
    echo 'if [ -f .vault-secrets ]; then' >> ~/.bashrc
    echo '    set -a' >> ~/.bashrc
    echo '    source .vault-secrets 2>/dev/null' >> ~/.bashrc
    echo '    set +a' >> ~/.bashrc
    echo 'fi' >> ~/.bashrc
    echo "✓ Added Vault secrets auto-load to ~/.bashrc"
fi

# Continue with your normal setup...
pip install -r requirements.txt
