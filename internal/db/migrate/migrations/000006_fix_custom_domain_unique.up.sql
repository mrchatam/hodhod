UPDATE agents SET custom_domain = NULL WHERE custom_domain = '' OR trim(custom_domain) = '';

ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_custom_domain_key;

DROP INDEX IF EXISTS idx_agents_custom_domain;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_custom_domain
    ON agents (custom_domain)
    WHERE custom_domain IS NOT NULL AND custom_domain <> '';
