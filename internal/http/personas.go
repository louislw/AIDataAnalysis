package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/louislw/AIDataAnalysis/internal/sqlguard"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

func (s *Server) listPersonas(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListPersonas(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取人物失败")
		return
	}
	if items == nil {
		items = []store.Persona{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) personasMine(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListPersonas(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取人物失败")
		return
	}
	user := currentUser(r)
	out := []store.Persona{}
	for _, item := range items {
		roles, _ := s.store.RoleGrants(r.Context(), item.ID)
		users, _ := s.store.UserGrants(r.Context(), item.ID)
		if store.CanUse(user, item, roles, users) {
			out = append(out, item)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createPersona(w http.ResponseWriter, r *http.Request) {
	p, ok := s.readPersona(w, r, 0)
	if !ok {
		return
	}
	created, err := s.store.CreatePersona(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法创建人物")
		return
	}
	_ = s.rebuildOne(r.Context(), created.ID)
	created, _ = s.store.Persona(r.Context(), created.ID)
	writeJSON(w, http.StatusOK, created)
}

func (s *Server) getPersona(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return
	}
	p, err := s.store.Persona(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "人物不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取人物失败")
		return
	}
	tables, _ := s.store.TableGrants(r.Context(), id)
	roles, _ := s.store.RoleGrants(r.Context(), id)
	users, _ := s.store.UserGrants(r.Context(), id)
	metrics, _ := s.store.Metrics(r.Context(), id)
	examples, _ := s.store.Examples(r.Context(), id)
	if tables == nil {
		tables = []string{}
	}
	if roles == nil {
		roles = []string{}
	}
	if users == nil {
		users = []int64{}
	}
	if metrics == nil {
		metrics = []store.Metric{}
	}
	if examples == nil {
		examples = []store.Example{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"persona": p, "tables": tables, "roles": roles, "userIds": users, "metrics": metrics, "examples": examples,
	})
}

func (s *Server) updatePersona(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return
	}
	p, ok := s.readPersona(w, r, id)
	if !ok {
		return
	}
	updated, err := s.store.UpdatePersona(r.Context(), p)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "人物不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法更新人物")
		return
	}
	_ = s.rebuildOne(r.Context(), id)
	updated, _ = s.store.Persona(r.Context(), id)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deletePersona(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return
	}
	if err := s.store.DeletePersona(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "人物不存在")
			return
		}
		writeError(w, http.StatusBadRequest, "无法删除人物")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) readPersona(w http.ResponseWriter, r *http.Request, id int64) (store.Persona, bool) {
	var body struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		SystemPrompt string `json:"systemPrompt"`
		Prohibitions string `json:"prohibitions"`
		RowFilter    string `json:"rowFilter"`
		DatasourceID int64  `json:"datasourceId"`
		Status       *int   `json:"status"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return store.Persona{}, false
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "需要人物名称")
		return store.Persona{}, false
	}
	if err := sqlguard.ValidateRowFilter(body.RowFilter); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return store.Persona{}, false
	}
	if body.DatasourceID != 0 {
		if _, err := s.store.Datasource(r.Context(), body.DatasourceID); err != nil {
			writeError(w, http.StatusBadRequest, "数据源不存在")
			return store.Persona{}, false
		}
	}
	status := 1
	if body.Status != nil {
		if *body.Status != 0 && *body.Status != 1 {
			writeError(w, http.StatusBadRequest, "状态只能是启用或停用")
			return store.Persona{}, false
		}
		status = *body.Status
	}
	return store.Persona{
		ID: id, Name: body.Name, Description: body.Description, SystemPrompt: body.SystemPrompt,
		Prohibitions: body.Prohibitions, RowFilter: strings.TrimSpace(body.RowFilter),
		DatasourceID: body.DatasourceID, Status: status,
	}, true
}

func (s *Server) putPersonaTables(w http.ResponseWriter, r *http.Request) {
	id, p, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	var body struct {
		Tables []string `json:"tables"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Tables == nil {
		body.Tables = []string{}
	}
	if p.DatasourceID != 0 && len(body.Tables) > 0 {
		ds, err := s.store.Datasource(r.Context(), p.DatasourceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "数据源不存在")
			return
		}
		known, err := s.files.Tables(r.Context(), ds.FileName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		metas, _ := s.store.TableMetas(r.Context(), ds.ID)
		okName := map[string]string{}
		for _, name := range known {
			okName[strings.ToLower(name)] = name
		}
		clean := make([]string, 0, len(body.Tables))
		for _, name := range body.Tables {
			real, found := okName[strings.ToLower(strings.TrimSpace(name))]
			if !found {
				writeError(w, http.StatusBadRequest, "表不存在: "+name)
				return
			}
			if meta, ok := metas[real]; ok && meta.Hidden {
				writeError(w, http.StatusBadRequest, "隐藏表不能授权: "+real)
				return
			}
			clean = append(clean, real)
		}
		body.Tables = clean
	}
	if err := s.store.ReplaceTableGrants(r.Context(), id, body.Tables); err != nil {
		writeError(w, http.StatusBadRequest, "无法保存表授权")
		return
	}
	_ = s.rebuildOne(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"tables": body.Tables})
}

func (s *Server) putPersonaRoles(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	var body struct {
		Roles []string `json:"roles"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Roles == nil {
		body.Roles = []string{}
	}
	for _, role := range body.Roles {
		if role != "ADMIN" && role != "USER" {
			writeError(w, http.StatusBadRequest, "角色只能是 ADMIN 或 USER")
			return
		}
	}
	if err := s.store.ReplaceRoleGrants(r.Context(), id, body.Roles); err != nil {
		writeError(w, http.StatusBadRequest, "无法保存角色授权")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": body.Roles})
}

func (s *Server) putPersonaUsers(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	var body struct {
		UserIDs []int64 `json:"userIds"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.UserIDs == nil {
		body.UserIDs = []int64{}
	}
	for _, uid := range body.UserIDs {
		if _, err := s.store.UserByID(r.Context(), uid); err != nil {
			writeError(w, http.StatusBadRequest, "用户不存在")
			return
		}
	}
	if err := s.store.ReplaceUserGrants(r.Context(), id, body.UserIDs); err != nil {
		writeError(w, http.StatusBadRequest, "无法保存用户授权")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userIds": body.UserIDs})
}

func (s *Server) putMetrics(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	var body struct {
		Metrics []store.Metric `json:"metrics"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Metrics == nil {
		body.Metrics = []store.Metric{}
	}
	for _, m := range body.Metrics {
		if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Definition) == "" {
			writeError(w, http.StatusBadRequest, "口径需要名称和定义")
			return
		}
	}
	if err := s.store.ReplaceMetrics(r.Context(), id, body.Metrics); err != nil {
		writeError(w, http.StatusBadRequest, "无法保存口径")
		return
	}
	_ = s.rebuildOne(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"metrics": body.Metrics})
}

func (s *Server) putExamples(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	var body struct {
		Examples []store.Example `json:"examples"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Examples == nil {
		body.Examples = []store.Example{}
	}
	for _, ex := range body.Examples {
		if strings.TrimSpace(ex.Question) == "" || strings.TrimSpace(ex.SQL) == "" {
			writeError(w, http.StatusBadRequest, "示例需要问题和 SQL")
			return
		}
	}
	if err := s.store.ReplaceExamples(r.Context(), id, body.Examples); err != nil {
		writeError(w, http.StatusBadRequest, "无法保存示例")
		return
	}
	_ = s.rebuildOne(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"examples": body.Examples})
}

func (s *Server) rebuildCatalog(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.adminPersona(w, r)
	if !ok {
		return
	}
	if err := s.rebuildOne(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, "无法重建目录")
		return
	}
	p, _ := s.store.Persona(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"catalogText": p.CatalogText})
}

func (s *Server) adminPersona(w http.ResponseWriter, r *http.Request) (int64, store.Persona, bool) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return 0, store.Persona{}, false
	}
	p, err := s.store.Persona(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "人物不存在")
		return 0, store.Persona{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取人物失败")
		return 0, store.Persona{}, false
	}
	return id, p, true
}
