import { useEffect, useMemo, useState } from "react";
import { MonitorCog, Plus, Star, Trash2, Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useAuthStore } from "@/stores/use-auth-store";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useWorkstations } from "@/pages/workstations/hooks/use-workstations";
import { useWorkstationLinks, type WorkstationLink } from "@/pages/workstations/hooks/use-workstation-links";
import {
  availableWorkstationsForBind,
  bindButtonEnabled,
  bindFormAfterSuccess,
  bindPickerView,
} from "@/pages/workstations/binding-results";

interface WorkstationBindingsCardProps {
  agentId: string;
}

export function WorkstationBindingsCard({ agentId }: WorkstationBindingsCardProps) {
  const { t } = useTranslation("workstations");
  const role = useAuthStore((s) => s.role);
  const edition = useAuthStore((s) => s.edition);
  const isAdmin = role === "admin" || role === "owner";

  const canManage = isAdmin && edition !== "lite";
  const { workstations, loading: wsLoading, error: wsError } = useWorkstations({ enabled: canManage });
  const { links, loading, error, load, linkAgent, unlinkAgent, setDefault } = useWorkstationLinks();
  const [workstationId, setWorkstationId] = useState("");
  const [isDefault, setIsDefault] = useState(true);
  const [saving, setSaving] = useState(false);
  const [unlinkTarget, setUnlinkTarget] = useState<WorkstationLink | null>(null);
  const [unlinking, setUnlinking] = useState(false);

  useEffect(() => {
    if (canManage) {
      load({ agentId });
    }
  }, [agentId, canManage, load]);

  const available = useMemo(
    () => availableWorkstationsForBind(workstations, links),
    [workstations, links],
  );
  const picker = bindPickerView(wsLoading, wsError, available.length);

  if (!canManage) {
    return null;
  }

  async function handleBind() {
    if (!workstationId) return;
    setSaving(true);
    try {
      await linkAgent({ agentId, workstationId, isDefault });
      const next = bindFormAfterSuccess();
      setWorkstationId(next.selectedId);
      setIsDefault(next.isDefault);
      await load({ agentId });
    } catch {
      // toast handled by hook
    } finally {
      setSaving(false);
    }
  }

  async function handleUnlink() {
    if (!unlinkTarget) return;
    setUnlinking(true);
    try {
      await unlinkAgent({ agentId, workstationId: unlinkTarget.workstationId });
      setUnlinkTarget(null);
      await load({ agentId });
    } catch {
      // toast handled by hook
    } finally {
      setUnlinking(false);
    }
  }

  function workstationLabel(link: WorkstationLink): string {
    return link.workstationName || link.workstationKey || link.workstationId;
  }

  return (
    <div className="rounded-lg border p-4 space-y-3">
      <div className="flex items-center gap-2">
        <MonitorCog className="h-4 w-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">{t("bindings.title")}</h3>
      </div>
      <p className="text-xs text-muted-foreground">{t("bindings.agentCardHint")}</p>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          {picker === "loading" ? (
            <p className="text-sm text-muted-foreground">
              <Loader2 className="mr-1 inline h-3.5 w-3.5 animate-spin" />
            </p>
          ) : picker === "error" ? (
            <p className="text-sm text-destructive">{wsError}</p>
          ) : picker === "empty" ? (
            <p className="text-sm text-muted-foreground">{t("bindings.noWorkstations")}</p>
          ) : (
            <Select value={workstationId || undefined} onValueChange={setWorkstationId}>
              <SelectTrigger className="w-full text-base md:text-sm">
                <SelectValue placeholder={t("bindings.workstationPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {available.map((ws) => (
                  <SelectItem key={ws.id} value={ws.id}>
                    {ws.name} ({ws.workstationKey})
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
        <Button size="sm" className="gap-1.5" disabled={!bindButtonEnabled(workstationId, saving)} onClick={handleBind}>
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
          {t("bindings.add")}
        </Button>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}

      {loading && links.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Loader2 className="mr-1 inline h-3.5 w-3.5 animate-spin" />
        </p>
      ) : links.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("bindings.empty")}</p>
      ) : (
        <ul className="space-y-2">
          {links.map((link) => (
            <li key={link.workstationId} className="flex items-center justify-between gap-2 rounded-md border px-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">
                  {link.workstationName || link.workstationKey || link.workstationId}
                </p>
                {link.workstationKey && (
                  <p className="font-mono text-xs text-muted-foreground">{link.workstationKey}</p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-1">
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
                    onClick={async () => {
                      try {
                        await setDefault({ agentId, workstationId: link.workstationId });
                        await load({ agentId });
                      } catch {
                        // toast handled by hook
                      }
                    }}
                  >
                    <Star className="h-3 w-3" />
                    {t("bindings.setDefault")}
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-muted-foreground hover:text-destructive"
                  onClick={() => setUnlinkTarget(link)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {unlinkTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setUnlinkTarget(null)}
          title={t("bindings.unlinkTitle")}
          description={t("bindings.unlinkDescription", { name: workstationLabel(unlinkTarget) })}
          confirmLabel={t("bindings.unlink")}
          variant="destructive"
          loading={unlinking}
          onConfirm={handleUnlink}
        />
      )}
    </div>
  );
}
