DROP INDEX IF EXISTS idx_agents_custom_domain;

ALTER TABLE agents ADD CONSTRAINT agents_custom_domain_key UNIQUE (custom_domain);
