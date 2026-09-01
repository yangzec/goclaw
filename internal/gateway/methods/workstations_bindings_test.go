package methods

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// UI-shaped payloads — keep these identical to use-workstation-links.ts.
func TestWorkstationsBinding_UIRoundTrip(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	agentID := uuid.Must(uuid.NewV7())

	wsStore := &bindWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, TenantID: tid, WorkstationKey: "dev-server", Name: "Dev Server", Active: true},
	}}
	agents := &bindAgentStub{byID: map[uuid.UUID]store.AgentData{
		agentID: {
			BaseModel:   store.BaseModel{ID: agentID},
			AgentKey:    "coder",
			DisplayName: "Coder",
			Emoji:       "🦊",
			Status:      "active",
		},
	}}
	links := &bindLinkStore{}

	m := NewWorkstationsMethods(wsStore, links)
	m.SetAgentStore(agents)

	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tid, "admin", 8)
	ctx := wsCallCtx(client)

	// 1. list empty
	m.handleListLinks(ctx, client, bindReq(t, protocol.MethodWorkstationsListLinks, map[string]any{
		"workstationId": wsID.String(),
	}))
	empty := readPayload(t, ch)
	if got := asSlice(empty["links"]); len(got) != 0 {
		t.Fatalf("expected no links, got %#v", empty["links"])
	}

	// 2. bind with the exact camelCase body the web UI sends
	m.handleLinkAgent(ctx, client, bindReq(t, protocol.MethodWorkstationsLinkAgent, map[string]any{
		"agentId":       agentID.String(),
		"workstationId": wsID.String(),
		"isDefault":     true,
	}))
	linked := readPayload(t, ch)
	if linked["linked"] != true {
		t.Fatalf("bind failed: %#v", linked)
	}

	// 3. list — UI table reads these camelCase fields
	m.handleListLinks(ctx, client, bindReq(t, protocol.MethodWorkstationsListLinks, map[string]any{
		"workstationId": wsID.String(),
	}))
	listed := readPayload(t, ch)
	rows := asSlice(listed["links"])
	if len(rows) != 1 {
		t.Fatalf("expected 1 link, got %#v", listed["links"])
	}
	row := rows[0].(map[string]any)
	for _, key := range []string{"agentId", "agentKey", "displayName", "workstationId", "workstationKey", "isDefault"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("list payload missing %s: %#v", key, row)
		}
	}
	if row["agentKey"] != "coder" || row["displayName"] != "Coder" || row["isDefault"] != true {
		t.Fatalf("enrichment: %#v", row)
	}

	// 4. setDefault (already default; must still succeed)
	m.handleSetDefault(ctx, client, bindReq(t, protocol.MethodWorkstationsSetDefault, map[string]any{
		"agentId":       agentID.String(),
		"workstationId": wsID.String(),
	}))
	def := readPayload(t, ch)
	if def["default"] != true {
		t.Fatalf("setDefault failed: %#v", def)
	}

	// 5. unlink
	m.handleUnlinkAgent(ctx, client, bindReq(t, protocol.MethodWorkstationsUnlinkAgent, map[string]any{
		"agentId":       agentID.String(),
		"workstationId": wsID.String(),
	}))
	unlinked := readPayload(t, ch)
	if unlinked["unlinked"] != true {
		t.Fatalf("unlink failed: %#v", unlinked)
	}

	m.handleListLinks(ctx, client, bindReq(t, protocol.MethodWorkstationsListLinks, map[string]any{
		"workstationId": wsID.String(),
	}))
	after := readPayload(t, ch)
	if got := asSlice(after["links"]); len(got) != 0 {
		t.Fatalf("expected empty after unlink, got %#v", after["links"])
	}
}

func TestWorkstationsBinding_ViewerDenied(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	m := NewWorkstationsMethods(&bindWSStore{byID: map[uuid.UUID]*store.Workstation{}}, &bindLinkStore{})
	m.SetAgentStore(&bindAgentStub{byID: map[uuid.UUID]store.AgentData{}})

	client, ch := gateway.NewCapturingTestClient(permissions.RoleViewer, tid, "viewer", 4)
	ctx := wsCallCtx(client)
	m.adminOnly(m.handleListLinks)(ctx, client, bindReq(t, protocol.MethodWorkstationsListLinks, map[string]any{
		"workstationId": uuid.Must(uuid.NewV7()).String(),
	}))
	frame := readFrame(t, ch)
	if frame.OK {
		t.Fatal("viewer must not list bindings")
	}
	if frame.Error == nil || frame.Error.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized, got %#v", frame.Error)
	}
}

func TestWorkstationsBinding_UnlinkPromotesRemainingDefault(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	wsA := uuid.Must(uuid.NewV7())
	wsB := uuid.Must(uuid.NewV7())
	agentID := uuid.Must(uuid.NewV7())

	wsStore := &bindWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsA: {ID: wsA, TenantID: tid, WorkstationKey: "a", Name: "A", Active: true},
		wsB: {ID: wsB, TenantID: tid, WorkstationKey: "b", Name: "B", Active: true},
	}}
	agents := &bindAgentStub{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder", DisplayName: "Coder"},
	}}
	links := &bindLinkStore{}
	m := NewWorkstationsMethods(wsStore, links)
	m.SetAgentStore(agents)
	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tid, "admin", 8)
	ctx := wsCallCtx(client)

	m.handleLinkAgent(ctx, client, bindReq(t, protocol.MethodWorkstationsLinkAgent, map[string]any{
		"agentId": agentID.String(), "workstationId": wsA.String(), "isDefault": true,
	}))
	readPayload(t, ch)
	m.handleLinkAgent(ctx, client, bindReq(t, protocol.MethodWorkstationsLinkAgent, map[string]any{
		"agentId": agentID.String(), "workstationId": wsB.String(), "isDefault": false,
	}))
	readPayload(t, ch)

	m.handleUnlinkAgent(ctx, client, bindReq(t, protocol.MethodWorkstationsUnlinkAgent, map[string]any{
		"agentId": agentID.String(), "workstationId": wsA.String(),
	}))
	if unlinked := readPayload(t, ch); unlinked["unlinked"] != true {
		t.Fatalf("unlink: %#v", unlinked)
	}

	m.handleListLinks(ctx, client, bindReq(t, protocol.MethodWorkstationsListLinks, map[string]any{
		"agentId": agentID.String(),
	}))
	rows := asSlice(readPayload(t, ch)["links"])
	if len(rows) != 1 {
		t.Fatalf("expected remaining B, got %#v", rows)
	}
	row := rows[0].(map[string]any)
	if row["workstationId"] != wsB.String() || row["isDefault"] != true {
		t.Fatalf("remaining default should be B: %#v", row)
	}
}

func TestWorkstationsBinding_MissingAgentIsNotFound(t *testing.T) {
	tid := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	wsStore := &bindWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, TenantID: tid, WorkstationKey: "dev", Name: "Dev"},
	}}
	m := NewWorkstationsMethods(wsStore, &bindLinkStore{})
	m.SetAgentStore(&bindAgentStub{byID: map[uuid.UUID]store.AgentData{}})

	client, ch := gateway.NewCapturingTestClient(permissions.RoleAdmin, tid, "admin", 4)
	m.handleLinkAgent(wsCallCtx(client), client, bindReq(t, protocol.MethodWorkstationsLinkAgent, map[string]any{
		"agentId":       uuid.Must(uuid.NewV7()).String(),
		"workstationId": wsID.String(),
		"isDefault":     true,
	}))
	frame := readFrame(t, ch)
	if frame.OK {
		t.Fatal("expected not-found for unknown agent")
	}
	if frame.Error == nil || frame.Error.Code != protocol.ErrNotFound {
		t.Fatalf("expected not_found, got %#v", frame.Error)
	}
}

func bindReq(t *testing.T, method string, params map[string]any) *protocol.RequestFrame {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return &protocol.RequestFrame{Type: protocol.FrameTypeRequest, ID: "req-1", Method: method, Params: raw}
}

func readFrame(t *testing.T, ch <-chan []byte) protocol.ResponseFrame {
	t.Helper()
	select {
	case raw := <-ch:
		var frame protocol.ResponseFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("decode frame: %v", err)
		}
		return frame
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}
	return protocol.ResponseFrame{}
}

func readPayload(t *testing.T, ch <-chan []byte) map[string]any {
	t.Helper()
	frame := readFrame(t, ch)
	if !frame.OK {
		t.Fatalf("expected ok response, got %#v", frame.Error)
	}
	raw, err := json.Marshal(frame.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return payload
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	s, _ := v.([]any)
	return s
}

var (
	_ store.WorkstationStore           = (*bindWSStore)(nil)
	_ store.AgentWorkstationLinkStore  = (*bindLinkStore)(nil)
	_ store.AgentStore                 = (*bindAgentStub)(nil)
)

type bindWSStore struct {
	byID map[uuid.UUID]*store.Workstation
}

func (s *bindWSStore) Create(_ context.Context, _ *store.Workstation) error { return nil }
func (s *bindWSStore) GetByID(_ context.Context, id uuid.UUID) (*store.Workstation, error) {
	ws, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return ws, nil
}
func (s *bindWSStore) GetByKey(_ context.Context, _ string) (*store.Workstation, error) {
	return nil, sql.ErrNoRows
}
func (s *bindWSStore) List(_ context.Context) ([]store.Workstation, error) { return nil, nil }
func (s *bindWSStore) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *bindWSStore) SetActive(_ context.Context, _ uuid.UUID, _ bool) error { return nil }
func (s *bindWSStore) Delete(_ context.Context, _ uuid.UUID) error            { return nil }

type bindLinkStore struct {
	links []store.AgentWorkstationLink
}

func (s *bindLinkStore) Link(_ context.Context, link *store.AgentWorkstationLink) error {
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

func (s *bindLinkStore) Unlink(_ context.Context, agentID, workstationID uuid.UUID) error {
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

func (s *bindLinkStore) SetDefault(_ context.Context, agentID, workstationID uuid.UUID) error {
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

func (s *bindLinkStore) ListForAgent(_ context.Context, agentID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.AgentID == agentID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *bindLinkStore) ListForWorkstation(_ context.Context, workstationID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.WorkstationID == workstationID {
			out = append(out, l)
		}
	}
	return out, nil
}

type bindAgentStub struct {
	byID map[uuid.UUID]store.AgentData
}

func (s *bindAgentStub) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, errors.New("agent not found")
	}
	cp := a
	return &cp, nil
}
func (s *bindAgentStub) GetByIDs(_ context.Context, ids []uuid.UUID) ([]store.AgentData, error) {
	var out []store.AgentData
	for _, id := range ids {
		if a, ok := s.byID[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *bindAgentStub) Create(_ context.Context, _ *store.AgentData) error { return nil }
func (s *bindAgentStub) GetByKey(_ context.Context, _ string) (*store.AgentData, error) {
	return nil, nil
}
func (s *bindAgentStub) GetByIDUnscoped(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	return s.GetByID(context.Background(), id)
}
func (s *bindAgentStub) GetByKeys(_ context.Context, _ []string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *bindAgentStub) Update(_ context.Context, _ uuid.UUID, _ map[string]any) error {
	return nil
}
func (s *bindAgentStub) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (s *bindAgentStub) List(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *bindAgentStub) GetDefault(_ context.Context) (*store.AgentData, error) { return nil, nil }
func (s *bindAgentStub) ResetStuckSummoning(_ context.Context) (int64, error)   { return 0, nil }
func (s *bindAgentStub) ShareAgent(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *bindAgentStub) RevokeShare(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *bindAgentStub) ListShares(_ context.Context, _ uuid.UUID) ([]store.AgentShareData, error) {
	return nil, nil
}
func (s *bindAgentStub) CanAccess(_ context.Context, _ uuid.UUID, _ string) (bool, string, error) {
	return true, "owner", nil
}
func (s *bindAgentStub) ListAccessible(_ context.Context, _ string) ([]store.AgentData, error) {
	return nil, nil
}
func (s *bindAgentStub) GetAgentContextFiles(_ context.Context, _ uuid.UUID) ([]store.AgentContextFileData, error) {
	return nil, nil
}
func (s *bindAgentStub) SetAgentContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *bindAgentStub) PropagateContextFile(_ context.Context, _ uuid.UUID, _ string) (int, error) {
	return 0, nil
}
func (s *bindAgentStub) GetUserContextFiles(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *bindAgentStub) ListUserContextFilesByName(_ context.Context, _ uuid.UUID, _ string) ([]store.UserContextFileData, error) {
	return nil, nil
}
func (s *bindAgentStub) SetUserContextFile(_ context.Context, _ uuid.UUID, _, _, _ string) error {
	return nil
}
func (s *bindAgentStub) DeleteUserContextFile(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (s *bindAgentStub) MigrateUserDataOnMerge(_ context.Context, _ []string, _ string) error {
	return nil
}
func (s *bindAgentStub) GetUserOverride(_ context.Context, _ uuid.UUID, _ string) (*store.UserAgentOverrideData, error) {
	return nil, nil
}
func (s *bindAgentStub) SetUserOverride(_ context.Context, _ *store.UserAgentOverrideData) error {
	return nil
}
func (s *bindAgentStub) GetOrCreateUserProfile(_ context.Context, _ uuid.UUID, _, _, _ string) (bool, string, error) {
	return false, "", nil
}
func (s *bindAgentStub) EnsureUserProfile(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (s *bindAgentStub) ListUserInstances(_ context.Context, _ uuid.UUID) ([]store.UserInstanceData, error) {
	return nil, nil
}
func (s *bindAgentStub) UpdateUserProfileMetadata(_ context.Context, _ uuid.UUID, _ string, _ map[string]string) error {
	return nil
}
