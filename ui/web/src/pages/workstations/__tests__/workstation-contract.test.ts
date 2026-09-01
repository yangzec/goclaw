import { describe, expect, it } from "vitest";
import { Methods } from "@/api/protocol";
import {
  buildWorkstationCreatePayload,
  type WorkstationCreateFormState,
} from "../workstation-create-dialog-helpers";
import { buildLinkAgentPayload, type WorkstationLink } from "../hooks/use-workstation-links";
import type { Workstation } from "../hooks/use-workstations";
import {
  availableAgentsForBind,
  availableWorkstationsForBind,
  bindButtonEnabled,
  bindFormAfterSuccess,
  bindPickerView,
  remainingDefaultAfterUnlink,
} from "../binding-results";
import enWorkstations from "@/i18n/locales/en/workstations.json";
import viWorkstations from "@/i18n/locales/vi/workstations.json";
import zhWorkstations from "@/i18n/locales/zh/workstations.json";
import ruWorkstations from "@/i18n/locales/ru/workstations.json";

function form(overrides: Partial<WorkstationCreateFormState> = {}): WorkstationCreateFormState {
  return {
    key: "dev-server",
    name: "Dev Server",
    backend: "ssh",
    host: "192.168.1.100",
    port: "22",
    user: "ubuntu",
    authMethod: "privateKey",
    privateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----",
    password: "",
    container: "",
    image: "",
    socketPath: "",
    ...overrides,
  };
}

describe("workstation create payload contract", () => {
  it("submits camelCase params the gateway handler actually decodes", () => {
    const result = buildWorkstationCreatePayload(form());

    expect(result.kind).toBe("ok");
    if (result.kind !== "ok") return;

    // Regression guard for #1258: snake_case here decodes to the zero value in
    // Go and the handler rejects the request with "workstationKey is required".
    expect(Object.keys(result.payload).sort()).toEqual([
      "backendType",
      "metadata",
      "name",
      "workstationKey",
    ]);
    expect(result.payload.workstationKey).toBe("dev-server");
    expect(result.payload.backendType).toBe("ssh");
    expect(result.payload).not.toHaveProperty("workstation_key");
    expect(result.payload).not.toHaveProperty("backend_type");
  });

  it("sends SSH credentials as privateKey, not the unsupported identity_file", () => {
    const result = buildWorkstationCreatePayload(form());
    if (result.kind !== "ok") throw new Error("expected ok");

    // store.SSHMetadata accepts privateKey or password only; identity_file was
    // silently dropped and left the request failing metadata validation.
    expect(result.payload.metadata).toEqual({
      host: "192.168.1.100",
      port: 22,
      user: "ubuntu",
      privateKey: expect.stringContaining("BEGIN OPENSSH PRIVATE KEY"),
    });
    expect(result.payload.metadata).not.toHaveProperty("identity_file");
  });

  it("sends a password when password auth is selected", () => {
    const result = buildWorkstationCreatePayload(
      form({ authMethod: "password", privateKey: "", password: "hunter2" }),
    );
    if (result.kind !== "ok") throw new Error("expected ok");

    expect(result.payload.metadata).toMatchObject({ password: "hunter2" });
    expect(result.payload.metadata).not.toHaveProperty("privateKey");
  });

  it("rejects SSH auth with no credential rather than letting the backend 400", () => {
    expect(
      buildWorkstationCreatePayload(form({ privateKey: "" })),
    ).toEqual({ kind: "error", errorKey: "sshPrivateKeyRequired" });

    expect(
      buildWorkstationCreatePayload(form({ authMethod: "password", privateKey: "", password: "" })),
    ).toEqual({ kind: "error", errorKey: "sshPasswordRequired" });
  });

  it("maps the Docker container name onto host and requires an image", () => {
    const ok = buildWorkstationCreatePayload(
      form({ backend: "docker", container: "my-box", image: "ubuntu:24.04", socketPath: "" }),
    );
    if (ok.kind !== "ok") throw new Error("expected ok");

    // store.DockerMetadata carries the container name in `host` and requires
    // `image`; the old payload sent {container, docker_host} and always failed.
    expect(ok.payload.metadata).toEqual({ host: "my-box", image: "ubuntu:24.04" });

    expect(
      buildWorkstationCreatePayload(form({ backend: "docker", container: "my-box", image: "" })),
    ).toEqual({ kind: "error", errorKey: "dockerImageRequired" });
  });

  it("requires a workstation key before hitting the network", () => {
    expect(buildWorkstationCreatePayload(form({ key: "  " }))).toEqual({
      kind: "error",
      errorKey: "keyRequired",
    });
  });
});

describe("workstation list contract", () => {
  // Verbatim body of store.Workstation.SanitizedView() as serialized by the
  // gateway, kept complete even though the UI type is a narrower subset — that
  // is what proves the subset is actually assignable from a real response.
  // If the TS interface drifts back to snake_case this stops compiling.
  const apiResponse = {
    id: "8f2b0f7e-1c4a-4c9e-9f1a-2b3c4d5e6f70",
    workstationKey: "aether",
    tenantId: "11111111-2222-3333-4444-555555555555",
    name: "Aether",
    backendType: "ssh",
    defaultCwd: "/home/ubuntu",
    active: true,
    createdAt: "2026-06-22T15:06:38Z",
    updatedAt: "2026-06-22T15:06:38Z",
    createdBy: "admin",
    metadataSummary: { host: "10.0.0.5", port: 22, user: "ubuntu", hasKey: true },
  } as const;

  it("resolves every field the workstations table renders", () => {
    const ws: Workstation = apiResponse;

    // Regression guard for #1258: these read undefined under the old snake_case
    // interface, rendering a blank key, "backend.undefined" and "Invalid Date".
    expect(ws.workstationKey).toBe("aether");
    expect(ws.backendType).toBe("ssh");
    expect(Number.isNaN(new Date(ws.createdAt).getTime())).toBe(false);
    expect(ws.name).toBe("Aether");
    expect(ws.active).toBe(true);
  });
});

describe("workstation binding contract", () => {
  it("uses camelCase method names the gateway router registers", () => {
    // Regression: snake_case constants never reached handleLinkAgent / handleUnlinkAgent.
    expect(Methods.WORKSTATIONS_LINK_AGENT).toBe("workstations.linkAgent");
    expect(Methods.WORKSTATIONS_UNLINK_AGENT).toBe("workstations.unlinkAgent");
    expect(Methods.WORKSTATIONS_LIST_LINKS).toBe("workstations.listLinks");
    expect(Methods.WORKSTATIONS_SET_DEFAULT).toBe("workstations.setDefault");
  });

  it("submits camelCase link params the gateway handler decodes", () => {
    expect(buildLinkAgentPayload({
      agentId: "8f2b0f7e-1c4a-4c9e-9f1a-2b3c4d5e6f70",
      workstationId: "11111111-2222-3333-4444-555555555555",
      isDefault: true,
    })).toEqual({
      agentId: "8f2b0f7e-1c4a-4c9e-9f1a-2b3c4d5e6f70",
      workstationId: "11111111-2222-3333-4444-555555555555",
      isDefault: true,
    });
  });

  it("resolves every field the bindings table renders", () => {
    const apiResponse = {
      agentId: "8f2b0f7e-1c4a-4c9e-9f1a-2b3c4d5e6f70",
      agentKey: "coder",
      displayName: "Coder",
      emoji: "🦊",
      workstationId: "11111111-2222-3333-4444-555555555555",
      workstationKey: "dev-server",
      workstationName: "Dev Server",
      isDefault: true,
      createdAt: "2026-08-31T00:00:00Z",
    } as const;
    const link: WorkstationLink = apiResponse;

    expect(link.agentId).toBe(apiResponse.agentId);
    expect(link.agentKey).toBe("coder");
    expect(link.displayName).toBe("Coder");
    expect(link.workstationKey).toBe("dev-server");
    expect(link.isDefault).toBe(true);
  });

  it("keeps empty-bind copy keys so the dropdown never goes blank", () => {
    // These keys are shown when every agent/workstation is already bound.
    // Missing keys render the raw path and look like a stuck form.
    for (const catalog of [enWorkstations, viWorkstations, zhWorkstations, ruWorkstations]) {
      expect(catalog.bindings.noAgents.length).toBeGreaterThan(0);
      expect(catalog.bindings.noWorkstations.length).toBeGreaterThan(0);
      expect(catalog.liteUnavailableTitle.length).toBeGreaterThan(0);
      expect(catalog.pageToast.deleteFailed.length).toBeGreaterThan(0);
    }
  });
});

describe("binding action results", () => {
  const agents = [
    { id: "a-active", status: "active" },
    { id: "a-inactive", status: "inactive" },
    { id: "a-bound", status: "active" },
  ];
  const workstations = [
    { id: "ws-on", active: true },
    { id: "ws-off", active: false },
    { id: "ws-bound", active: true },
  ];
  const links = [
    { agentId: "a-bound", workstationId: "ws-bound", isDefault: true },
  ];

  it("Bind picker: loading or error is not the empty-bind result", () => {
    expect(bindPickerView(true, null, 0)).toBe("loading");
    expect(bindPickerView(false, "denied", 0)).toBe("error");
    expect(bindPickerView(false, null, 0)).toBe("empty");
    expect(bindPickerView(false, null, 2)).toBe("ready");
  });

  it("Bind picker: only active unbound targets are choosable", () => {
    expect(availableAgentsForBind(agents, links).map((a) => a.id)).toEqual(["a-active"]);
    expect(availableWorkstationsForBind(workstations, links).map((ws) => ws.id)).toEqual(["ws-on"]);
  });

  it("Bind button: disabled until a target is selected and idle", () => {
    expect(bindButtonEnabled("", false)).toBe(false);
    expect(bindButtonEnabled("a-active", true)).toBe(false);
    expect(bindButtonEnabled("a-active", false)).toBe(true);
  });

  it("Bind success: form resets so the same click cannot replay", () => {
    expect(bindFormAfterSuccess()).toEqual({ selectedId: "", isDefault: true });
  });

  it("Bind success: bound target leaves the picker", () => {
    const after = availableAgentsForBind(agents, [
      ...links,
      { agentId: "a-active", workstationId: "ws-on", isDefault: true },
    ]);
    expect(after.map((a) => a.id)).toEqual([]);
  });

  it("Unlink default: remaining link is promoted so exec still has a default", () => {
    const two = [
      { agentId: "coder", workstationId: "ws-a", isDefault: true },
      { agentId: "coder", workstationId: "ws-b", isDefault: false },
    ];
    expect(remainingDefaultAfterUnlink(two, "ws-a")).toBe("ws-b");
    expect(remainingDefaultAfterUnlink(two, "ws-b")).toBe("ws-a");
    expect(remainingDefaultAfterUnlink(two.slice(0, 1), "ws-a")).toBeNull();
  });

  it("Set default: payload is the selected pair (camelCase)", () => {
    expect(buildLinkAgentPayload({
      agentId: "a-active",
      workstationId: "ws-on",
      isDefault: true,
    })).toEqual({
      agentId: "a-active",
      workstationId: "ws-on",
      isDefault: true,
    });
  });
});
