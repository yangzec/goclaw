package workstation

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type fakeWSStore struct {
	byID map[uuid.UUID]*store.Workstation
	err  error
}

func (s *fakeWSStore) GetByID(_ context.Context, id uuid.UUID) (*store.Workstation, error) {
	if s.err != nil {
		return nil, s.err
	}
	ws, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return ws, nil
}

type fakeAgentStore struct {
	byID map[uuid.UUID]store.AgentData
}

func (s *fakeAgentStore) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	a, ok := s.byID[id]
	if !ok {
		return nil, errors.New("agent not found")
	}
	return &a, nil
}

func (s *fakeAgentStore) GetByIDs(_ context.Context, ids []uuid.UUID) ([]store.AgentData, error) {
	out := make([]store.AgentData, 0, len(ids))
	for _, id := range ids {
		if a, ok := s.byID[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

type fakeLinkStore struct {
	links []store.AgentWorkstationLink
}

func (s *fakeLinkStore) Link(_ context.Context, link *store.AgentWorkstationLink) error {
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

func (s *fakeLinkStore) Unlink(_ context.Context, agentID, workstationID uuid.UUID) error {
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

func (s *fakeLinkStore) SetDefault(_ context.Context, agentID, workstationID uuid.UUID) error {
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

func (s *fakeLinkStore) ListForAgent(_ context.Context, agentID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.AgentID == agentID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *fakeLinkStore) ListForWorkstation(_ context.Context, workstationID uuid.UUID) ([]store.AgentWorkstationLink, error) {
	var out []store.AgentWorkstationLink
	for _, l := range s.links {
		if l.WorkstationID == workstationID {
			out = append(out, l)
		}
	}
	return out, nil
}

func TestBindAgent_RequiresBothSides(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.Must(uuid.NewV7())
	agentID := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, WorkstationKey: "dev", Name: "Dev"},
	}}
	agents := &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder", DisplayName: "Coder"},
	}}
	links := &fakeLinkStore{}

	if err := BindAgent(ctx, wsStore, agents, links, agentID, uuid.Must(uuid.NewV7()), false); !errors.Is(err, ErrWorkstationNotFound) {
		t.Fatalf("missing workstation: got %v", err)
	}
	if err := BindAgent(ctx, wsStore, agents, links, uuid.Must(uuid.NewV7()), wsID, false); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("missing agent: got %v", err)
	}
	if err := BindAgent(ctx, wsStore, agents, links, agentID, wsID, true); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if len(links.links) != 1 || !links.links[0].IsDefault {
		t.Fatalf("expected one default link, got %+v", links.links)
	}
}

func TestBindAgent_SecondDefaultClearsFirst(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.Must(uuid.NewV7())
	wsA := uuid.Must(uuid.NewV7())
	wsB := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsA: {ID: wsA, WorkstationKey: "a", Name: "A"},
		wsB: {ID: wsB, WorkstationKey: "b", Name: "B"},
	}}
	agents := &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder"},
	}}
	links := &fakeLinkStore{}

	if err := BindAgent(ctx, wsStore, agents, links, agentID, wsA, true); err != nil {
		t.Fatal(err)
	}
	if err := BindAgent(ctx, wsStore, agents, links, agentID, wsB, true); err != nil {
		t.Fatal(err)
	}
	var defaults int
	for _, l := range links.links {
		if l.IsDefault {
			defaults++
			if l.WorkstationID != wsB {
				t.Fatalf("default should be B, got %s", l.WorkstationID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("want 1 default, got %d", defaults)
	}
}

func TestSetDefaultBinding_RequiresExistingLink(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, WorkstationKey: "dev", Name: "Dev"},
	}}
	links := &fakeLinkStore{}

	if err := SetDefaultBinding(ctx, wsStore, links, agentID, wsID); !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUnbindAgent_PromotesRemainingDefault(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.Must(uuid.NewV7())
	wsA := uuid.Must(uuid.NewV7())
	wsB := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsA: {ID: wsA, WorkstationKey: "a", Name: "A"},
		wsB: {ID: wsB, WorkstationKey: "b", Name: "B"},
	}}
	links := &fakeLinkStore{}
	if err := BindAgent(ctx, wsStore, &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder"},
	}}, links, agentID, wsA, true); err != nil {
		t.Fatal(err)
	}
	if err := BindAgent(ctx, wsStore, &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder"},
	}}, links, agentID, wsB, false); err != nil {
		t.Fatal(err)
	}

	if err := UnbindAgent(ctx, wsStore, links, agentID, wsA); err != nil {
		t.Fatal(err)
	}
	if len(links.links) != 1 {
		t.Fatalf("expected 1 remaining link, got %+v", links.links)
	}
	if links.links[0].WorkstationID != wsB || !links.links[0].IsDefault {
		t.Fatalf("remaining link should become default: %+v", links.links[0])
	}
}

func TestUnbindAgent_LastLinkLeavesNoDefault(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, WorkstationKey: "a", Name: "A"},
	}}
	links := &fakeLinkStore{}
	if err := BindAgent(ctx, wsStore, &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder"},
	}}, links, agentID, wsID, true); err != nil {
		t.Fatal(err)
	}
	if err := UnbindAgent(ctx, wsStore, links, agentID, wsID); err != nil {
		t.Fatal(err)
	}
	if len(links.links) != 0 {
		t.Fatalf("expected no links, got %+v", links.links)
	}
}

func TestListBindings_FilterAndEnrichment(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.Must(uuid.NewV7())
	wsID := uuid.Must(uuid.NewV7())
	wsStore := &fakeWSStore{byID: map[uuid.UUID]*store.Workstation{
		wsID: {ID: wsID, WorkstationKey: "dev", Name: "Dev Box"},
	}}
	agents := &fakeAgentStore{byID: map[uuid.UUID]store.AgentData{
		agentID: {BaseModel: store.BaseModel{ID: agentID}, AgentKey: "coder", DisplayName: "Coder", Emoji: "🦊"},
	}}
	links := &fakeLinkStore{links: []store.AgentWorkstationLink{{
		AgentID: agentID, WorkstationID: wsID, IsDefault: true, CreatedAt: time.Unix(1, 0).UTC(),
	}}}

	if _, err := ListBindings(ctx, wsStore, agents, links, uuid.Nil, uuid.Nil); !errors.Is(err, ErrInvalidListFilter) {
		t.Fatalf("empty filter: got %v", err)
	}
	if _, err := ListBindings(ctx, wsStore, agents, links, wsID, agentID); !errors.Is(err, ErrInvalidListFilter) {
		t.Fatalf("both filters: got %v", err)
	}

	views, err := ListBindings(ctx, wsStore, agents, links, wsID, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("got %d views", len(views))
	}
	v := views[0]
	if v.AgentKey != "coder" || v.DisplayName != "Coder" || v.Emoji != "🦊" {
		t.Fatalf("agent enrichment: %+v", v)
	}
	if v.WorkstationKey != "dev" || v.WorkstationName != "Dev Box" || !v.IsDefault {
		t.Fatalf("workstation enrichment: %+v", v)
	}

	byAgent, err := ListBindings(ctx, wsStore, agents, links, uuid.Nil, agentID)
	if err != nil || len(byAgent) != 1 {
		t.Fatalf("list by agent: %v %#v", err, byAgent)
	}
}
