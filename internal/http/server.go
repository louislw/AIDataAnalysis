package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/louislw/AIDataAnalysis/internal/agent"
	"github.com/louislw/AIDataAnalysis/internal/auth"
	"github.com/louislw/AIDataAnalysis/internal/catalog"
	"github.com/louislw/AIDataAnalysis/internal/config"
	"github.com/louislw/AIDataAnalysis/internal/datasource"
	"github.com/louislw/AIDataAnalysis/internal/llm"
	"github.com/louislw/AIDataAnalysis/internal/query"
	"github.com/louislw/AIDataAnalysis/internal/sqlguard"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	files  *datasource.Opener
	llm    llm.Client
	static http.Handler
}

func New(cfg config.Config, st *store.Store, files *datasource.Opener, client llm.Client) http.Handler {
	s := &Server{cfg: cfg, store: st, files: files, llm: client, static: staticHandler()}
	r := chi.NewRouter()
	r.Get("/api/health", s.health)
	r.Post("/api/auth/login", s.login)
	r.Post("/api/auth/register", s.register)
	r.Group(func(r chi.Router) {
		r.Use(s.authenticated)
		r.Get("/api/auth/me", s.me)
		r.Get("/api/personas/mine", s.personasMine)
		r.Get("/api/personas/{id}/sessions", s.listSessions)
		r.Post("/api/personas/{id}/sessions", s.createSession)
		r.Get("/api/sessions/{id}", s.getSession)
		r.Delete("/api/sessions/{id}", s.deleteSession)
		r.Post("/api/sessions/{id}/query", s.query)

		r.Group(func(r chi.Router) {
			r.Use(s.adminOnly)
			r.Get("/api/auth/users", s.listUsers)
			r.Put("/api/auth/users/{id}", s.updateUser)
			r.Get("/api/datasources", s.listDatasources)
			r.Post("/api/datasources", s.createDatasource)
			r.Post("/api/datasources/test", s.testDatasource)
			r.Put("/api/datasources/{id}", s.updateDatasource)
			r.Delete("/api/datasources/{id}", s.deleteDatasource)
			r.Get("/api/datasources/{id}/tables", s.listTables)
			r.Get("/api/datasources/{id}/tables/{table}/columns", s.listColumns)
			r.Get("/api/datasources/{id}/tables/{table}/preview", s.preview)
			r.Put("/api/datasources/{id}/tables/{table}/meta", s.saveTableMeta)
			r.Put("/api/datasources/{id}/tables/{table}/columns/{column}/meta", s.saveColumnMeta)
			r.Get("/api/personas", s.listPersonas)
			r.Post("/api/personas", s.createPersona)
			r.Get("/api/personas/{id}", s.getPersona)
			r.Put("/api/personas/{id}", s.updatePersona)
			r.Delete("/api/personas/{id}", s.deletePersona)
			r.Put("/api/personas/{id}/tables", s.putPersonaTables)
			r.Put("/api/personas/{id}/roles", s.putPersonaRoles)
			r.Put("/api/personas/{id}/users", s.putPersonaUsers)
			r.Put("/api/personas/{id}/metrics", s.putMetrics)
			r.Put("/api/personas/{id}/examples", s.putExamples)
			r.Post("/api/personas/{id}/catalog/rebuild", s.rebuildCatalog)
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "接口不存在")
			return
		}
		s.static.ServeHTTP(w, r)
	})
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"llmTimeoutSeconds": int(s.cfg.LLMTimeout / time.Second),
	})
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		raw, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || strings.TrimSpace(raw) == "" {
			writeError(w, http.StatusUnauthorized, "需要登录")
			return
		}
		id, err := auth.Parse(s.cfg.JWTSecret, strings.TrimSpace(raw))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "登录已失效")
			return
		}
		user, err := s.store.UserByID(r.Context(), id)
		if err != nil || !user.Enabled {
			writeError(w, http.StatusUnauthorized, "登录已失效")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "ADMIN" {
			writeError(w, http.StatusForbidden, "需要管理员")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) store.User {
	u, _ := r.Context().Value(userKey).(store.User)
	return u
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	user, err := s.store.UserByUsername(r.Context(), strings.TrimSpace(body.Username))
	if err != nil || !user.Enabled || !auth.CheckPassword(user.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.writeToken(w, user)
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		Dept        string `json:"dept"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	username := strings.TrimSpace(body.Username)
	if len(username) < 3 || len(username) > 32 || strings.ContainsAny(username, " \t") {
		writeError(w, http.StatusBadRequest, "用户名需要 3 到 32 个字符")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "密码至少 8 位")
		return
	}
	user, err := s.store.CreateUser(r.Context(), username, body.Password, "USER", strings.TrimSpace(body.DisplayName), strings.TrimSpace(body.Dept))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "用户名已存在")
			return
		}
		writeError(w, http.StatusBadRequest, "无法注册")
		return
	}
	s.writeToken(w, user)
}

func (s *Server) writeToken(w http.ResponseWriter, user store.User) {
	token, err := auth.Sign(s.cfg.JWTSecret, user.ID, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建登录态")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":       token,
		"username":    user.Username,
		"role":        user.Role,
		"displayName": user.DisplayName,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取用户失败")
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "用户编号无效")
		return
	}
	var body struct {
		Role        string `json:"role"`
		DisplayName string `json:"displayName"`
		Dept        string `json:"dept"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Role != "ADMIN" && body.Role != "USER" {
		writeError(w, http.StatusBadRequest, "角色只能是 ADMIN 或 USER")
		return
	}
	existing, err := s.store.UserByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取用户失败")
		return
	}
	enabled := existing.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	user, err := s.store.UpdateUser(r.Context(), id, body.Role, strings.TrimSpace(body.DisplayName), strings.TrimSpace(body.Dept), enabled)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "更新用户失败")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request, name string) (int64, error) {
	var id int64
	for _, c := range chi.URLParam(r, name) {
		if c < '0' || c > '9' {
			return 0, errors.New("bad id")
		}
		id = id*10 + int64(c-'0')
	}
	if id <= 0 {
		return 0, errors.New("bad id")
	}
	return id, nil
}

func clipTitle(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "新对话"
	}
	if utf8.RuneCountInString(s) > 40 {
		return string([]rune(s)[:40])
	}
	return s
}

func (s *Server) requirePersonaAccess(w http.ResponseWriter, r *http.Request, id int64) (store.Persona, bool) {
	p, err := s.store.Persona(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "人物不存在")
		return store.Persona{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取人物失败")
		return store.Persona{}, false
	}
	roles, _ := s.store.RoleGrants(r.Context(), id)
	users, _ := s.store.UserGrants(r.Context(), id)
	if !store.CanUse(currentUser(r), p, roles, users) {
		writeError(w, http.StatusForbidden, "不能使用该人物")
		return store.Persona{}, false
	}
	return p, true
}

func (s *Server) rebuildOne(ctx context.Context, personaID int64) error {
	p, err := s.store.Persona(ctx, personaID)
	if err != nil {
		return err
	}
	text, err := s.catalogFor(ctx, p)
	if err != nil {
		return err
	}
	return s.store.SetCatalog(ctx, personaID, text)
}

func (s *Server) rebuildDatasource(ctx context.Context, datasourceID int64) {
	personas, err := s.store.PersonasByDatasource(ctx, datasourceID)
	if err != nil {
		return
	}
	for _, p := range personas {
		_ = s.rebuildOne(ctx, p.ID)
	}
}

func (s *Server) catalogFor(ctx context.Context, p store.Persona) (string, error) {
	grants, err := s.store.TableGrants(ctx, p.ID)
	if err != nil {
		return "", err
	}
	metrics, err := s.store.Metrics(ctx, p.ID)
	if err != nil {
		return "", err
	}
	examples, err := s.store.Examples(ctx, p.ID)
	if err != nil {
		return "", err
	}
	in := catalog.Input{Prohibitions: p.Prohibitions, Metrics: metrics, Examples: examples}
	if p.DatasourceID == 0 {
		return catalog.Build(in), nil
	}
	ds, err := s.store.Datasource(ctx, p.DatasourceID)
	if err != nil {
		return catalog.Build(in), nil
	}
	metas, _ := s.store.TableMetas(ctx, ds.ID)
	for _, name := range grants {
		if meta, ok := metas[name]; ok && meta.Hidden {
			continue
		}
		cols, err := s.files.Columns(ctx, ds.FileName, name)
		if err != nil {
			continue
		}
		colMeta, _ := s.store.ColumnMetas(ctx, ds.ID, name)
		table := catalog.Table{Name: name}
		if meta, ok := metas[name]; ok {
			table.Comment = meta.CustomComment
			table.Description = meta.Description
		}
		for _, col := range cols {
			meta := colMeta[col.Name]
			if meta.Sensitive {
				continue
			}
			table.Columns = append(table.Columns, catalog.Column{
				Name: col.Name, Type: col.Type, Comment: meta.CustomComment, Description: meta.Description,
			})
		}
		in.Tables = append(in.Tables, table)
	}
	return catalog.Build(in), nil
}

func (s *Server) loadScope(ctx context.Context, user store.User, p store.Persona) (agent.Scope, error) {
	scope := agent.Scope{
		User:        user,
		Persona:     p,
		MaxRows:     s.cfg.AgentMaxRows,
		MaxLoop:     s.cfg.AgentMaxLoop,
		Sensitive:   map[string][]string{},
		ColumnNotes: map[string]map[string]string{},
		TableNotes:  map[string]string{},
	}
	if p.DatasourceID == 0 {
		return scope, nil
	}
	ds, err := s.store.Datasource(ctx, p.DatasourceID)
	if err != nil {
		return scope, err
	}
	if ds.Status != 1 {
		return scope, errors.New("数据源已停用")
	}
	path, err := s.files.Resolve(ds.FileName)
	if err != nil {
		return scope, err
	}
	scope.FilePath = path
	if strings.TrimSpace(p.RowFilter) != "" {
		compiled, err := sqlguard.CompileRowFilter(p.RowFilter, map[string]string{
			"dept":         user.Dept,
			"id":           itoa(user.ID),
			"username":     user.Username,
			"display_name": user.DisplayName,
		})
		if err != nil {
			return scope, err
		}
		scope.RowFilterSQL = compiled
	}
	grants, err := s.store.TableGrants(ctx, p.ID)
	if err != nil {
		return scope, err
	}
	metas, _ := s.store.TableMetas(ctx, ds.ID)
	for _, name := range grants {
		if meta, ok := metas[name]; ok && meta.Hidden {
			continue
		}
		cols, err := s.files.Columns(ctx, ds.FileName, name)
		if err != nil {
			return scope, err
		}
		colMeta, _ := s.store.ColumnMetas(ctx, ds.ID, name)
		qt := query.Table{Name: name}
		var sens []string
		notes := map[string]string{}
		for _, col := range cols {
			qt.Columns = append(qt.Columns, col.Name)
			meta := colMeta[col.Name]
			if meta.Sensitive {
				sens = append(sens, col.Name)
				continue
			}
			note := meta.Description
			if note == "" {
				note = meta.CustomComment
			}
			if note != "" {
				notes[col.Name] = note
			}
		}
		if meta, ok := metas[name]; ok {
			note := meta.Description
			if note == "" {
				note = meta.CustomComment
			}
			scope.TableNotes[name] = note
		}
		scope.Tables = append(scope.Tables, qt)
		scope.Sensitive[strings.ToLower(name)] = sens
		scope.ColumnNotes[name] = notes
	}
	return scope, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
