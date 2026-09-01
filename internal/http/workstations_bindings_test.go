package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestWorkstationsHTTPBinding_UIRoundTrip(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	agentID := uuid.Must(uuid.NewV7())

	wsStore := &httpBindWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, TenantID: tid, WorkstationKey: "dev-server", Name: "Dev Server", Active: true},
	}}
	agents := &httpBindAgentStub{byID: map[uuid.UUID]store.AgentData{
		agentID: {
			BaseModel:   store.BaseModel{ID: agentID},
			AgentKey:    "coder",
			DisplayName: "Coder",
			Emoji:       "🦊",
			Status:      "active",
		},
	}}
	links := &httpBindLinkStore{}
	h := NewWorkstationsHandler(wsStore, links, nil)
	h.SetAgentStore(agents)

	// 1. list empty
	rec := serveBind(t, h.handleListLinks, ownerReq(http.MethodGet, "/v1/workstations/"+wsID.String()+"/links", tid, nil),
		map[string]string{"id": wsID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("list empty: %d %s", rec.Code, rec.Body.String())
	}
	if got := decodeLinks(t, rec); len(got) != 0 {
		t.Fatalf("expected no links, got %#v", got)
	}

	// 2. bind — same camelCase body the HTTP API documents
	rec = serveBind(t, h.handleLinkAgent, ownerReq(http.MethodPost, "/v1/workstations/"+wsID.String()+"/links", tid,
		bytes.NewBufferString(`{"agentId":"`+agentID.String()+`","isDefault":true}`)),
		map[string]string{"id": wsID.String()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("bind: %d %s", rec.Code, rec.Body.String())
	}

	// 3. list — UI table reads these camelCase fields
	rec = serveBind(t, h.handleListLinks, ownerReq(http.MethodGet, "/v1/workstations/"+wsID.String()+"/links", tid, nil),
		map[string]string{"id": wsID.String()})
	rows := decodeLinks(t, rec)
	if len(rows) != 1 {
		t.Fatalf("expected 1 link, got %#v", rows)
	}
	row := rows[0]
	for _, key := range []string{"agentId", "agentKey", "displayName", "workstationId", "workstationKey", "isDefault"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("list payload missing %s: %#v", key, row)
		}
	}
	if row["agentKey"] != "coder" || row["displayName"] != "Coder" || row["isDefault"] != true {
		t.Fatalf("enrichment: %#v", row)
	}

	// 4. setDefault + agent-side list
	rec = serveBind(t, h.handleSetDefault, ownerReq(http.MethodPut,
		"/v1/workstations/"+wsID.String()+"/links/"+agentID.String()+"/default", tid, nil),
		map[string]string{"id": wsID.String(), "agentId": agentID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("setDefault: %d %s", rec.Code, rec.Body.String())
	}
	rec = serveBind(t, h.handleListAgentLinks, ownerReq(http.MethodGet, "/v1/agents/"+agentID.String()+"/workstations", tid, nil),
		map[string]string{"id": agentID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("agent list: %d %s", rec.Code, rec.Body.String())
	}
	if got := decodeLinks(t, rec); len(got) != 1 {
		t.Fatalf("agent list: %#v", got)
	}

	// 5. unlink
	rec = serveBind(t, h.handleUnlinkAgent, ownerReq(http.MethodDelete,
		"/v1/workstations/"+wsID.String()+"/links/"+agentID.String(), tid, nil),
		map[string]string{"id": wsID.String(), "agentId": agentID.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("unlink: %d %s", rec.Code, rec.Body.String())
	}
	rec = serveBind(t, h.handleListLinks, ownerReq(http.MethodGet, "/v1/workstations/"+wsID.String()+"/links", tid, nil),
		map[string]string{"id": wsID.String()})
	if got := decodeLinks(t, rec); len(got) != 0 {
		t.Fatalf("expected empty after unlink, got %#v", got)
	}
}

func TestWorkstationsHTTPBinding_MissingAgentIsNotFound(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	h := NewWorkstationsHandler(&httpBindWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, TenantID: tid, WorkstationKey: "dev", Name: "Dev"},
	}}, &httpBindLinkStore{}, nil)
	h.SetAgentStore(&httpBindAgentStub{byID: map[uuid.UUID]store.AgentData{}})

	rec := serveBind(t, h.handleLinkAgent, ownerReq(http.MethodPost, "/v1/workstations/"+wsID.String()+"/links", tid,
		bytes.NewBufferString(`{"agentId":"`+uuid.Must(uuid.NewV7()).String()+`","isDefault":true}`)),
		map[string]string{"id": wsID.String()})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", rec.Code, rec.Body.String())
	}
}

func ownerReq(method, path string, tid uuid.UUID, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, path, body)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	ctx := store.WithTenantID(r.Context(), tid)
	ctx = store.WithRole(ctx, store.RoleOwner)
	return r.WithContext(ctx)
}

func serveBind(t *testing.T, next http.HandlerFunc, r *http.Request, pathVals map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	for k, v := range pathVals {
		r.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	next(rec, r)
	return rec
}

func decodeLinks(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	raw, _ := payload["links"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		row, _ := item.(map[string]any)
		out = append(out, row)
	}
	return out
}

type httpBindWSStore struct {
	byID map[uuid.UUID]*store.Workstation
}

func (s *httpBindWSStore) Create(_ context.Context, _ *store.Workstation) error { return nil }
func (s *httpBindWSStore) GetByID(_ context.Context, id uuid.UUID) (*store.Workstation, error) {
	ws, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return ws, nil
}
func (s *httpBindWSStore) GetByKey(_ context.Context, _ string) (*store.Workstation, error) {
	return nil, sql.ErrNoRows
}
func (s *httpBindWSStore) List(_ context.Context) ([]store.Workstation, error) { return nil, nil }
func (s *httpBindWSStore) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *httpBindWSStore) SetActive(_ context.Context, _ uuid.UUID, _ bool) error { return nil }
func (s *httpBindWSStore) Delete(_ context.Context, _ uuid.UUID) error            { return nil }

type httpBindLinkStore struct {
	links []store.AgentWorkstationLink
}

func (s *httpBindLinkStore) Link(_ context.Context, link *store.AgentWorkstationLink) error {
	for _, existing := range s.links {
		if existing.AgentID == link.AgentID && existing.WorkstationID == link.WorkstationID {
			return nil
		}
	}
	copied := *link
	if copied.CreatedAt.IsZero() {
		copied.CreatedAt = time.Now().UTC()
	}
	s.links = append(s.links, copied)
	return nil
}

func (s *httpBindLinkStore) Unlink(_ context.Context, agentID, workstationID uuid.UUID) error {
	kept := s.links[:0]
	for _, l := range s.links {
		if l.AgentID == agentID && l.WorkstationID == workstationID {
			continue
		}
		kept = append(kept, l)
	}
	s.links = kept
	return nil
}

func (s *httpBindLinkStore) SetDefault(_ context.Context, agentID, workstationID uuid.UUID) error {
	found := false
	for i := range s.links {
		if s.links[i].AgentID != agentID {
			continue
		}
		s.links[i].IsDefault = s.links[i].WorkstationID == workstationID
		if s.links[i].IsDefault {
			found = true
		}
	}
	if !found {
		return errors.New("no matching link")
	}
	return nil
}

func (s *httpBindLinkStore) ListForAgent(_ context.Context, agentID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.AgentID == agentID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *httpBindLinkStore) ListForWorkstation(_ context.Context, workstationID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.WorkstationID == workstationID {
			out = append(out, l)
		}
	}
	return out, nil
}

type httpBindAgentStub struct {
	byID map[uuid.UUID]store.AgentData
}

func (s *httpBindAgentStub) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, errors.New("agent not found")
	}
	cp := a
	return &cp, nil
}
func (s *httpBindAgentStub) GetByIDs(_ context.Context, ids []uuid.UUID) ([]store.AgentData, error) {
	var out []store.AgentData
	for _, id := range ids {
		if a, ok := s.byID[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *httpBindAgentStub) Create(_ context.Context, _ *store.AgentData) error { return nil }
func (s *httpBindAgentStub) GetByKey(_ context.Context, _ string) (*store.AgentData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) GetByIDUnscoped(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	return s.GetByID(context.Background(), id)
}
func (s *httpBindAgentStub) GetByKeys(_ context.Context, _ []string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *httpBindAgentStub) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (s *httpBindAgentStub) List(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) GetDefault(_ context.Context) (*store.AgentData, error) { return nil, nil }
func (s *httpBindAgentStub) ResetStuckSummoning(_ context.Context) (int64, error)   { return 0, nil }
func (s *httpBindAgentStub) ShareAgent(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *httpBindAgentStub) RevokeShare(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *httpBindAgentStub) ListShares(_ context.Context, _ uuid.UUID) ([]store.AgentShareData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) CanAccess(_ context.Context, _ uuid.UUID, _ string) (bool, string, error) {
	return true, "owner", nil
}
func (s *httpBindAgentStub) ListAccessible(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) GetAgentContextFiles(_ context.Context, _ uuid.UUID) ([]store.AgentContextFileData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) SetAgentContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *httpBindAgentStub) PropagateContextFile(_ context.Context, _ uuid.UUID, _ string) (int, error) {
	return 0, nil
}
func (s *httpBindAgentStub) GetUserContextFiles(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) ListUserContextFilesByName(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) SetUserContextFile(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *httpBindAgentStub) DeleteUserContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *httpBindAgentStub) MigrateUserDataOnMerge(_ context.Context, _ []string, _ string) error {
	return nil
}
func (s *httpBindAgentStub) GetUserOverride(_ context.Context, _ uuid.UUID, _ string) (*store.UserAgentOverrideData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) SetUserOverride(_ context.Context, _ *store.UserAgentOverrideData) error {
	return nil
}
func (s *httpBindAgentStub) GetOrCreateUserProfile(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, string, error) {
	return false, "", nil
}
func (s *httpBindAgentStub) EnsureUserProfile(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *httpBindAgentStub) ListUserInstances(_ context.Context, _ uuid.UUID) ([]store.UserInstanceData, error) {
	return nil, nil
}
func (s *httpBindAgentStub) UpdateUserProfileMetadata(_ context.Context, _ uuid.UUID, _ string, _ map[string]string) error {
	return nil
}

var (
	_ store.WorkstationStore          = (*httpBindWSStore)(nil)
	_ store.AgentWorkstationLinkStore = (*httpBindLinkStore)(nil)
	_ store.AgentStore                = (*httpBindAgentStub)(nil)
)
