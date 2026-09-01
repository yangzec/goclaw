import { useEffect, useMemo, useState } from "react";
import { Star, Trash2, RefreshCw, Plus, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useWorkstationLinks, type WorkstationLink } from "./hooks/use-workstation-links";
import {
  availableAgentsForBind,
  bindButtonEnabled,
  bindFormAfterSuccess,
  bindPickerView,
} from "./binding-results";

interface WorkstationBindingsTabProps {
  workstationId: string;
}

function agentLabel(link: WorkstationLink): string {
  const name = link.displayName || link.agentKey || link.agentId;
  return link.emoji ? `${link.emoji} ${name}` : name;
}

export function WorkstationBindingsTab({ workstationId }: WorkstationBindingsTabProps) {
  const { t } = useTranslation("workstations");
  const { agents, loading: agentsLoading, error: agentsError, refresh: refreshAgents } = useAgents();
  const { links, loading, error, load, linkAgent, unlinkAgent, setDefault } = useWorkstationLinks();

  const [agentId, setAgentId] = useState("");
  const [isDefault, setIsDefault] = useState(true);
  const [saving, setSaving] = useState(false);
  const [unlinkTarget, setUnlinkTarget] = useState<WorkstationLink | null>(null);
  const [unlinking, setUnlinking] = useState(false);

  useEffect(() => {
    refreshAgents();
  }, [refreshAgents]);

  useEffect(() => {
    load({ workstationId });
  }, [workstationId, load]);

  const availableAgents = useMemo(() => availableAgentsForBind(agents, links), [agents, links]);
  const picker = bindPickerView(agentsLoading, agentsError, availableAgents.length);

  async function handleBind() {
    if (!agentId) return;
    setSaving(true);
    try {
      await linkAgent({ agentId, workstationId, isDefault });
      const next = bindFormAfterSuccess();
      setAgentId(next.selectedId);
      setIsDefault(next.isDefault);
      await load({ workstationId });
    } catch {
      // toast handled by hook
    } finally {
      setSaving(false);
    }
  }

  async function handleSetDefault(link: WorkstationLink) {
    if (link.isDefault) return;
    try {
      await setDefault({ agentId: link.agentId, workstationId });
      await load({ workstationId });
    } catch {
      // toast handled by hook
    }
  }

  async function handleUnlink() {
    if (!unlinkTarget) return;
    setUnlinking(true);
    try {
      await unlinkAgent({ agentId: unlinkTarget.agentId, workstationId });
      setUnlinkTarget(null);
      await load({ workstationId });
    } catch {
      // toast handled by hook
    } finally {
      setUnlinking(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1 space-y-1.5">
          <label className="text-sm font-medium">{t("bindings.agentLabel")}</label>
          {picker === "loading" ? (
            <p className="text-sm text-muted-foreground">
              <Loader2 className="mr-1 inline h-3.5 w-3.5 animate-spin" />
            </p>
          ) : picker === "error" ? (
            <p className="text-sm text-destructive">{agentsError}</p>
          ) : picker === "empty" ? (
            <p className="text-sm text-muted-foreground">{t("bindings.noAgents")}</p>
          ) : (
            <Select value={agentId || undefined} onValueChange={setAgentId}>
              <SelectTrigger className="w-full text-base md:text-sm">
                <SelectValue placeholder={t("bindings.agentPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {availableAgents.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.emoji ? `${a.emoji} ` : ""}
                    {a.display_name || a.agent_key}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>
        <label className="flex items-center gap-2 pb-1 text-sm">
          <Switch checked={isDefault} onCheckedChange={setIsDefault} />
          {t("bindings.defaultLabel")}
        </label>
        <Button size="sm" className="gap-1.5" disabled={!bindButtonEnabled(agentId, saving)} onClick={handleBind}>
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
          {t("bindings.add")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="gap-1"
          onClick={() => {
            refreshAgents();
            load({ workstationId });
          }}
          disabled={loading}
        >
          <RefreshCw className={"h-3.5 w-3.5" + (loading ? " animate-spin" : "")} />
        </Button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className="overflow-x-auto rounded-md border">
        <table className="min-w-[600px] w-full text-sm">
          <thead className="border-b bg-muted/50">
            <tr>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("bindings.columns.agent")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("bindings.columns.key")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground">
                {t("bindings.columns.default")}
              </th>
              <th className="px-3 py-2 text-right font-medium text-muted-foreground">
                {t("columns.actions")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {loading && links.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-8 text-center text-muted-foreground">
                  <Loader2 className="inline h-4 w-4 animate-spin" />
                </td>
              </tr>
            ) : links.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-8 text-center text-muted-foreground">
                  {t("bindings.empty")}
                </td>
              </tr>
            ) : (
              links.map((link) => (
                <tr key={link.agentId} className="hover:bg-muted/30">
                  <td className="px-3 py-2 font-medium">{agentLabel(link)}</td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">
                    {link.agentKey || "—"}
                  </td>
                  <td className="px-3 py-2">
                    {link.isDefault ? (
                      <Badge variant="default" className="gap-1 text-xs">
                        <Star className="h-3 w-3 fill-current" />
                        {t("bindings.defaultBadge")}
                      </Badge>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 gap-1 text-xs"
                        onClick={() => handleSetDefault(link)}
                      >
                        <Star className="h-3 w-3" />
                        {t("bindings.setDefault")}
                      </Button>
                    )}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-1 text-muted-foreground hover:text-destructive"
                      onClick={() => setUnlinkTarget(link)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t("bindings.unlink")}
                    </Button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {unlinkTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setUnlinkTarget(null)}
          title={t("bindings.unlinkTitle")}
          description={t("bindings.unlinkDescription", { name: agentLabel(unlinkTarget) })}
          confirmLabel={t("bindings.unlink")}
          variant="destructive"
          loading={unlinking}
          onConfirm={handleUnlink}
        />
      )}
    </div>
  );
}
