CREATE TABLE IF NOT EXISTS vault (
	chat_id BIGINT PRIMARY KEY,
	owner   TEXT NOT NULL,
	repo    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS alias (
	id     UUID PRIMARY KEY,
	chat_id BIGINT NOT NULL,
	path   TEXT NOT NULL,
	path_type TEXT NOT NULL,
	alias  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alias_chat_id ON alias (chat_id);

CREATE TABLE IF NOT EXISTS template (
	id         UUID PRIMARY KEY,
	chat_id    BIGINT NOT NULL,
	name       TEXT NOT NULL,
	value      TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_template_chat_id ON template (chat_id);
