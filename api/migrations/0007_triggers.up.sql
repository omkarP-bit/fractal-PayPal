CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% on % is not allowed', TG_OP, TG_TABLE_NAME;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_evidence_immutable') THEN
    EXECUTE 'CREATE TRIGGER trg_evidence_immutable BEFORE UPDATE OR DELETE ON case_evidence FOR EACH ROW EXECUTE FUNCTION forbid_mutation()';
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_decisions_immutable') THEN
    EXECUTE 'CREATE TRIGGER trg_decisions_immutable BEFORE UPDATE OR DELETE ON policy_decisions FOR EACH ROW EXECUTE FUNCTION forbid_mutation()';
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_audit_immutable') THEN
    EXECUTE 'CREATE TRIGGER trg_audit_immutable BEFORE UPDATE OR DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION forbid_mutation()';
  END IF;
END $$;
