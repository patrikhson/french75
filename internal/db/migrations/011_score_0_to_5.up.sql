-- Change score range from 0–100 to 0–5.
-- Existing scores are reset to 0; set them manually via the edit form.
ALTER TABLE check_ins DROP CONSTRAINT IF EXISTS check_ins_score_check;
UPDATE check_ins SET score = 0;
ALTER TABLE check_ins ADD CONSTRAINT check_ins_score_check CHECK (score >= 0 AND score <= 5);
