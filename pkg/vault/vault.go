package vault

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/hashicorp/vault/api"
)

// Client wraps the Vault API client
type Client struct {
	client *api.Client
}

// CSISecrets contains the credentials needed for Ceph CSI volume operations
type CSISecrets struct {
	UserID  string
	UserKey string
}

// NewClient creates a new Vault client
func NewClient(addr string, namespace string) (*Client, error) {
	config := api.DefaultConfig()
	config.Address = addr

	// Configure TLS to skip verification (for self-signed certs in dev environments)
	// In production, you should configure proper CA certificates
	config.HttpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	if namespace != "" {
		client.SetNamespace(namespace)
	}

	return &Client{client: client}, nil
}

// SetToken sets the Vault token for authentication
func (c *Client) SetToken(token string) {
	c.client.SetToken(token)
}

// ReadCSISecrets reads CSI credentials from a Vault KV v2 path
// The secret should contain "userID" and "userKey" fields
func (c *Client) ReadCSISecrets(path string) (*CSISecrets, error) {
	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret from %s: %w", path, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("no secret found at path %s", path)
	}

	// For KV v2, data is nested under "data"
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		// Try direct access for KV v1
		data = secret.Data
	}

	userID, ok := data["userID"].(string)
	if !ok {
		return nil, fmt.Errorf("secret at %s missing 'userID' field", path)
	}

	userKey, ok := data["userKey"].(string)
	if !ok {
		return nil, fmt.Errorf("secret at %s missing 'userKey' field", path)
	}

	return &CSISecrets{
		UserID:  userID,
		UserKey: userKey,
	}, nil
}

// GetNomadToken attempts to get a Vault token using Nomad workload identity
// This uses the VAULT_TOKEN environment variable if set, otherwise returns empty
func GetTokenFromEnv() string {
	// Check for explicit token first
	config := api.DefaultConfig()
	if config.Error != nil {
		return ""
	}
	// The DefaultConfig reads VAULT_TOKEN from environment
	client, err := api.NewClient(config)
	if err != nil {
		return ""
	}
	return client.Token()
}
