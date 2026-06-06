ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_scope_check;
ALTER TABLE settings ADD CONSTRAINT settings_scope_check
    CHECK (scope IN ('master', 'agent', 'bot'));
