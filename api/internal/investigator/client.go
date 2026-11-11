package investigator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrDisabled is returned when no investigator URL is configured.
var ErrDisabled = errors.New("investigator is not configured")

// Request is the investigation request sent to the Python service.
type Request struct {
	CaseID              string `json:"case_id"`
	RunNumber           int    `json:"run_number"`
	CaseType            string `json:"case_type"`
	CustomerDescription string `json:"customer_description"`
}

// Submission is the advisory output. Go verifies it before persisting.
type Submission struct {
	Category              string   `json:"category"`
	RootCauseHypothesis   string   `json:"root_cause_hypothesis"`
	EvidenceIDs           []string `json:"evidence_ids"`
	MissingEvidence       []string `json:"missing_evidence"`
	Recommendation        string   `json:"recommendation"`
	SuggestedSeverity     string   `json:"suggested_severity"`
	CustomerExplanation   string   `json:"customer_explanation"`
	MerchantFixSuggestion string   `json:"merchant_fix_suggestion"`
	AgentVersion          string   `json:"agent_version"`
	ModelID               string   `json:"model_id"`
}

// Client calls the investigator service over the private network.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

// Investigate sends verified facts to the investigator. The service only
// advises; a non-nil error means the caller must carry on without it.
func (c *Client) Investigate(ctx context.Context, req Request) (*Submission, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal investigation request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/investigate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build investigation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("X-Fractal-Internal-Token", c.token)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("investigator unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read investigation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("investigator status %d: %s", resp.StatusCode, string(raw))
	}

	var envelope struct {
		Status     string      `json:"status"`
		Submission *Submission `json:"submission"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("malformed investigation response: %w", err)
	}
	if envelope.Status != "submitted" || envelope.Submission == nil {
		return nil, fmt.Errorf("investigation was not submitted")
	}
	return envelope.Submission, nil
}
