-- Remove duplicate services (keep newest id per panel_id + panel_username).
DELETE FROM services s1
USING services s2
WHERE s1.panel_id = s2.panel_id
  AND s1.panel_username = s2.panel_username
  AND s1.panel_username != ''
  AND s1.id < s2.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_services_panel_username
  ON services (panel_id, panel_username)
  WHERE panel_username != '';
