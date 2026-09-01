/**
 * Result rules for workstation binding actions.
 * Each helper is the expected outcome of a button/input, kept here so UI and
 * tests share one definition.
 */

export interface BindableAgent {
  id: string;
  status: string;
}

export interface BindableWorkstation {
  id: string;
  active: boolean;
}

export interface BindingLinkRef {
  agentId: string;
  workstationId: string;
  isDefault: boolean;
}

/** Bind picker: only active agents that are not already on this workstation. */
export function availableAgentsForBind<T extends BindableAgent>(
  agents: T[],
  links: Pick<BindingLinkRef, "agentId">[],
): T[] {
  const bound = new Set(links.map((l) => l.agentId));
  return agents.filter((a) => a.status === "active" && !bound.has(a.id));
}

/** Agent-card picker: only active workstations not already bound to this agent. */
export function availableWorkstationsForBind<T extends BindableWorkstation>(
  workstations: T[],
  links: Pick<BindingLinkRef, "workstationId">[],
): T[] {
  const bound = new Set(links.map((l) => l.workstationId));
  return workstations.filter((ws) => ws.active && !bound.has(ws.id));
}

export type BindPickerView = "loading" | "error" | "empty" | "ready";

/** Bind picker result: never treat in-flight or failed loads as "nothing to bind". */
export function bindPickerView(
  loading: boolean,
  error: string | null | undefined,
  availableCount: number,
): BindPickerView {
  if (loading) return "loading";
  if (error) return "error";
  if (availableCount === 0) return "empty";
  return "ready";
}

/** Bind button is enabled only when a target is chosen and no write is in flight. */
export function bindButtonEnabled(selectedId: string, saving: boolean): boolean {
  return Boolean(selectedId) && !saving;
}

/** After a successful Bind, the form resets so the next click cannot replay the same row. */
export function bindFormAfterSuccess(): { selectedId: string; isDefault: boolean } {
  return { selectedId: "", isDefault: true };
}

/**
 * After Unbind of a default, exec still needs a default if other links remain.
 * Promote the first remaining workstation (stable: first in the given list).
 */
export function remainingDefaultAfterUnlink(
  links: BindingLinkRef[],
  unlinkedWorkstationId: string,
): string | null {
  const remaining = links.filter((l) => l.workstationId !== unlinkedWorkstationId);
  if (remaining.length === 0) return null;
  const stillDefault = remaining.find((l) => l.isDefault);
  if (stillDefault) return stillDefault.workstationId;
  return remaining[0].workstationId;
}
