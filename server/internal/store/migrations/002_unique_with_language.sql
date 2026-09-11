-- 002_unique_with_language.sql
-- The unique constraint on (left_hash, right_hash) ignored language, so
-- submitting the same content with a different parser returned the stale
-- result from the first language. Include language in the constraint.

ALTER TABLE jobs DROP CONSTRAINT unique_input_pair;
ALTER TABLE jobs ADD CONSTRAINT unique_input_pair UNIQUE (left_hash, right_hash, language);
