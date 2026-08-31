import { useState, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";
import { toast } from "@/stores/use-toast-store";
import i18n from "@/i18n";
import { userFriendlyError } from "@/lib/error-utils";

/**
 * Binding row as returned by workstations.listLinks. Field names are camelCase
 * to match workstation.LinkView — snake_case here would render blank names.
 */
export interface WorkstationLink {
  agentId: string;
  agentKey: string;
  displayName: string;
  emoji?: string;
  workstationId: string;
  workstationKey?: string;
  workstationName?: string;
  isDefault: boolean;
  createdAt: string;
}

export interface LinkAgentParams {
  agentId: string;
  workstationId: string;
  isDefault?: boolean;
}

export function useWorkstationLinks() {
  const ws = useWs();
  const [links, setLinks] = useState<WorkstationLink[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (filter: { workstationId?: string; agentId?: string }) => {
      setLoading(true);
      setError(null);
      try {
        const res = await ws.call<{ links: WorkstationLink[] }>(
          Methods.WORKSTATIONS_LIST_LINKS,
          filter,
        );
        setLinks(res.links ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load bindings");
      } finally {
        setLoading(false);
      }
    },
    [ws],
  );

  const linkAgent = useCallback(
    async (params: LinkAgentParams) => {
      try {
        await ws.call(Methods.WORKSTATIONS_LINK_AGENT, {
          agentId: params.agentId,
          workstationId: params.workstationId,
          isDefault: Boolean(params.isDefault),
        });
        toast.success(i18n.t("workstations:bindings.toast.linked"));
      } catch (err) {
        toast.error(i18n.t("workstations:bindings.toast.linkFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [ws],
  );

  const unlinkAgent = useCallback(
    async (params: { agentId: string; workstationId: string }) => {
      try {
        await ws.call(Methods.WORKSTATIONS_UNLINK_AGENT, {
          agentId: params.agentId,
          workstationId: params.workstationId,
        });
        toast.success(i18n.t("workstations:bindings.toast.unlinked"));
      } catch (err) {
        toast.error(i18n.t("workstations:bindings.toast.unlinkFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [ws],
  );

  const setDefault = useCallback(
    async (params: { agentId: string; workstationId: string }) => {
      try {
        await ws.call(Methods.WORKSTATIONS_SET_DEFAULT, {
          agentId: params.agentId,
          workstationId: params.workstationId,
        });
        toast.success(i18n.t("workstations:bindings.toast.defaultSet"));
      } catch (err) {
        toast.error(i18n.t("workstations:bindings.toast.defaultFailed"), userFriendlyError(err));
        throw err;
      }
    },
    [ws],
  );

  return { links, loading, error, load, linkAgent, unlinkAgent, setDefault };
}

/** Builds the workstations.linkAgent payload the gateway actually decodes. */
export function buildLinkAgentPayload(params: LinkAgentParams): Record<string, unknown> {
  return {
    agentId: params.agentId,
    workstationId: params.workstationId,
    isDefault: Boolean(params.isDefault),
  };
}
