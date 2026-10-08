package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/louislw/AIDataAnalysis/internal/agent"
	"github.com/louislw/AIDataAnalysis/internal/store"
)

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return
	}
	if _, ok := s.requirePersonaAccess(w, r, id); !ok {
		return
	}
	items, err := s.store.ListSessions(r.Context(), currentUser(r).ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取会话失败")
		return
	}
	if items == nil {
		items = []store.Session{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "人物编号无效")
		return
	}
	if _, ok := s.requirePersonaAccess(w, r, id); !ok {
		return
	}
	sess, err := s.store.CreateSession(r.Context(), currentUser(r).ID, id, uuid.NewString(), "新对话")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建会话")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.ownSession(w, r)
	if !ok {
		return
	}
	msgs, err := s.store.Messages(r.Context(), sess.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取消息失败")
		return
	}
	if msgs == nil {
		msgs = []store.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": sess.ID, "personaId": sess.PersonaID, "title": sess.Title, "messages": msgs,
	})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.ownSession(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteSession(r.Context(), sess.ID, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, "无法删除会话")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.ownSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Question string `json:"question"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	question := strings.TrimSpace(body.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "请输入问题")
		return
	}
	persona, ok := s.requirePersonaAccess(w, r, sess.PersonaID)
	if !ok {
		return
	}
	user := currentUser(r)
	scope, err := s.loadScope(r.Context(), user, persona)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	msgs, err := s.store.Messages(r.Context(), sess.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取消息失败")
		return
	}
	history := recentTurns(msgs, s.cfg.AgentHistoryTurns)
	answer, err := agent.Run(r.Context(), s.llm, scope, question, history)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	title := sess.Title
	if title == "" || title == "新对话" {
		title = clipTitle(question)
	}
	assistant := store.Message{
		Content: answer.Content, Kind: answer.Kind, SQL: answer.SQL, Tables: answer.Tables,
		RowCount: answer.RowCount, Truncated: answer.Truncated, Reasoning: answer.Reasoning,
	}
	if err := s.store.AddExchange(r.Context(), sess.ID, title, store.Message{Content: question}, assistant); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存会话")
		return
	}
	tables := answer.Tables
	if tables == nil {
		tables = []string{}
	}
	rowCount := 0
	if answer.RowCount != nil {
		rowCount = *answer.RowCount
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":      answer.Kind,
		"content":   answer.Content,
		"sql":       answer.SQL,
		"tables":    tables,
		"rowCount":  rowCount,
		"truncated": answer.Truncated,
		"reasoning": answer.Reasoning,
	})
}

func (s *Server) ownSession(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	id := chi.URLParam(r, "id")
	sess, err := s.store.Session(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "会话不存在")
		return store.Session{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取会话失败")
		return store.Session{}, false
	}
	if sess.UserID != currentUser(r).ID {
		writeError(w, http.StatusForbidden, "不能访问该会话")
		return store.Session{}, false
	}
	return sess, true
}

func recentTurns(msgs []store.Message, n int) []agent.Turn {
	var turns []agent.Turn
	for _, m := range msgs {
		if m.Role == "USER" {
			turns = append(turns, agent.Turn{Question: m.Content})
			continue
		}
		if len(turns) == 0 {
			continue
		}
		last := &turns[len(turns)-1]
		last.SQL = m.SQL
		last.Summary = m.Kind + " " + oneLine(m.Content)
		if m.RowCount != nil {
			last.Summary += " rows"
		}
	}
	if n > 0 && len(turns) > n {
		turns = turns[len(turns)-n:]
	}
	return turns
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 80 {
		return string([]rune(s)[:80])
	}
	return s
}
