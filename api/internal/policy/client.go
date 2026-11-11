package policy

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

// DefaultOPABaseURL is used when OPA_URL is not set.
const DefaultOPABaseURL = "http://localhost:8181"

// decisionPath is the OPA data API path for data.fractal.guard.decision.
const decisionPath = "/v1/data/fractal/guard/decision"

// maxResponseBytes bounds an OPA response read.
const maxResponseBytes = 1 << 20

// validOutcomes are the outcomes guard.rego can produce. Anything else from
// OPA is treated as malformed (fail closed).
var validOutcomes = map[string]bool{
	"ALLOW":            true,
	"BLOCK":            true,
	"WAIT":             true,
	"REQUIRE_APPROVAL": true,
}

// Decision mirrors the object returned by data.fractal.guard.decision.
type Decision struct {
	Outcome           string         `json:"outcome"`
	Action            *string        `json:"action"`
	Findings          []Finding      `json:"findings"`
	MissingGates      []string       `json:"missing_gates"`
	RequiredApprovals []string       `json:"required_approvals"`
	Policy            map[string]any `json:"policy"`
}

// Finding mirrors lib.finding / lib.gate objects (extra keys are optional).
type Finding struct {
	RuleID       string `json:"rule_id"`
	Outcome      string `json:"outcome"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message,omitempty"`
	Gate         string `json:"gate,omitempty"`
	ApproverRole string `json:"approver_role,omitempty"`
}

// FailClosed converts any evaluation failure into a BLOCK decision. The
// cause is returned alongside for logging; the decision itself is always
// safe to act on because BLOCK never authorizes anything.
func FailClosed(cause error) (Decision, error) {
	return Decision{
		Outcome: "BLOCK",
		Findings: []Finding{{
			RuleID:  "POL-FAIL",
			Outcome: "BLOCK",
			Code:    "POLICY_UNAVAILABLE",
			Message: fmt.Sprintf("policy evaluation failed closed: %v", cause),
		}},
		MissingGates:      []string{},
		RequiredApprovals: []string{},
	}, cause
}

// Client evaluates OPA inputs against data.fractal.guard.decision.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client for the given OPA base URL ("" -> default).
// The HTTP timeout bounds how long a decision may take before failing closed.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultOPABaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// Evaluate POSTs the input to OPA's data API. Failures — unreachable OPA,
// non-200, malformed body, missing/invalid result — return a fail-closed
// BLOCK decision with a non-nil error for logging. A nil error means the
// decision came from OPA unchanged.
func (c *Client) Evaluate(ctx context.Context, in Input) (Decision, error) {
	body, err := json.Marshal(map[string]any{"input": in})
	if err != nil {
		return FailClosed(fmt.Errorf("marshal input: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+decisionPath, bytes.NewReader(body))
	if err != nil {
		return FailClosed(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return FailClosed(fmt.Errorf("opa unreachable: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return FailClosed(fmt.Errorf("read response: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return FailClosed(fmt.Errorf("opa status %d", resp.StatusCode))
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return FailClosed(fmt.Errorf("malformed response: %w", err))
	}
	if len(envelope.Result) == 0 {
		return FailClosed(errors.New("response missing result"))
	}
	var d Decision
	if err := json.Unmarshal(envelope.Result, &d); err != nil {
		return FailClosed(fmt.Errorf("malformed result: %w", err))
	}
	if !validOutcomes[d.Outcome] {
		return FailClosed(fmt.Errorf("invalid outcome %q", d.Outcome))
	}
	// Normalize list sections so callers never see null arrays.
	if d.Findings == nil {
		d.Findings = []Finding{}
	}
	if d.MissingGates == nil {
		d.MissingGates = []string{}
	}
	if d.RequiredApprovals == nil {
		d.RequiredApprovals = []string{}
	}
	return d, nil
}
