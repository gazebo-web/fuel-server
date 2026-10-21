package aibuilder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"

	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/bundles/projects"
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/fuel-server/mcp"
)

// The agent's HTTP API (design §9.2), under /1.0/ai-builder:
//
//	POST   /projects/{name}/chat                 start a turn {message, provider?, model?}; streams its events
//	GET    /projects/{name}/chat/stream          follow a turn (?turn=, default the latest) after Last-Event-ID
//	POST   /projects/{name}/chat/{turn}/cancel   stop a running turn
//	GET    /projects/{name}/chat/history         messages, plus the running turn if any
//	DELETE /projects/{name}/chat/history         clear the chat
//	GET    /users/{username}/llm-keys            the caller's keys, masked
//	POST   /users/{username}/llm-keys            add or replace a key
//	DELETE /users/{username}/llm-keys/{provider}
//	GET    /providers                            supported providers
//	GET    /providers/{provider}/models          models the caller's key can use
//
// Events stream as SSE: "id: <n>", "event: <type>", "data: <Event JSON>".
// A turn's last event is "done", with its status.

// Config is what the server needs besides the database.
type Config struct {
	Encryptor *Encryptor
	Providers *llm.Registry
	// Tools is the aitools registry, called in-process.
	Tools *mcp.Registry
	// Auth authenticates a request and puts its user in the context
	// (WithUser); it answers unauthenticated requests itself.
	Auth func(http.Handler) http.Handler
	// Effort is the reasoning effort on models that support it.
	Effort string
	// AllowedOrigin is the web app's origin, for CORS ("" echoes the
	// request's origin).
	AllowedOrigin string
}

// Server is the agent's HTTP server.
type Server struct {
	cfg   Config
	db    *gorm.DB
	store *Store
	turns *turnRunner
}

var projectService = &projects.Service{}

// NewServer migrates the agent's tables, marks turns a previous process
// left running as failed, and returns the server.
func NewServer(db *gorm.DB, cfg Config) (*Server, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	if n, err := store.FailInterrupted(); err != nil {
		return nil, err
	} else if n > 0 {
		log.Printf("aibuilder: marked %d interrupted turns as failed", n)
	}
	return &Server{cfg: cfg, db: db, store: store, turns: newTurnRunner(store)}, nil
}

// Handler is the whole API with CORS and an access log.
func (s *Server) Handler() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods("GET")
	api := r.PathPrefix("/1.0/ai-builder").Subrouter()
	api.HandleFunc("/providers", s.listProviders).Methods("GET")
	auth := api.NewRoute().Subrouter()
	auth.Use(mux.MiddlewareFunc(s.cfg.Auth))
	auth.HandleFunc("/providers/{provider}/models", s.listModels).Methods("GET")
	auth.HandleFunc("/projects/{name}/chat", s.chat).Methods("POST")
	auth.HandleFunc("/projects/{name}/chat/stream", s.stream).Methods("GET")
	auth.HandleFunc("/projects/{name}/chat/{turn}/cancel", s.cancel).Methods("POST")
	auth.HandleFunc("/projects/{name}/chat/history", s.history).Methods("GET")
	auth.HandleFunc("/projects/{name}/chat/history", s.clearHistory).Methods("DELETE")
	auth.HandleFunc("/users/{username}/llm-keys", s.listKeys).Methods("GET")
	auth.HandleFunc("/users/{username}/llm-keys", s.putKey).Methods("POST")
	auth.HandleFunc("/users/{username}/llm-keys/{provider}", s.deleteKey).Methods("DELETE")
	return cors(s.cfg.AllowedOrigin, accessLog(r))
}

// -- users -----------------------------------------------------------------------

type userKey struct{}

// WithUser returns ctx carrying the authenticated user.
func WithUser(ctx context.Context, u *users.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

func userFrom(r *http.Request) *users.User {
	u, _ := r.Context().Value(userKey{}).(*users.User)
	return u
}

func username(r *http.Request) string {
	if u := userFrom(r); u != nil && u.Username != nil {
		return *u.Username
	}
	return ""
}

// -- chat ------------------------------------------------------------------------

type chatRequest struct {
	Message  string `json:"message"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Attachments are project files (ids or names) sent with the message.
	Attachments []string `json:"attachments,omitempty"`
}

// session resolves the URL's project for the caller and its chat session.
func (s *Server) session(w http.ResponseWriter, r *http.Request) (*projects.Project, *ChatSession, bool) {
	p, em := projectService.GetForCaller(s.db, mux.Vars(r)["name"], userFrom(r))
	if em != nil {
		writeError(w, em.StatusCode, em.Msg)
		return nil, nil, false
	}
	if p.UUID == nil {
		writeError(w, http.StatusInternalServerError, "project has no uuid")
		return nil, nil, false
	}
	cs, err := s.store.Session(*p.UUID, username(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	return p, cs, true
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Message == "" && len(body.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "message required")
		return
	}
	p, cs, ok := s.session(w, r)
	if !ok {
		return
	}
	in, err := resolveAttachments(s.db, *p.Name, userFrom(r), body.Message, body.Attachments)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	provider, key, ok := s.provider(w, username(r), body.Provider)
	if !ok {
		return
	}
	history, err := s.store.Messages(cs.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	turn, err := s.store.StartTurn(cs.ID, key.Provider, body.Model, body.Message)
	if errors.Is(err, ErrTurnRunning) {
		writeError(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cfg := DefaultEngineConfig()
	cfg.SystemPrompt = BuildSystemPrompt(*p.Name, str(p.GazeboVersion), str(p.SDFVersion), str(p.ROS2Distro))
	cfg.SDFVersion = str(p.SDFVersion)
	cfg.Model = body.Model
	cfg.Effort = s.cfg.Effort
	engine := NewEngine(provider, &userTools{reg: s.cfg.Tools, db: s.db, user: userFrom(r)}, cfg)
	s.turns.start(turn, func(ctx context.Context, out chan<- Event) {
		engine.Run(ctx, history, in, cs.ID, out, s.store.AppendMessage)
	})
	w.Header().Set("X-Turn-ID", turn.ID)
	s.follow(w, r, turn.ID, 0)
}

// provider builds the caller's LLM provider from their key for name, or
// their latest key when name is "".
func (s *Server) provider(w http.ResponseWriter, owner, name string) (llm.Provider, *LLMKey, bool) {
	key, err := s.store.Key(owner, name)
	if errors.Is(err, ErrNotFound) {
		msg := "no LLM API keys configured; add one in AI Builder settings"
		if name != "" {
			msg = "no API key configured for " + name
		}
		writeError(w, http.StatusPreconditionRequired, msg)
		return nil, nil, false
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, nil, false
	}
	apiKey := ""
	if key.Encrypted != "" {
		if apiKey, err = s.cfg.Encryptor.Decrypt(key.Encrypted); err != nil {
			writeError(w, http.StatusInternalServerError, "your stored key can't be read; add it again in AI Builder settings")
			return nil, nil, false
		}
	}
	p, err := s.cfg.Providers.Build(key.Provider, apiKey, key.Endpoint)
	if err != nil {
		writeError(w, http.StatusBadRequest, "provider: "+err.Error())
		return nil, nil, false
	}
	return p, key, true
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	_, cs, ok := s.session(w, r)
	if !ok {
		return
	}
	var turn *ChatTurn
	var err error
	if id := r.URL.Query().Get("turn"); id != "" {
		turn, err = s.store.Turn(cs.ID, id)
	} else {
		turn, err = s.store.LatestTurn(cs.ID)
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such turn")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	after := r.Header.Get("Last-Event-ID")
	if after == "" {
		after = r.URL.Query().Get("after")
	}
	n, _ := strconv.ParseUint(after, 10, 64)
	w.Header().Set("X-Turn-ID", turn.ID)
	s.follow(w, r, turn.ID, uint(n))
}

// follow streams a turn's events after the id as SSE until its done event
// or the client leaves; leaving doesn't stop the turn.
func (s *Server) follow(w http.ResponseWriter, r *http.Request, turnID string, after uint) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	err := s.turns.follow(r.Context(), turnID, after, func(e ChatEvent) error {
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.Type, e.Payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}, func() error {
		if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
	if err != nil && r.Context().Err() == nil {
		log.Printf("aibuilder: stream turn %s: %v", turnID, err)
	}
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	_, cs, ok := s.session(w, r)
	if !ok {
		return
	}
	turn, err := s.store.Turn(cs.ID, mux.Vars(r)["turn"])
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such turn")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if turn.Status != TurnRunning || !s.turns.cancel(turn.ID) {
		writeError(w, http.StatusConflict, "the turn isn't running")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"turn_id": turn.ID, "status": "cancelling"})
}

// historyResponse keeps the shape the web app reads, plus the running turn
// so a reloaded page can follow it. The running turn's own messages are
// left out: following its stream replays them.
type historyResponse struct {
	ID         string        `json:"id"`
	ProjectID  string        `json:"project_id"`
	Messages   []ChatMessage `json:"messages"`
	CreatedAt  time.Time     `json:"created_at"`
	ActiveTurn *ChatTurn     `json:"active_turn,omitempty"`
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	_, cs, ok := s.session(w, r)
	if !ok {
		return
	}
	msgs, err := s.store.Messages(cs.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []ChatMessage{}
	}
	resp := historyResponse{ID: cs.ID, ProjectID: cs.ProjectUUID, CreatedAt: cs.CreatedAt}
	if t, err := s.store.LatestTurn(cs.ID); err == nil && t.Status == TurnRunning {
		resp.ActiveTurn = t
		for i, m := range msgs {
			if !m.CreatedAt.Before(t.StartedAt) {
				msgs = msgs[:i]
				break
			}
		}
	}
	resp.Messages = msgs
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	_, cs, ok := s.session(w, r)
	if !ok {
		return
	}
	if t, err := s.store.LatestTurn(cs.ID); err == nil && t.Status == TurnRunning {
		writeError(w, http.StatusConflict, "stop the assistant before clearing the chat")
		return
	}
	if err := s.store.ClearSession(cs.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// -- keys and providers ------------------------------------------------------------

type putKeyRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint,omitempty"`
}

// self checks that the URL's user is the caller: keys are only ever the
// caller's own.
func self(w http.ResponseWriter, r *http.Request) (string, bool) {
	me := username(r)
	if me == "" || mux.Vars(r)["username"] != me {
		writeError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return me, true
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	me, ok := self(w, r)
	if !ok {
		return
	}
	keys, err := s.store.Keys(me)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) putKey(w http.ResponseWriter, r *http.Request) {
	me, ok := self(w, r)
	if !ok {
		return
	}
	var body putKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !knownProvider(s.cfg.Providers, body.Provider) {
		writeError(w, http.StatusBadRequest, "unknown provider "+strconv.Quote(body.Provider))
		return
	}
	// Ollama has no auth, so its key is optional.
	if body.Provider != "ollama" && body.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key required for "+body.Provider)
		return
	}
	k := &LLMKey{Owner: me, Provider: body.Provider, Masked: Mask(body.APIKey), Endpoint: body.Endpoint}
	if body.APIKey != "" {
		enc, err := s.cfg.Encryptor.Encrypt(body.APIKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "encrypt: "+err.Error())
			return
		}
		k.Encrypted = enc
	}
	if err := s.store.PutKey(k); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	me, ok := self(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteKey(me, mux.Vars(r)["provider"])
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such key")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func knownProvider(reg *llm.Registry, name string) bool {
	for _, p := range reg.Providers() {
		if p == name {
			return true
		}
	}
	return false
}

func (s *Server) listProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.cfg.Providers.Providers()})
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["provider"]
	p, _, ok := s.provider(w, username(r), name)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	models, err := p.ListModels(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "list models: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// -- helpers -----------------------------------------------------------------------

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func cors(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := allowedOrigin
		if origin == "" {
			origin = r.Header.Get("Origin")
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID, Private-Token")
		w.Header().Set("Access-Control-Expose-Headers", "X-Turn-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder notes the status for the access log; Flush passes through
// so streams still stream.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s → %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
