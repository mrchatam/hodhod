# Migration 000014 — `services_panel_username_unique`

## Pre-deploy audit (required)

Run against production **before** applying the migration. Review every duplicate group; the migration keeps the row with the **highest** `id` and deletes the rest (irreversible).

```sql
SELECT panel_id, panel_username, COUNT(*) AS cnt, array_agg(id ORDER BY id) AS ids
FROM services
WHERE panel_username != ''
GROUP BY panel_id, panel_username
HAVING COUNT(*) > 1
ORDER BY cnt DESC;
```

For each duplicate group, verify which row should survive (check `order_id`, `bot_id`, `agent_id`, `created_at`). Manually merge or delete rows if the auto-kept row (max `id`) is not the canonical one.

Take a full backup of `services` (or the whole database) before migrate.

## What the migration does

1. `DELETE` duplicate rows per `(panel_id, panel_username)`, keeping `id = MAX(id)`.
2. `CREATE UNIQUE INDEX idx_services_panel_username ON services (panel_id, panel_username) WHERE panel_username != ''`.

## Rollout

- **Zero-downtime:** Not guaranteed. `DELETE` + index build may lock `services` on large tables.
- **Recommendation:** Apply during a low-traffic window or maintenance period.
- **After deploy:** Confirm no duplicate pairs remain with the audit query above (should return 0 rows).

## Rollback (`000014_services_panel_username_unique.down.sql`)

- Drops `idx_services_panel_username` only.
- **Does not restore** deleted duplicate rows. Restore from backup if dedupe was wrong.
