BEGIN;
  ALTER TABLE containers ADD COLUMN runtime_lifecycle boolean DEFAULT false;
COMMIT;
