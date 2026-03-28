-- Change score range from 0–100 to 0–5.
-- Existing scores are set to their manually confirmed 0–5 values.
ALTER TABLE check_ins DROP CONSTRAINT IF EXISTS check_ins_score_check;
UPDATE check_ins SET score = 1 WHERE id = '5ec697ec-5c36-4e9f-acbb-42e7b574fb14'; -- French 75, 2026-01-31
UPDATE check_ins SET score = 3 WHERE id = 'ffc12fcb-bc6d-4afe-b577-1d912d540a91'; -- French 75, 2026-03-12
UPDATE check_ins SET score = 5 WHERE id = '718c8fcf-0578-41c0-8a47-683c527ce0d1'; -- French 75, 2026-03-21
UPDATE check_ins SET score = 5 WHERE id = 'b9157c5e-dd4c-4f2e-91d3-a0a5c6f2c354'; -- French 75, 2026-03-22
UPDATE check_ins SET score = 0 WHERE id = 'd83761b0-3109-45b2-ad0a-2b6677c9384f'; -- French 75, 2026-03-25
UPDATE check_ins SET score = 4 WHERE id = 'fdae7cc5-8b76-4ee2-915c-63b72618ed6c'; -- French 75, 2026-03-27
UPDATE check_ins SET score = 3 WHERE id = 'd35cd597-9d45-4869-a80b-fe9bac22a4f7'; -- Negroni,   2026-03-27
ALTER TABLE check_ins ADD CONSTRAINT check_ins_score_check CHECK (score >= 0 AND score <= 5);
