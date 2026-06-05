DROP INDEX IF EXISTS idx_agents_custom_domain;
ALTER TABLE agents DROP COLUMN IF EXISTS domain_verify_token;
ALTER TABLE agents DROP COLUMN IF EXISTS domain_verified_at;
ALTER TABLE agents DROP COLUMN IF EXISTS domain_enabled;
ALTER TABLE agents DROP COLUMN IF EXISTS custom_domain;
