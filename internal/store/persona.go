package store

import (
	"context"
	"database/sql"
)

type Persona struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"systemPrompt"`
	Prohibitions string `json:"prohibitions"`
	RowFilter    string `json:"rowFilter"`
	DatasourceID int64  `json:"datasourceId"`
	Status       int    `json:"status"`
	CatalogText  string `json:"catalogText"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type Metric struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	SortOrder  int    `json:"sortOrder"`
}

type Example struct {
	Question  string `json:"question"`
	SQL       string `json:"sql"`
	SortOrder int    `json:"sortOrder"`
}

func (s *Store) CreatePersona(ctx context.Context, p Persona) (Persona, error) {
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO persona
		(name, description, system_prompt, prohibitions, row_filter, datasource_id, status, catalog_text, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		p.Name, p.Description, p.SystemPrompt, p.Prohibitions, p.RowFilter, idOrNil(p.DatasourceID), p.Status, now, now)
	if err != nil {
		return Persona{}, err
	}
	id, _ := res.LastInsertId()
	return s.Persona(ctx, id)
}

func (s *Store) UpdatePersona(ctx context.Context, p Persona) (Persona, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE persona SET name = ?, description = ?, system_prompt = ?, prohibitions = ?,
		row_filter = ?, datasource_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		p.Name, p.Description, p.SystemPrompt, p.Prohibitions, p.RowFilter, idOrNil(p.DatasourceID), p.Status, Now(), p.ID)
	if err != nil {
		return Persona{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Persona{}, sql.ErrNoRows
	}
	return s.Persona(ctx, p.ID)
}

func (s *Store) DeletePersona(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM persona WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	for _, q := range []string{
		`DELETE FROM persona_table_grant WHERE persona_id = ?`,
		`DELETE FROM persona_role_grant WHERE persona_id = ?`,
		`DELETE FROM persona_user_grant WHERE persona_id = ?`,
		`DELETE FROM persona_metric WHERE persona_id = ?`,
		`DELETE FROM persona_example WHERE persona_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_message WHERE session_id IN (SELECT id FROM chat_session WHERE persona_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_session WHERE persona_id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListPersonas(ctx context.Context) ([]Persona, error) {
	rows, err := s.DB.QueryContext(ctx, personaSelect+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPersonas(rows)
}

func (s *Store) PersonasByDatasource(ctx context.Context, datasourceID int64) ([]Persona, error) {
	rows, err := s.DB.QueryContext(ctx, personaSelect+` WHERE datasource_id = ? ORDER BY id`, datasourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPersonas(rows)
}

func (s *Store) Persona(ctx context.Context, id int64) (Persona, error) {
	return scanPersona(s.DB.QueryRowContext(ctx, personaSelect+` WHERE id = ?`, id))
}

func (s *Store) SetCatalog(ctx context.Context, id int64, text string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE persona SET catalog_text = ?, updated_at = ? WHERE id = ?`, text, Now(), id)
	return err
}

func (s *Store) TableGrants(ctx context.Context, personaID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT table_name FROM persona_table_grant WHERE persona_id = ? ORDER BY table_name`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *Store) RoleGrants(ctx context.Context, personaID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT role_code FROM persona_role_grant WHERE persona_id = ? ORDER BY role_code`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *Store) UserGrants(ctx context.Context, personaID int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM persona_user_grant WHERE persona_id = ? ORDER BY user_id`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceTableGrants(ctx context.Context, personaID int64, tables []string) error {
	return s.replaceStrings(ctx, personaID, `DELETE FROM persona_table_grant WHERE persona_id = ?`,
		`INSERT INTO persona_table_grant (persona_id, table_name) VALUES (?, ?)`, tables)
}

func (s *Store) ReplaceRoleGrants(ctx context.Context, personaID int64, roles []string) error {
	return s.replaceStrings(ctx, personaID, `DELETE FROM persona_role_grant WHERE persona_id = ?`,
		`INSERT INTO persona_role_grant (persona_id, role_code) VALUES (?, ?)`, roles)
}

func (s *Store) ReplaceUserGrants(ctx context.Context, personaID int64, userIDs []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM persona_user_grant WHERE persona_id = ?`, personaID); err != nil {
		return err
	}
	for _, id := range userIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO persona_user_grant (persona_id, user_id) VALUES (?, ?)`, personaID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) replaceStrings(ctx context.Context, personaID int64, del, ins string, values []string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, del, personaID); err != nil {
		return err
	}
	for _, v := range values {
		if _, err := tx.ExecContext(ctx, ins, personaID, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Metrics(ctx context.Context, personaID int64) ([]Metric, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT name, definition, sort_order FROM persona_metric WHERE persona_id = ? ORDER BY sort_order, id`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Metric
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.Name, &m.Definition, &m.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Examples(ctx context.Context, personaID int64) ([]Example, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT question, sql_text, sort_order FROM persona_example WHERE persona_id = ? ORDER BY sort_order, id`, personaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Example
	for rows.Next() {
		var m Example
		if err := rows.Scan(&m.Question, &m.SQL, &m.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceMetrics(ctx context.Context, personaID int64, items []Metric) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM persona_metric WHERE persona_id = ?`, personaID); err != nil {
		return err
	}
	for i, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO persona_metric (persona_id, name, definition, sort_order) VALUES (?, ?, ?, ?)`,
			personaID, item.Name, item.Definition, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ReplaceExamples(ctx context.Context, personaID int64, items []Example) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM persona_example WHERE persona_id = ?`, personaID); err != nil {
		return err
	}
	for i, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO persona_example (persona_id, question, sql_text, sort_order) VALUES (?, ?, ?, ?)`,
			personaID, item.Question, item.SQL, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CanUse reports whether user may open this enabled persona.
// Administrators skip the grant check. Other users need a matching role grant,
// and a non-empty user grant list further restricts who may enter.
func CanUse(user User, persona Persona, roles []string, userIDs []int64) bool {
	if persona.Status != 1 || !user.Enabled {
		return false
	}
	if user.Role == "ADMIN" {
		return true
	}
	roleOK := false
	for _, role := range roles {
		if role == user.Role {
			roleOK = true
		}
	}
	if !roleOK {
		return false
	}
	if len(userIDs) == 0 {
		return true
	}
	for _, id := range userIDs {
		if id == user.ID {
			return true
		}
	}
	return false
}

func idOrNil(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

const personaSelect = `SELECT id, name, description, system_prompt, prohibitions, row_filter, datasource_id, status, catalog_text, created_at, updated_at FROM persona`

func scanPersona(row scanner) (Persona, error) {
	var p Persona
	var ds sql.NullInt64
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.SystemPrompt, &p.Prohibitions, &p.RowFilter, &ds, &p.Status, &p.CatalogText, &p.CreatedAt, &p.UpdatedAt)
	if ds.Valid {
		p.DatasourceID = ds.Int64
	}
	return p, err
}

func scanPersonas(rows *sql.Rows) ([]Persona, error) {
	var out []Persona
	for rows.Next() {
		p, err := scanPersona(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
