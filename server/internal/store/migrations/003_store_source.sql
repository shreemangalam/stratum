-- 003_store_source.sql
-- Store submitted source code alongside the job so diff results are
-- accessible via permalink without requiring the client to resend them.

ALTER TABLE jobs ADD COLUMN left_source TEXT;
ALTER TABLE jobs ADD COLUMN right_source TEXT;
