package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

type Session struct {
	ID        string `json:"id"`
	UserID    int64  `json:"userId"`
	PersonaID int64  `json:"personaId"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Message struct {
	ID        int64    `json:"id"`
	SessionID string   `json:"sessionId,omitempty"`
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Kind      string   `json:"kind"`
	SQL       string   `json:"sql"`
	Tables    []string `json:"tables"`
	RowCount  *int     `json:"rowCount"`
	Truncated bool     `json:"truncated"`
	Reasoning string   `json:"reasoning"`
	CreatedAt string   `json:"createdAt"`
}

func (s *Store) CreateSession(ctx context.Context, userID, personaID int64, id, title string) (Session, error) {
	now := Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO chat_session (id, user_id, persona_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, personaID, title, now, now)
	if err != nil {
		return Session{}, err
	}
	return s.Session(ctx, id)
}

func (s *Store) Session(ctx context.Context, id string) (Session, error) {
	var sess Session
	err := s.DB.QueryRowContext(ctx, `SELECT id, user_id, persona_id, title, created_at, updated_at FROM chat_session WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.PersonaID, &sess.Title, &sess.CreatedAt, &sess.UpdatedAt)
	return sess, err
}

func (s *Store) ListSessions(ctx context.Context, userID, personaID int64) ([]Session, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, persona_id, title, created_at, updated_at FROM chat_session
		WHERE user_id = ? AND persona_id = ? ORDER BY updated_at DESC, id DESC`, userID, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.PersonaID, &sess.Title, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSession(ctx context.Context, id string, userID int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM chat_session WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_message WHERE session_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Messages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, session_id, role, content, kind, sql_text, tables_used, row_count, truncated, reasoning, created_at
		FROM chat_message WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddExchange(ctx context.Context, sessionID, title string, userMsg, assistant Message) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := Now()
	if _, err := insertMessage(ctx, tx, sessionID, "USER", userMsg, now); err != nil {
		return err
	}
	if _, err := insertMessage(ctx, tx, sessionID, "ASSISTANT", assistant, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE chat_session SET title = ?, updated_at = ? WHERE id = ?`, title, now, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertMessage(ctx context.Context, tx *sql.Tx, sessionID, role string, m Message, now string) (int64, error) {
	if m.Tables == nil {
		m.Tables = []string{}
	}
	tables, err := json.Marshal(m.Tables)
	if err != nil {
		return 0, err
	}
	var rowCount any
	if m.RowCount != nil {
		rowCount = *m.RowCount
	}
	trunc := 0
	if m.Truncated {
		trunc = 1
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO chat_message
		(session_id, role, content, kind, sql_text, tables_used, row_count, truncated, reasoning, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, role, m.Content, m.Kind, m.SQL, string(tables), rowCount, trunc, m.Reasoning, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func scanMessage(row scanner) (Message, error) {
	var m Message
	var tables string
	var rowCount sql.NullInt64
	var trunc int
	err := row.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Kind, &m.SQL, &tables, &rowCount, &trunc, &m.Reasoning, &m.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	m.Truncated = trunc == 1
	if rowCount.Valid {
		n := int(rowCount.Int64)
		m.RowCount = &n
	}
	_ = json.Unmarshal([]byte(tables), &m.Tables)
	if m.Tables == nil {
		m.Tables = []string{}
	}
	return m, nil
}
