// Package policy wires the Rego guard (policies/) into the API: it builds
// the OPA input document (schema 1.0, see policies/tests/fixtures.rego),
// evaluates it through OPA's data API, and fails closed — any evaluation
// failure yields a BLOCK decision, never ALLOW.
package policy

// SchemaVersion is the supported input schema version
// (data.rego config.supported_input_versions = ["1.0"]).
const SchemaVersion = "1.0"

// Input mirrors the Phase 10/11 OPA input document. Field names match the
// Rego fixtures exactly so tests and runtime share one shape.
type Input struct {
	SchemaVersion string    `json:"schema_version"`
	Request       Request   `json:"request"`
	Case          Case      `json:"case"`
	Actor         Actor     `json:"actor"`
	Merchant      Merchant  `json:"merchant"`
	Facts         Facts     `json:"facts"`
	Execution     Execution `json:"execution"`
	Risk          Risk      `json:"risk"`
	Approvals     []any     `json:"approvals"`
	Applicability []any     `json:"applicability"`
	Advisory      any       `json:"advisory,omitempty"`
}

// Request is the action being evaluated.
type Request struct {
	EvaluationID string `json:"evaluation_id"`
	EvaluatedAt  string `json:"evaluated_at"`
	Action       Action `json:"action"`
}

// Action is the requested operation plus integrity binds.
type Action struct {
	Type           string         `json:"type"`
	Params         map[string]any `json:"params"`
	ParamsSHA256   string         `json:"params_sha256"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// Case identifies the exception case under evaluation.
type Case struct {
	ID         string `json:"id"`
	CaseNumber string `json:"case_number"`
	Status     string `json:"status"`
}

// Actor is the requesting principal.
type Actor struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	MerchantID  string   `json:"merchant_id"`
	Permissions []string `json:"permissions"`
}

// Merchant is the tenant the decision applies to.
type Merchant struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}

// Facts are verified provider/order/webhook observations.
type Facts struct {
	Provider ProviderFact `json:"provider"`
	Order    OrderFact    `json:"order"`
	Webhook  WebhookFact  `json:"webhook"`
}

// ProviderFact is the provider-side truth.
type ProviderFact struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Source      string `json:"source"`
	FetchedAt   string `json:"fetched_at"`
}

// OrderFact is the merchant order snapshot.
type OrderFact struct {
	Status      string `json:"status"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Source      string `json:"source"`
	FetchedAt   string `json:"fetched_at"`
}

// WebhookFact carries signature verification state.
type WebhookFact struct {
	SignatureVerified bool `json:"signature_verified"`
	EventCount        int  `json:"event_count"`
}

// Execution guards against double-execution.
type Execution struct {
	AlreadyExecuted bool `json:"already_executed"`
}

// Risk is the operational risk section. AnomalyScore is the Phase 11
// tighten-only signal: it is JSON null unless the model is enabled AND a
// valid score exists (see ApplyAnomalyScore).
type Risk struct {
	Severity     string   `json:"severity"`
	AnomalyScore *float64 `json:"anomaly_score"` // null == Phase 10 behavior
}

// NewInput returns an empty schema-1.0 input with list sections present as
// [] (not null) so the document matches the Phase 10 fixtures.
func NewInput() Input {
	return Input{
		SchemaVersion: SchemaVersion,
		Approvals:     []any{},
		Applicability: []any{},
	}
}

// BuildAnomalyScore implements the Phase 11 enable gate:
//
//   - disabled (default): nil -> JSON null, identical to Phase 10 (invariant 1);
//   - enabled + nil score: null ("unscored", invariant 2);
//   - enabled + score outside [0,1]: null — an out-of-range value is corrupt
//     data and is treated as unscored rather than trusted.
func BuildAnomalyScore(enabled bool, score *float64) *float64 {
	if !enabled || score == nil {
		return nil
	}
	if *score < 0 || *score > 1 {
		return nil
	}
	s := *score
	return &s
}

// BuildRisk constructs the risk section under the same gate.
func BuildRisk(enabled bool, severity string, score *float64) Risk {
	return Risk{Severity: severity, AnomalyScore: BuildAnomalyScore(enabled, score)}
}

// ApplyAnomalyScore returns a copy of in with risk.anomaly_score set by the
// gate above — the server-side switch is authoritative, so a caller cannot
// smuggle a score in while the model is disabled. It also normalizes the
// document (schema version, empty lists) to the fixture shape.
func ApplyAnomalyScore(in Input, enabled bool, score *float64) Input {
	if in.SchemaVersion == "" {
		in.SchemaVersion = SchemaVersion
	}
	if in.Approvals == nil {
		in.Approvals = []any{}
	}
	if in.Applicability == nil {
		in.Applicability = []any{}
	}
	in.Risk.AnomalyScore = BuildAnomalyScore(enabled, score)
	return in
}
