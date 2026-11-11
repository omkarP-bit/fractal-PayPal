package webapi

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"fractal/api/internal/investigator"
)

var validSeverities = map[string]bool{
	"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true,
}

// ReinvestigateCase refreshes evidence and runs the advisory investigator for a live case.
func (h *Handlers) ReinvestigateCase(ctx context.Context, caseID string) {
	if h.Investigator == nil || !h.Investigator.Enabled() {
		return
	}
	var caseType, orderRef, providerOrderID string
	err := h.DB.QueryRow(ctx, `
		SELECT c.case_type, c.merchant_order_ref, COALESCE(p.provider_order_id, '')
		FROM exception_cases c
		LEFT JOIN payment_transactions p ON p.id = c.payment_transaction_id
		WHERE c.id = $1`, caseID).Scan(&caseType, &orderRef, &providerOrderID)
	if err != nil {
		h.Log.Warn("reinvestigation lookup", "case_id", caseID, "err", err)
		return
	}
	defer func() {
		_, _ = h.DB.Exec(ctx, `
			UPDATE exception_cases SET status = 'OPEN', updated_at = now()
			WHERE id = $1 AND status = 'COLLECTING_EVIDENCE'`, caseID)
	}()
	if providerOrderID != "" && h.Collector != nil {
		if _, err := h.Collector.Collect(ctx, caseID, providerOrderID, orderRef); err != nil {
			h.Log.Warn("reinvestigation evidence collection", "case_id", caseID, "err", err)
		}
	}
	var description string
	_ = h.DB.QueryRow(ctx, `
		SELECT body FROM case_messages
		WHERE case_id = $1 AND author_type = 'CUSTOMER'
		ORDER BY created_at ASC LIMIT 1`, caseID).Scan(&description)
	h.runInvestigation(ctx, caseID, caseType, description)
}

// runInvestigation persists advisory findings only after verifying every evidence reference; failures never block the case.
func (h *Handlers) runInvestigation(parent context.Context, caseID, caseType, description string) {
	if h.Investigator == nil || !h.Investigator.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 20*time.Second)
	defer cancel()

	var runNumber int
	if err := h.DB.QueryRow(ctx, `
		SELECT COALESCE(MAX(run_number), 0) + 1 FROM investigations WHERE case_id = $1`, caseID).
		Scan(&runNumber); err != nil {
		h.Log.Warn("investigation run number", "case_id", caseID, "err", err)
		return
	}

	sub, err := h.Investigator.Investigate(ctx, investigator.Request{
		CaseID:              caseID,
		RunNumber:           runNumber,
		CaseType:            caseType,
		CustomerDescription: description,
	})
	if err != nil {
		h.Log.Warn("investigation skipped", "case_id", caseID, "err", err)
		return
	}

	category := strings.TrimSpace(sub.Category)
	hypothesis := strings.TrimSpace(sub.RootCauseHypothesis)
	if category == "" || len(category) > 100 || hypothesis == "" || len(hypothesis) > 2000 {
		h.Log.Warn("investigation rejected", "case_id", caseID, "reason", "schema")
		return
	}
	severity := strings.ToUpper(strings.TrimSpace(sub.SuggestedSeverity))
	if !validSeverities[severity] {
		severity = "MEDIUM"
	}
	recommendation := strings.TrimSpace(sub.Recommendation)
	if recommendation == "" {
		recommendation = "NO_ACTION"
	}

	verified := make([]string, 0, len(sub.EvidenceIDs))
	rejected := false
	for _, id := range sub.EvidenceIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) != 36 {
			rejected = true
			continue
		}
		var exists bool
		if err := h.DB.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM case_evidence WHERE case_id = $1 AND id = $2)`,
			caseID, id).Scan(&exists); err != nil {
			h.Log.Warn("investigation evidence check", "case_id", caseID, "err", err)
			return
		}
		if !exists {
			rejected = true
			continue
		}
		verified = append(verified, id)
	}

	validation := "SCHEMA_VALID"
	if rejected {
		validation = "REJECTED"
		verified = []string{}
		h.Log.Warn("investigation rejected", "case_id", caseID, "reason", "unknown evidence reference")
	} else if len(verified) > 0 {
		validation = "REFS_VERIFIED"
	}

	agentVersion := strings.TrimSpace(sub.AgentVersion)
	if agentVersion == "" {
		agentVersion = "investigator/1.0"
	}
	modelID := strings.TrimSpace(sub.ModelID)
	if modelID == "" {
		modelID = "unknown"
	}

	findings, err := json.Marshal(map[string]any{
		"category":                category,
		"root_cause_hypothesis":   hypothesis,
		"missing_evidence":        sub.MissingEvidence,
		"customer_explanation":    strings.TrimSpace(sub.CustomerExplanation),
		"merchant_fix_suggestion": strings.TrimSpace(sub.MerchantFixSuggestion),
	})
	if err != nil {
		h.Log.Warn("investigation encode", "case_id", caseID, "err", err)
		return
	}

	array := "{}"
	if len(verified) > 0 {
		array = "{" + strings.Join(verified, ",") + "}"
	}
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO investigations
			(case_id, run_number, agent_version, model_id, findings, recommendation,
			 suggested_severity, evidence_ids, validation_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::uuid[], $9)
		ON CONFLICT (case_id, run_number) DO NOTHING`,
		caseID, runNumber, agentVersion, modelID, string(findings), recommendation,
		severity, array, validation); err != nil {
		h.Log.Warn("investigation persist", "case_id", caseID, "err", err)
	}
}
