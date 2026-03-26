ALTER TABLE notifications ADD COLUMN entity_id TEXT;
CREATE INDEX idx_notifications_entity ON notifications(type, entity_id) WHERE entity_id IS NOT NULL;
