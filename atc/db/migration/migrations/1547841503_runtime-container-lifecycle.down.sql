BEGIN;
  ALTER TABLE containers DROP COLUMN runtime_lifecycle;
COMMIT;
