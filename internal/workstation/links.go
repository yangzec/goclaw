package workstation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Binding errors returned by BindAgent / UnbindAgent / SetDefaultBinding / ListBindings.
var (
	ErrWorkstationNotFound = errors.New("workstation not found")
	ErrAgentNotFound       = errors.New("agent not found")
	ErrLinkNotFound        = errors.New("agent workstation link not found")
	ErrInvalidListFilter   = errors.New("exactly one of workstationId or agentId is required")
)

// LinkView is the API-facing agent↔workstation binding, camelCase to match
// store.SanitizedWorkstation / WS workstation handlers.
type LinkView struct {
	AgentID         uuid.UUID `json:"agentId"`
	AgentKey        string    `json:"agentKey"`
	DisplayName     string    `json:"displayName"`
	Emoji           string    `json:"emoji,omitempty"`
	WorkstationID   uuid.UUID `json:"workstationId"`
	WorkstationKey  string    `json:"workstationKey,omitempty"`
	WorkstationName string    `json:"workstationName,omitempty"`
	IsDefault       bool      `json:"isDefault"`
	CreatedAt       time.Time `json:"createdAt"`
}

type workstationLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*store.Workstation, error)
}

type agentLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*store.AgentData, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]store.AgentData, error)
}

// BindAgent creates an agent↔workstation link after verifying both sides exist
// in the caller's tenant. isDefault is applied via SetDefault after insert so
// the partial unique index on is_default is never violated on insert.
func BindAgent(
	ctx context.Context,
	wsStore workstationLookup,
	agents agentLookup,
	links store.AgentWorkstationLinkStore,
	agentID, workstationID uuid.UUID,
	isDefault bool,
) error {
	if _, err := wsStore.GetByID(ctx, workstationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkstationNotFound
		}
		return err
	}
	if _, err := agents.GetByID(ctx, agentID); err != nil {
		return ErrAgentNotFound
	}
	if err := links.Link(ctx, &store.AgentWorkstationLink{
		AgentID:       agentID,
		WorkstationID: workstationID,
		IsDefault:     false,
	}); err != nil {
		return err
	}
	if isDefault {
		return links.SetDefault(ctx, agentID, workstationID)
	}
	return nil
}

// UnbindAgent removes an agent↔workstation link after verifying the workstation
// belongs to the caller's tenant.
func UnbindAgent(
	ctx context.Context,
	wsStore workstationLookup,
	links store.AgentWorkstationLinkStore,
	agentID, workstationID uuid.UUID,
) error {
	if _, err := wsStore.GetByID(ctx, workstationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkstationNotFound
		}
		return err
	}
	return links.Unlink(ctx, agentID, workstationID)
}

// SetDefaultBinding marks workstationID as the agent's default after verifying
// the link exists in the caller's tenant.
func SetDefaultBinding(
	ctx context.Context,
	wsStore workstationLookup,
	links store.AgentWorkstationLinkStore,
	agentID, workstationID uuid.UUID,
) error {
	if _, err := wsStore.GetByID(ctx, workstationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkstationNotFound
		}
		return err
	}
	existing, err := links.ListForAgent(ctx, agentID)
	if err != nil {
		return err
	}
	found := false
	for i := range existing {
		if existing[i].WorkstationID == workstationID {
			found = true
			break
		}
	}
	if !found {
		return ErrLinkNotFound
	}
	return links.SetDefault(ctx, agentID, workstationID)
}

// ListBindings returns enriched links for exactly one of workstationID or agentID.
func ListBindings(
	ctx context.Context,
	wsStore workstationLookup,
	agents agentLookup,
	links store.AgentWorkstationLinkStore,
	workstationID, agentID uuid.UUID,
) ([]LinkView, error) {
	wsSet := workstationID != uuid.Nil
	agentSet := agentID != uuid.Nil
	if wsSet == agentSet {
		return nil, ErrInvalidListFilter
	}

	var raw []store.AgentWorkstationLink
	var err error
	if wsSet {
		if _, err := wsStore.GetByID(ctx, workstationID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrWorkstationNotFound
			}
			return nil, err
		}
		raw, err = links.ListForWorkstation(ctx, workstationID)
	} else {
		if _, err := agents.GetByID(ctx, agentID); err != nil {
			return nil, ErrAgentNotFound
		}
		raw, err = links.ListForAgent(ctx, agentID)
	}
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return []LinkView{}, nil
	}
	return enrichLinks(ctx, wsStore, agents, raw), nil
}

func enrichLinks(
	ctx context.Context,
	wsStore workstationLookup,
	agents agentLookup,
	links []store.AgentWorkstationLink,
) []LinkView {
	views := make([]LinkView, 0, len(links))
	if len(links) == 0 {
		return views
	}

	agentIDs := make([]uuid.UUID, 0, len(links))
	seenAgents := make(map[uuid.UUID]struct{}, len(links))
	wsIDs := make([]uuid.UUID, 0, len(links))
	seenWS := make(map[uuid.UUID]struct{}, len(links))
	for i := range links {
		if _, ok := seenAgents[links[i].AgentID]; !ok {
			seenAgents[links[i].AgentID] = struct{}{}
			agentIDs = append(agentIDs, links[i].AgentID)
		}
		if _, ok := seenWS[links[i].WorkstationID]; !ok {
			seenWS[links[i].WorkstationID] = struct{}{}
			wsIDs = append(wsIDs, links[i].WorkstationID)
		}
	}

	agentByID := map[uuid.UUID]store.AgentData{}
	if agents != nil && len(agentIDs) > 0 {
		if found, err := agents.GetByIDs(ctx, agentIDs); err == nil {
			for i := range found {
				agentByID[found[i].ID] = found[i]
			}
		}
	}

	wsByID := map[uuid.UUID]*store.Workstation{}
	if wsStore != nil {
		for _, id := range wsIDs {
			if ws, err := wsStore.GetByID(ctx, id); err == nil && ws != nil {
				wsByID[id] = ws
			}
		}
	}

	for i := range links {
		v := LinkView{
			AgentID:       links[i].AgentID,
			WorkstationID: links[i].WorkstationID,
			IsDefault:     links[i].IsDefault,
			CreatedAt:     links[i].CreatedAt,
		}
		if a, ok := agentByID[links[i].AgentID]; ok {
			v.AgentKey = a.AgentKey
			v.DisplayName = a.DisplayName
			v.Emoji = a.Emoji
		}
		if ws, ok := wsByID[links[i].WorkstationID]; ok {
			v.WorkstationKey = ws.WorkstationKey
			v.WorkstationName = ws.Name
		}
		views = append(views, v)
	}
	return views
}
