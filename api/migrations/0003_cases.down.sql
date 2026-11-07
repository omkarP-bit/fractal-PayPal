DROP INDEX IF EXISTS ix_messages_case;
DROP INDEX IF EXISTS ix_cases_customer;
DROP INDEX IF EXISTS ix_cases_merchant;
DROP INDEX IF EXISTS uq_live_case;
DROP TABLE IF EXISTS case_messages;
DROP TABLE IF EXISTS exception_cases;
DROP SEQUENCE IF EXISTS case_number_seq;
