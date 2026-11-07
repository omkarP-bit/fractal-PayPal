DROP TRIGGER IF EXISTS trg_audit_immutable ON audit_events;
DROP TRIGGER IF EXISTS trg_decisions_immutable ON policy_decisions;
DROP TRIGGER IF EXISTS trg_evidence_immutable ON case_evidence;
DROP FUNCTION IF EXISTS forbid_mutation;
