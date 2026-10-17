package httpapi

import (
	"github.com/lwl0820/porter/internal/auth"
	"github.com/lwl0820/porter/internal/model"
	"net/http"
)

func (s *Server) plans(w http.ResponseWriter, r *http.Request, session auth.Session) {
	values, err := s.rules.Plans(actor(session))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]any{"plans": values})
}
func (s *Server) createPlan(w http.ResponseWriter, r *http.Request, session auth.Session) {
	var input model.PlanInput
	if !decode(w, r, &input) {
		return
	}
	value, err := s.rules.CreatePlan(r.Context(), actor(session), input)
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 201, value)
}
func (s *Server) users(w http.ResponseWriter, r *http.Request, session auth.Session) {
	values, err := s.rules.Users(actor(session))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]any{"users": values})
}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request, session auth.Session) {
	var input model.UserInput
	if !decode(w, r, &input) {
		return
	}
	value, err := s.rules.CreateUser(r.Context(), actor(session), input)
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 201, value)
}
func (s *Server) account(w http.ResponseWriter, r *http.Request, session auth.Session) {
	value, err := s.rules.Account(actor(session))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, value)
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request, session auth.Session) {
	value, err := s.rules.Settings(r.Context(), actor(session))
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("ETag", etag(value.Revision))
	write(w, 200, value)
}
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, session auth.Session) {
	revision, ok := match(w, r)
	if !ok {
		return
	}
	var input model.SettingsInput
	if !decode(w, r, &input) {
		return
	}
	value, err := s.rules.SaveSettings(r.Context(), actor(session), revision, input)
	if err != nil {
		failure(w, err)
		return
	}
	w.Header().Set("ETag", etag(value.Revision))
	write(w, 200, value)
}
