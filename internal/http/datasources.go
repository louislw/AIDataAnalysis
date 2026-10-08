package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

func (s *Server) listDatasources(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListDatasources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据源失败")
		return
	}
	if items == nil {
		items = []store.Datasource{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createDatasource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		FileName string `json:"fileName"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.FileName = strings.TrimSpace(body.FileName)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "需要数据源名称")
		return
	}
	if _, err := s.files.Resolve(body.FileName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.store.CreateDatasource(r.Context(), body.Name, body.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法登记数据源")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) testDatasource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FileName string `json:"fileName"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	n, err := s.files.TableCount(r.Context(), strings.TrimSpace(body.FileName))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tableCount": n})
}

func (s *Server) updateDatasource(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据源编号无效")
		return
	}
	var body struct {
		Name     string `json:"name"`
		FileName string `json:"fileName"`
		Status   *int   `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if _, err := s.files.Resolve(strings.TrimSpace(body.FileName)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := s.store.Datasource(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "数据源不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据源失败")
		return
	}
	status := existing.Status
	if body.Status != nil {
		status = *body.Status
	}
	item, err := s.store.UpdateDatasource(r.Context(), id, strings.TrimSpace(body.Name), strings.TrimSpace(body.FileName), status)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "数据源不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法更新数据源")
		return
	}
	s.rebuildDatasource(r.Context(), id)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteDatasource(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据源编号无效")
		return
	}
	if err := s.store.DeleteDatasource(r.Context(), id); err != nil {
		if store.IsInUse(err) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "数据源不存在")
			return
		}
		writeError(w, http.StatusBadRequest, "无法删除数据源")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) datasource(w http.ResponseWriter, r *http.Request) (store.Datasource, bool) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "数据源编号无效")
		return store.Datasource{}, false
	}
	item, err := s.store.Datasource(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "数据源不存在")
		return store.Datasource{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据源失败")
		return store.Datasource{}, false
	}
	return item, true
}

func (s *Server) listTables(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.datasource(w, r)
	if !ok {
		return
	}
	names, err := s.files.Tables(r.Context(), ds.FileName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metas, _ := s.store.TableMetas(r.Context(), ds.ID)
	type row struct {
		Name        string `json:"name"`
		Comment     string `json:"comment"`
		Description string `json:"description"`
		Hidden      bool   `json:"hidden"`
	}
	out := []row{}
	for _, name := range names {
		item := row{Name: name}
		if meta, ok := metas[name]; ok {
			item.Comment = meta.CustomComment
			item.Description = meta.Description
			item.Hidden = meta.Hidden
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": out})
}

func (s *Server) listColumns(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.datasource(w, r)
	if !ok {
		return
	}
	table := chi.URLParam(r, "table")
	cols, err := s.files.Columns(r.Context(), ds.FileName, table)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	metas, _ := s.store.ColumnMetas(r.Context(), ds.ID, table)
	type row struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Comment     string `json:"comment"`
		Description string `json:"description"`
		Sensitive   bool   `json:"sensitive"`
	}
	out := []row{}
	for _, col := range cols {
		item := row{Name: col.Name, Type: col.Type}
		if meta, ok := metas[col.Name]; ok {
			item.Comment = meta.CustomComment
			item.Description = meta.Description
			item.Sensitive = meta.Sensitive
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": out})
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.datasource(w, r)
	if !ok {
		return
	}
	table := chi.URLParam(r, "table")
	metas, _ := s.store.ColumnMetas(r.Context(), ds.ID, table)
	sensitive := map[string]bool{}
	for _, meta := range metas {
		if meta.Sensitive {
			sensitive[strings.ToLower(meta.ColumnName)] = true
		}
	}
	cols, rows, err := s.files.Preview(r.Context(), ds.FileName, table, sensitive, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cols == nil {
		cols = []string{}
	}
	if rows == nil {
		rows = [][]string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": cols, "rows": rows})
}

func (s *Server) saveTableMeta(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.datasource(w, r)
	if !ok {
		return
	}
	table := chi.URLParam(r, "table")
	var body struct {
		Comment     string `json:"comment"`
		Description string `json:"description"`
		Hidden      bool   `json:"hidden"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	err := s.store.SaveTableMeta(r.Context(), ds.ID, table, store.TableMeta{
		CustomComment: body.Comment, Description: body.Description, Hidden: body.Hidden,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法保存表注释")
		return
	}
	s.rebuildDatasource(r.Context(), ds.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) saveColumnMeta(w http.ResponseWriter, r *http.Request) {
	ds, ok := s.datasource(w, r)
	if !ok {
		return
	}
	table := chi.URLParam(r, "table")
	column := chi.URLParam(r, "column")
	var body struct {
		Comment     string `json:"comment"`
		Description string `json:"description"`
		Sensitive   bool   `json:"sensitive"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	err := s.store.SaveColumnMeta(r.Context(), ds.ID, table, column, store.ColumnMeta{
		CustomComment: body.Comment, Description: body.Description, Sensitive: body.Sensitive,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法保存字段注释")
		return
	}
	s.rebuildDatasource(r.Context(), ds.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
