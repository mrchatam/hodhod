ALTER TABLE services ADD COLUMN IF NOT EXISTS contact TEXT NOT NULL DEFAULT '';

UPDATE services s SET contact = c.contact
FROM customers c
WHERE s.customer_id = c.id AND c.contact != '' AND s.contact = '';
