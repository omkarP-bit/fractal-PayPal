package policy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- input builder ---------------------------------------------------------

func TestBuildAnomalyScoreDisabledIsNull(t *testing.T) {
	high := 0.97
	if got := BuildAnomalyScore(false, &high); got != nil {
		t.Fatalf("disabled gate must yield nil (null), got %v", *got)
	}
}

func TestBuildAnomalyScoreEnabledPassesValidScore(t *testing.T) {
	score := 0.85
	got := BuildAnomalyScore(true, &score)
	if got == nil || *got != 0.85 {
		t.Fatalf("enabled gate must pass the score, got %v", got)
	}
}

func TestBuildAnomalyScoreUnscoredIsNull(t *testing.T) {
	if got := BuildAnomalyScore(true, nil); got != nil {
		t.Fatalf("nil score must stay nil, got %v", *got)
	}
}

func TestBuildAnomalyScoreOutOfRangeIsNull(t *testing.T) {
	for _, v := range []float64{-0.1, 1.5, 100} {
		s := v
		if got := BuildAnomalyScore(true, &s); got != nil {
			t.Errorf("score %v out of [0,1] must be treated as unscored, got %v", v, *got)
		}
	}
}

func TestApplyAnomalyScoreOverridesCallerScoreWhenDisabled(t *testing.T) {
	in := NewInput()
	in.Risk.Severity = "MEDIUM"
	smuggled := 0.99
	out := ApplyAnomalyScore(in, false, &smuggled)
	if out.Risk.AnomalyScore != nil {
		t.Fatal("disabled server gate must null out a caller-supplied score")
	}
}

func TestApplyAnomalyScoreNormalizesDocument(t *testing.T) {
	in := Input{} // fully empty document
	out := ApplyAnomalyScore(in, true, nil)
	if out.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %q, want %q", out.SchemaVersion, SchemaVersion)
	}
	if out.Approvals == nil || out.Applicability == nil {
		t.Error("list sections must be [] not null")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"anomaly_score":null`) {
		t.Errorf("null score must serialize explicitly, got %s", raw)
	}
}

func TestRiskSerializesScoreAsNumber(t *testing.T) {
	score := 0.9
	raw, err := json.Marshal(BuildRisk(true, "MEDIUM", &score))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"anomaly_score":0.9`) {
		t.Errorf("score must serialize as a number, got %s", raw)
	}
}

// --- fail-closed client ----------------------------------------------------

// okOPA returns a fixed ALLOW decision.
func okOPA(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != decisionPath {
			t.Errorf("unexpected OPA path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"outcome":"ALLOW","action":"RECONCILE_MERCHANT_ORDER",`+
			`"findings":[],"missing_gates":[],"required_approvals":[],"policy":{"revision":"2026-11-07.1"}}}`)
	}))
}

func testInput() Input {
	in := NewInput()
	in.Request = Request{
		EvaluationID: "11111111-1111-1111-1111-111111111111",
		EvaluatedAt:  "2026-11-07T10:00:00Z",
		Action:       Action{Type: "RECONCILE_MERCHANT_ORDER", Params: map[string]any{"amount_minor": 4900}},
	}
	in.Risk = Risk{Severity: "MEDIUM"}
	return in
}

func TestClientPassesOPADecisionThrough(t *testing.T) {
	srv := okOPA(t)
	defer srv.Close()

	c := NewClient(srv.URL)
	d, err := c.Evaluate(context.Background(), testInput())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.Outcome != "ALLOW" {
		t.Fatalf("outcome = %q, want ALLOW", d.Outcome)
	}
	if d.Findings == nil || d.MissingGates == nil || d.RequiredApprovals == nil {
		t.Error("list sections must be normalized to [] (never null)")
	}
}

func TestClientFailsClosedWhenOPAUnreachable(t *testing.T) {
	srv := okOPA(t)
	url := srv.URL
	srv.Close() // server gone

	c := NewClient(url)
	d, err := c.Evaluate(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error when OPA is down")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want fail-closed BLOCK", d.Outcome)
	}
	if !hasRule(d, "POL-FAIL") {
		t.Fatalf("expected POL-FAIL finding, got %+v", d.Findings)
	}
}

func TestClientFailsClosedOnHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, err := NewClient(srv.URL).Evaluate(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error on HTTP 500")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want BLOCK", d.Outcome)
	}
}

func TestClientFailsClosedOnMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "this is not json")
	}))
	defer srv.Close()

	d, err := NewClient(srv.URL).Evaluate(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error on malformed body")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want BLOCK", d.Outcome)
	}
}

func TestClientFailsClosedOnMissingResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":null}`)
	}))
	defer srv.Close()

	d, err := NewClient(srv.URL).Evaluate(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error on missing result")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want BLOCK", d.Outcome)
	}
}

func TestClientFailsClosedOnInvalidOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":{"outcome":"MAYBE"}}`)
	}))
	defer srv.Close()

	d, err := NewClient(srv.URL).Evaluate(context.Background(), testInput())
	if err == nil {
		t.Fatal("expected an error on invalid outcome")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want BLOCK", d.Outcome)
	}
}

func TestFailClosedNeverAllows(t *testing.T) {
	d, err := FailClosed(context.DeadlineExceeded)
	if err == nil {
		t.Fatal("FailClosed must return the cause")
	}
	if d.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want BLOCK", d.Outcome)
	}
	if hasRule(d, "POL-FAIL") != true {
		t.Fatal("expected POL-FAIL finding")
	}
}

func hasRule(d Decision, ruleID string) bool {
	for _, f := range d.Findings {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

// --- handler ----------------------------------------------------------------

// recordingOPA captures the input document it received.
type recordingOPA struct {
	received map[string]any
	status   int
	body     string
}

func (rec *recordingOPA) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Input map[string]any `json:"input"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Errorf("OPA received unparseable input: %v", err)
		}
		rec.received = payload.Input
		if rec.status != 0 {
			w.WriteHeader(rec.status)
			_, _ = io.WriteString(w, rec.body)
			return
		}
		_, _ = io.WriteString(w, `{"result":{"outcome":"ALLOW","findings":[],`+
			`"missing_gates":[],"required_approvals":[]}}`)
	}))
}

func newTestHandler(opaURL string, enabled bool) *Handler {
	return &Handler{
		Client:         NewClient(opaURL),
		AnomalyEnabled: enabled,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func postEvaluate(t *testing.T, h *Handler, body string) EvaluateResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/policy/evaluate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Evaluate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp EvaluateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v (%s)", err, rec.Body.String())
	}
	return resp
}

func anomalyScoreOf(t *testing.T, input map[string]any) any {
	t.Helper()
	risk, ok := input["risk"].(map[string]any)
	if !ok {
		t.Fatalf("input has no risk section: %v", input)
	}
	score, ok := risk["anomaly_score"]
	if !ok {
		t.Fatal("risk section missing anomaly_score key (must be present, null when unscored)")
	}
	return score
}

func TestHandlerDisabledSendsNullScoreToOPA(t *testing.T) {
	opa := &recordingOPA{}
	srv := opa.server(t)
	defer srv.Close()

	h := newTestHandler(srv.URL, false)
	body := `{"input":{"request":{"action":{"type":"RECONCILE_MERCHANT_ORDER"}},"risk":{"severity":"MEDIUM"}},` +
		`"anomaly_score":0.99}`
	resp := postEvaluate(t, h, body)

	if resp.Decision.Outcome != "ALLOW" {
		t.Fatalf("outcome = %q", resp.Decision.Outcome)
	}
	if got := anomalyScoreOf(t, opa.received); got != nil {
		t.Fatalf("disabled handler sent anomaly_score=%v to OPA, want null", got)
	}
}

func TestHandlerEnabledSendsScoreToOPA(t *testing.T) {
	opa := &recordingOPA{}
	srv := opa.server(t)
	defer srv.Close()

	h := newTestHandler(srv.URL, true)
	body := `{"input":{"request":{"action":{"type":"RECONCILE_MERCHANT_ORDER"}},"risk":{"severity":"MEDIUM"}},` +
		`"anomaly_score":0.97}`
	_ = postEvaluate(t, h, body)

	got := anomalyScoreOf(t, opa.received)
	score, ok := got.(float64)
	if !ok || score != 0.97 {
		t.Fatalf("enabled handler sent anomaly_score=%v, want 0.97", got)
	}
}

func TestHandlerEnabledWithoutScoreSendsNull(t *testing.T) {
	opa := &recordingOPA{}
	srv := opa.server(t)
	defer srv.Close()

	h := newTestHandler(srv.URL, true)
	body := `{"input":{"request":{"action":{"type":"RECONCILE_MERCHANT_ORDER"}},"risk":{"severity":"MEDIUM"}}}`
	_ = postEvaluate(t, h, body)

	if got := anomalyScoreOf(t, opa.received); got != nil {
		t.Fatalf("unscored request sent anomaly_score=%v, want null", got)
	}
}

func TestHandlerOPADownReturnsBlockDecision(t *testing.T) {
	srv := okOPA(t)
	url := srv.URL
	srv.Close()

	h := newTestHandler(url, true)
	body := `{"input":{"request":{"action":{"type":"RECONCILE_MERCHANT_ORDER"}},"risk":{"severity":"MEDIUM"}}}`
	resp := postEvaluate(t, h, body)

	if resp.Decision.Outcome != "BLOCK" {
		t.Fatalf("outcome = %q, want fail-closed BLOCK when OPA is down", resp.Decision.Outcome)
	}
	if !hasRule(resp.Decision, "POL-FAIL") {
		t.Fatalf("expected POL-FAIL finding, got %+v", resp.Decision.Findings)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	h := newTestHandler("", false)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{nope"))
	rec := httptest.NewRecorder()
	h.Evaluate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandlerRejectsMissingActionType(t *testing.T) {
	h := newTestHandler("", false)
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"input":{}}`))
	rec := httptest.NewRecorder()
	h.Evaluate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing action.type", rec.Code)
	}
}

func TestHandlerRejectsNonPost(t *testing.T) {
	h := newTestHandler("", false)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.Evaluate(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
