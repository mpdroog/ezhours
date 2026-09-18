package invoiced

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds the InvoiceD API configuration.
type Config struct {
	BaseURL string
	APIKey  string
	Entity  string
	Year    int
}

// DefaultConfig returns the default configuration for InvoiceD.
func DefaultConfig() Config {
	return Config{
		BaseURL: "http://localhost:9999",
		APIKey:  "mOh3LejvMRKgjFpq2Y9esa5+Y8FeB/s4DDmtwxT5oYA=",
		Entity:  "rootdev",
		Year:    time.Now().Year(),
	}
}

// Client is an HTTP client for the InvoiceD API.
type Client struct {
	config     Config
	httpClient *http.Client
}

// NewClient creates a new InvoiceD API client.
func NewClient(config Config) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExportHour sends a single Hour to the InvoiceD API.
func (c *Client) ExportHour(hour *Hour) error {
	url := fmt.Sprintf("%s/api/v1/hour/%s/%d/concept",
		c.config.BaseURL, c.config.Entity, c.config.Year)

	body, err := json.Marshal(hour)
	if err != nil {
		return fmt.Errorf("marshal hour: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ExportAll exports all hours to the InvoiceD API.
func (c *Client) ExportAll(hours []*Hour) error {
	for _, hour := range hours {
		if err := c.ExportHour(hour); err != nil {
			return fmt.Errorf("export %s: %w", hour.Name, err)
		}
	}
	return nil
}

// GetYear returns the configured year.
func (c *Client) GetYear() int {
	return c.config.Year
}

// GetEntity returns the configured entity.
func (c *Client) GetEntity() string {
	return c.config.Entity
}

// GetBaseURL returns the configured base URL.
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}
