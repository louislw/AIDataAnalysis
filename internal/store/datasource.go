package store

import (
	"context"
	"database/sql"
)

type Datasource struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FileName  string `json:"fileName"`
	Status    int    `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type TableMeta struct {
	TableName     string `json:"name"`
	CustomComment string `json:"comment"`
	Description   string `json:"description"`
	Hidden        bool   `json:"hidden"`
}

type ColumnMeta struct {
	ColumnName    string `json:"name"`
	CustomComment string `json:"comment"`
	Description   string `json:"description"`
	Sensitive     bool   `json:"sensitive"`
}

func (s *Store) CreateDatasource(ctx context.Context, name, fileName string) (Datasource, error) {
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO datasource (name, file_name, status, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		name, fileName, now, now)
	if err != nil {
		return Datasource{}, err
	}
	id, _ := res.LastInsertId()
	return s.Datasource(ctx, id)
}

func (s *Store) ListDatasources(ctx context.Context) ([]Datasource, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, file_name, status, created_at, updated_at FROM datasource ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Datasource
	for rows.Next() {
		d, err := scanDatasource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Datasource(ctx context.Context, id int64) (Datasource, error) {
	return scanDatasource(s.DB.QueryRowContext(ctx, `SELECT id, name, file_name, status, created_at, updated_at FROM datasource WHERE id = ?`, id))
}

func (s *Store) UpdateDatasource(ctx context.Context, id int64, name, fileName string, status int) (Datasource, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE datasource SET name = ?, file_name = ?, status = ?, updated_at = ? WHERE id = ?`,
		name, fileName, status, Now(), id)
	if err != nil {
		return Datasource{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Datasource{}, sql.ErrNoRows
	}
	return s.Datasource(ctx, id)
}

func (s *Store) DeleteDatasource(ctx context.Context, id int64) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM persona WHERE datasource_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errInUse
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM datasource WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM table_meta WHERE datasource_id = ?`, id)
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM column_meta WHERE datasource_id = ?`, id)
	return nil
}

var errInUse = errString("仍有人物使用该数据源")

type errString string

func (e errString) Error() string { return string(e) }

func IsInUse(err error) bool {
	_, ok := err.(errString)
	return ok
}

func (s *Store) TableMetas(ctx context.Context, datasourceID int64) (map[string]TableMeta, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT table_name, custom_comment, description, is_hidden FROM table_meta WHERE datasource_id = ?`, datasourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TableMeta{}
	for rows.Next() {
		var m TableMeta
		var hidden int
		if err := rows.Scan(&m.TableName, &m.CustomComment, &m.Description, &hidden); err != nil {
			return nil, err
		}
		m.Hidden = hidden == 1
		out[m.TableName] = m
	}
	return out, rows.Err()
}

func (s *Store) ColumnMetas(ctx context.Context, datasourceID int64, table string) (map[string]ColumnMeta, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT column_name, custom_comment, description, is_sensitive FROM column_meta WHERE datasource_id = ? AND table_name = ?`, datasourceID, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ColumnMeta{}
	for rows.Next() {
		var m ColumnMeta
		var sensitive int
		if err := rows.Scan(&m.ColumnName, &m.CustomComment, &m.Description, &sensitive); err != nil {
			return nil, err
		}
		m.Sensitive = sensitive == 1
		out[m.ColumnName] = m
	}
	return out, rows.Err()
}

func (s *Store) SaveTableMeta(ctx context.Context, datasourceID int64, table string, meta TableMeta) error {
	hidden := 0
	if meta.Hidden {
		hidden = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO table_meta (datasource_id, table_name, custom_comment, description, is_hidden)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (datasource_id, table_name) DO UPDATE SET
		  custom_comment = excluded.custom_comment,
		  description = excluded.description,
		  is_hidden = excluded.is_hidden`,
		datasourceID, table, meta.CustomComment, meta.Description, hidden)
	return err
}

func (s *Store) SaveColumnMeta(ctx context.Context, datasourceID int64, table, column string, meta ColumnMeta) error {
	sensitive := 0
	if meta.Sensitive {
		sensitive = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO column_meta (datasource_id, table_name, column_name, custom_comment, description, is_sensitive)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (datasource_id, table_name, column_name) DO UPDATE SET
		  custom_comment = excluded.custom_comment,
		  description = excluded.description,
		  is_sensitive = excluded.is_sensitive`,
		datasourceID, table, column, meta.CustomComment, meta.Description, sensitive)
	return err
}

func scanDatasource(row scanner) (Datasource, error) {
	var d Datasource
	err := row.Scan(&d.ID, &d.Name, &d.FileName, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
