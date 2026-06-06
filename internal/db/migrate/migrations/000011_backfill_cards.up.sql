INSERT INTO bot_payment_cards (bot_id, label, card_number, holder_name, active)
SELECT s.scope_id, trim(line), trim(line), '', true
FROM settings s,
     LATERAL regexp_split_to_table(s.value, E'\\n') AS line
WHERE s.scope = 'bot' AND s.key = 'card_numbers' AND trim(line) <> ''
  AND NOT EXISTS (SELECT 1 FROM bot_payment_cards c WHERE c.bot_id = s.scope_id);
