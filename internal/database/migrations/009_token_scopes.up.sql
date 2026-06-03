ALTER TABLE access_tokens ADD COLUMN org_id BIGINT REFERENCES organizations(id) ON DELETE CASCADE;
CREATE INDEX idx_access_tokens_org ON access_tokens(org_id);
