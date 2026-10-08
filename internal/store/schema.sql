CREATE TABLE IF NOT EXISTS sys_user (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('ADMIN', 'USER')),
  display_name  TEXT NOT NULL DEFAULT '',
  dept          TEXT NOT NULL DEFAULT '',
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS datasource (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  file_name  TEXT NOT NULL,
  status     INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS persona (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  system_prompt TEXT NOT NULL DEFAULT '',
  prohibitions  TEXT NOT NULL DEFAULT '',
  row_filter    TEXT NOT NULL DEFAULT '',
  datasource_id INTEGER REFERENCES datasource(id),
  status        INTEGER NOT NULL DEFAULT 1,
  catalog_text  TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS persona_table_grant (
  persona_id INTEGER NOT NULL,
  table_name TEXT NOT NULL,
  PRIMARY KEY (persona_id, table_name)
);

CREATE TABLE IF NOT EXISTS persona_role_grant (
  persona_id INTEGER NOT NULL,
  role_code  TEXT NOT NULL,
  PRIMARY KEY (persona_id, role_code)
);

CREATE TABLE IF NOT EXISTS persona_user_grant (
  persona_id INTEGER NOT NULL,
  user_id    INTEGER NOT NULL,
  PRIMARY KEY (persona_id, user_id)
);

CREATE TABLE IF NOT EXISTS table_meta (
  datasource_id INTEGER NOT NULL,
  table_name    TEXT NOT NULL,
  custom_comment TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  is_hidden     INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (datasource_id, table_name)
);

CREATE TABLE IF NOT EXISTS column_meta (
  datasource_id INTEGER NOT NULL,
  table_name    TEXT NOT NULL,
  column_name   TEXT NOT NULL,
  custom_comment TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  is_sensitive  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (datasource_id, table_name, column_name)
);

CREATE TABLE IF NOT EXISTS persona_metric (
  id         INTEGER PRIMARY KEY,
  persona_id INTEGER NOT NULL,
  name       TEXT NOT NULL,
  definition TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS persona_example (
  id         INTEGER PRIMARY KEY,
  persona_id INTEGER NOT NULL,
  question   TEXT NOT NULL,
  sql_text   TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS chat_session (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL,
  persona_id INTEGER NOT NULL,
  title      TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chat_message (
  id          INTEGER PRIMARY KEY,
  session_id  TEXT NOT NULL,
  role        TEXT NOT NULL CHECK (role IN ('USER', 'ASSISTANT')),
  content     TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT '',
  sql_text    TEXT NOT NULL DEFAULT '',
  tables_used TEXT NOT NULL DEFAULT '[]',
  row_count   INTEGER,
  truncated   INTEGER NOT NULL DEFAULT 0,
  reasoning   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chat_session_user ON chat_session(user_id, persona_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_chat_message_session ON chat_message(session_id, id);
