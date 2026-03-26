CREATE TABLE follow_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (requester_id, target_id),
    CHECK (requester_id != target_id)
);

CREATE INDEX idx_follow_requests_target_id ON follow_requests(target_id);
CREATE INDEX idx_follow_requests_requester_id ON follow_requests(requester_id);
