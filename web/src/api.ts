export type ResourceState = "healthy" | "degraded" | "down" | "unknown";
export interface ZoneAssessment {
  zone: string;
  state: ResourceState;
  coverage: string;
  q: number;
  h: number | null;
  evidence: string[];
  reason: string;
}
export interface Assessment {
  monitorId: string;
  state: ResourceState;
  coverageState: "sufficient" | "degraded" | "unknown";
  zoneStates: ZoneAssessment[];
  reasonCodes: string[];
  evidenceIds: string[];
  evaluatedAt: string;
  revision: string;
}
export class AccessError extends Error {}

// Read-only requests can overlap at startup or meet a short reconciliation gap.
// A single 8s budget includes all attempts. Never replay mutations or auth errors.
// Serialize reads from this browser tab to respect the small Controller's
// admission limits. Release at headers, before any retry delay or body parsing.
let readTail: Promise<void> = Promise.resolve();
async function readFetch(path: string, init: RequestInit): Promise<Response> {
  const previous = readTail;
  let release!: () => void;
  readTail = new Promise<void>(resolve => { release = resolve; });
  await previous;
  try {
    init.signal?.throwIfAborted();
    return await fetch(path, init);
  } finally { release(); }
}
async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  if ((init.method ?? "GET").toUpperCase() !== "GET") return fetch(path, init);
  const timeout = AbortSignal.timeout(8000);
  const signal = init.signal ? AbortSignal.any([init.signal, timeout]) : timeout;
  for (let attempt = 0; ; attempt++) {
    let delay = 250 * (attempt + 1);
    try {
      const response = await readFetch(path, { ...init, signal });
      if (attempt >= 2 || (response.status !== 429 && response.status !== 503)) return response;
      const seconds = Number(response.headers.get("Retry-After"));
      if (Number.isFinite(seconds) && seconds > 0) delay = Math.min(1000, seconds * 1000);
      await response.body?.cancel();
    } catch (error) {
      if (signal.aborted || attempt >= 2 || !(error instanceof TypeError)) throw error;
    }
    await new Promise<void>((resolve, reject) => {
      if (signal.aborted) { reject(signal.reason); return; }
      const abort = () => { clearTimeout(timer); reject(signal.reason); };
      const timer = setTimeout(() => { signal.removeEventListener("abort", abort); resolve(); }, delay);
      signal.addEventListener("abort", abort, { once: true });
    });
  }
}
export async function loadAssessment(
  monitor: string,
  token: string,
  signal: AbortSignal,
): Promise<Assessment> {
  const response = await apiFetch(
    `/api/v1/monitors/${encodeURIComponent(monitor)}/assessment`,
    {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
      cache: "no-store",
      credentials: token ? "omit" : "same-origin",
    },
  );
  if (!response.ok) {
    if (response.status === 401)
      throw new AccessError(
        token
          ? "Доступ відхилено. Перевірте токен читання."
          : "Сесію завершено. Увійдіть повторно.",
      );
    if (response.status === 503)
      throw new Error("Свіжа оцінка тимчасово недоступна.");
    throw new Error(`REST API повернув ${response.status}.`);
  }
  const data = (await response.json()) as Assessment;
  if (
    !Array.isArray(data.zoneStates) ||
    !["healthy", "degraded", "down", "unknown"].includes(data.state) ||
    typeof data.evaluatedAt !== "string"
  )
    throw new Error("Некоректна відповідь API.");
  return data;
}

export interface Session {
  userId: string;
  username: string;
  role: "admin" | "viewer";
  expiresAt: string;
  csrfToken: string;
  scopes: string[];
}
export async function sessionInfo(): Promise<Session | null> {
  const response = await apiFetch("/api/v1/auth/me", {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (response.status === 401) return null;
  if (!response.ok) throw new Error("Сервіс входу тимчасово недоступний.");
  return response.json() as Promise<Session>;
}
export async function login(
  username: string,
  password: string,
): Promise<Session> {
  const response = await apiFetch("/api/v1/auth/login", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
    signal: AbortSignal.timeout(8000),
  });
  if (response.status === 401)
    throw new Error("Невірні облікові дані або вхід тимчасово заблоковано.");
  if (response.status === 429)
    throw new Error("Забагато спроб входу. Спробуйте пізніше.");
  if (!response.ok) throw new Error("Не вдалося виконати вхід.");
  return response.json() as Promise<Session>;
}
export async function logout(session: Session): Promise<void> {
  const response = await apiFetch("/api/v1/auth/logout", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": session.csrfToken },
    signal: AbortSignal.timeout(8000),
  });
  if (response.status !== 204 && response.status !== 401)
    throw new Error("Вихід не підтверджено сервером. Спробуйте ще раз.");
}

export async function changePassword(
  session: Session,
  currentPassword: string,
  newPassword: string,
): Promise<Session> {
  const response = await apiFetch("/api/v1/auth/change-password", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify({ currentPassword, newPassword }),
    signal: AbortSignal.timeout(8000),
  });
  if (response.status === 401)
    throw new Error("Перевірте поточний пароль і чинність сесії.");
  if (response.status === 422)
    throw new Error("Новий пароль має містити від 16 до 256 байтів UTF-8.");
  if (response.status === 429)
    throw new Error("Забагато спроб. Спробуйте пізніше.");
  if (!response.ok)
    throw new Error(
      "Зміну пароля не підтверджено. Перевірте з’єднання та увійдіть повторно.",
    );
  return response.json() as Promise<Session>;
}

export interface ObservationRecord {
  observationId: string;
  monitorId: string;
  checkId: string;
  agentId: string;
  networkZone: string;
  configRevision: string;
  executionStatus: string;
  probeOutcome: string;
  reasonCode?: string;
  startedAt: string;
  finishedAt: string;
  receivedAt: string;
  durationMs: number;
  values?: Record<string, string | number | boolean>;
  diagnosticSessionId?: string;
  diagnosticTaskId?: string;
}
export interface ObservationPage {
  items: ObservationRecord[];
  nextCursor: string | null;
}
export async function loadHistory(
  monitor: string,
  zone: string,
  cursor: string | null,
  signal: AbortSignal,
): Promise<ObservationPage> {
  const params = new URLSearchParams({ limit: "50" });
  if (zone) params.set("zone", zone);
  if (cursor) params.set("cursor", cursor);
  const response = await apiFetch(
    `/api/v1/monitors/${encodeURIComponent(monitor)}/observations?${params}`,
    {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
    },
  );
  if (response.status === 401)
    throw new AccessError("Сесію завершено. Увійдіть повторно.");
  if (response.status === 429)
    throw new Error("Сервер зайнятий. Повторіть оновлення історії.");
  if (!response.ok)
    throw new Error(`Історія тимчасово недоступна (${response.status}).`);
  const page = (await response.json()) as ObservationPage;
  if (
    !Array.isArray(page.items) ||
    (page.nextCursor !== null && typeof page.nextCursor !== "string")
  )
    throw new Error("Некоректна відповідь історії.");
  return page;
}

export interface ApiToken {
  id: string;
  name: string;
  scopes: string[];
  expiresAt: string | null;
  createdAt: string;
  revokedAt: string | null;
}
export interface TokenPage {
  items: ApiToken[];
  nextCursor: string | null;
}
export interface IssuedToken extends ApiToken {
  token: string;
}
export async function tokens(
  cursor: string | null,
  signal: AbortSignal,
): Promise<TokenPage> {
  const params = new URLSearchParams({ limit: "50" });
  if (cursor) params.set("cursor", cursor);
  const response = await apiFetch(`/api/v1/auth/tokens?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
  });
  if (!response.ok)
    throw new Error(`Не вдалося отримати токени (${response.status}).`);
  return response.json() as Promise<TokenPage>;
}
export async function issueToken(
  session: Session,
  name: string,
  scopes: string[],
  expiresAt: string | null,
): Promise<IssuedToken> {
  const response = await apiFetch("/api/v1/auth/tokens", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify({ name, scopes, expiresAt }),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok)
    throw new Error(
      `Видачу токена не підтверджено (${response.status}). Оновіть список перед повторною спробою.`,
    );
  return response.json() as Promise<IssuedToken>;
}
export async function revokeToken(session: Session, id: string): Promise<void> {
  const response = await apiFetch(
    `/api/v1/auth/tokens/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      headers: { "X-CSRF-Token": session.csrfToken },
      signal: AbortSignal.timeout(8000),
    },
  );
  if (response.status !== 204)
    throw new Error(
      `Відкликання не підтверджено (${response.status}). Оновіть список.`,
    );
}

export interface UserView {
  id: string;
  username: string;
  role: "admin" | "viewer";
  disabled: boolean;
  createdAt?: string;
}
export interface UserPage {
  items: UserView[];
  nextCursor: string | null;
}
export interface VersionedUser {
  user: UserView;
  etag: string;
}
function userError(status: number): Error {
  switch (status) {
    case 401:
      return new Error("Сесію завершено. Увійдіть повторно.");
    case 403:
      return new Error("Потрібні права адміністратора та чинний CSRF-токен.");
    case 409:
      return new Error(
        "Назва користувача вже зайнята або зміна залишить систему без активного адміністратора.",
      );
    case 412:
      return new Error(
        "Користувача вже змінено. Повторно відкрийте його запис і перевірте поля.",
      );
    case 422:
      return new Error(
        "Перевірте ім’я, роль та пароль (16..256 байтів UTF-8).",
      );
    case 429:
      return new Error("Сервер зайнятий. Повторіть запит пізніше.");
    default:
      return new Error(
        `Операцію з користувачами не підтверджено (${status}). Оновіть список.`,
      );
  }
}
async function versionedUser(response: Response): Promise<VersionedUser> {
  if (!response.ok) throw userError(response.status);
  const etag = response.headers.get("ETag");
  if (!etag) throw new Error("Сервер не повернув версію користувача.");
  return { user: (await response.json()) as UserView, etag };
}
export async function users(
  cursor: string | null,
  signal: AbortSignal,
): Promise<UserPage> {
  const params = new URLSearchParams({ limit: "50" });
  if (cursor) params.set("cursor", cursor);
  const response = await apiFetch(`/api/v1/users?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
  });
  if (!response.ok) throw userError(response.status);
  return response.json() as Promise<UserPage>;
}
export async function user(id: string): Promise<VersionedUser> {
  return versionedUser(
    await apiFetch(`/api/v1/users/${encodeURIComponent(id)}`, {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
    }),
  );
}
export async function createUser(
  session: Session,
  username: string,
  role: string,
  initialPassword: string,
): Promise<VersionedUser> {
  return versionedUser(
    await apiFetch("/api/v1/users", {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrfToken,
      },
      body: JSON.stringify({ username, role, initialPassword }),
      signal: AbortSignal.timeout(8000),
    }),
  );
}
export async function patchUser(
  session: Session,
  selected: VersionedUser,
  role: string,
  disabled: boolean,
): Promise<VersionedUser> {
  return versionedUser(
    await apiFetch(`/api/v1/users/${encodeURIComponent(selected.user.id)}`, {
      method: "PATCH",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrfToken,
        "If-Match": selected.etag,
      },
      body: JSON.stringify({ role, disabled }),
      signal: AbortSignal.timeout(8000),
    }),
  );
}

export interface ResourceManifest {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: Record<string, unknown>;
}
export interface MonitorResource {
  manifest: ResourceManifest;
  uid: string;
  generation: number;
  resourceVersion: string;
  createdAt: string;
  updatedAt: string;
  status?: {
    state: string;
    desiredRevision: string;
    conditions: { type: string; status: string; reason: string }[];
  };
}
export interface VersionedResource {
  resource: MonitorResource;
  etag: string;
}
export interface ConfigValidation {
  valid: boolean;
  errors: { code: string; message: string; path?: string }[];
  warnings: { code: string; message: string }[];
  normalizedSummary: Record<string, unknown>;
}
export interface ConfigPlan {
  planId: string;
  diffs: {
    kind: string;
    name: string;
    action: string;
    resourceVersion: string | null;
    effectiveRevision: string;
  }[];
  warnings: { code: string; message: string }[];
  placements?: {
    monitorId: string;
    strategy: "fixed";
    replicas: number;
    pending: boolean;
    selectedAgents: { agentId: string; networkZone: string }[];
  }[];
}
function resourceError(status: number): Error {
  if (status === 412)
    return new Error(
      "Конфігурацію вже змінено. Завантажте актуальну версію перед повторним збереженням.",
    );
  if (status === 409)
    return new Error("Ресурс уже існує або його використовує Controller.");
  if (status === 422)
    return new Error(
      "Маніфест не підтримується або містить помилку. Виконайте серверну перевірку.",
    );
  if (status === 401) return new Error("Увійдіть повторно.");
  return new Error(
    `Операцію з конфігурацією не підтверджено (${status}). Оновіть запис.`,
  );
}
async function resourceResponse(
  response: Response,
): Promise<VersionedResource> {
  if (!response.ok) throw resourceError(response.status);
  const etag = response.headers.get("ETag");
  if (!etag) throw new Error("Відсутня версія ресурсу.");
  return { resource: (await response.json()) as MonitorResource, etag };
}
export async function monitorResource(
  name: string,
  kind = "Monitor",
): Promise<VersionedResource> {
  return resourceResponse(
    await apiFetch(`/api/v1/resources/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`, {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
    }),
  );
}
export async function resourceCatalog(
  kind: string,
  cursor: string | null = null,
  signal?: AbortSignal,
): Promise<{ items: MonitorResource[]; nextCursor: string | null }> {
  const query = new URLSearchParams({ limit: "20" });
  if (cursor) query.set("cursor", cursor);
  const response = await apiFetch(`/api/v1/resources/${encodeURIComponent(kind)}?${query}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(8000)]) : AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json();
}
export async function checkConfiguration(
  session: Session,
  manifest: ResourceManifest,
): Promise<ConfigValidation> {
  const response = await apiFetch("/api/v1/config/validate", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify({ manifests: [manifest] }),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<ConfigValidation>;
}
export async function planConfiguration(
  session: Session,
  manifest: ResourceManifest,
): Promise<ConfigPlan> {
  return planBatchConfiguration(session, { manifests: [manifest] });
}
export async function planBatchConfiguration(
  session: Session,
  batch: ConfigBatch,
): Promise<ConfigPlan> {
  const response = await apiFetch("/api/v1/config/plan", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify(batch),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<ConfigPlan>;
}
export async function saveMonitor(
  session: Session,
  manifest: ResourceManifest,
  current: VersionedResource | null,
): Promise<VersionedResource> {
  const path =
    "/api/v1/resources/" + encodeURIComponent(manifest.kind) +
    (current
      ? "/" + encodeURIComponent(current.resource.manifest.metadata.name)
      : "");
  return resourceResponse(
    await apiFetch(path, {
      method: current ? "PUT" : "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": session.csrfToken,
        ...(current ? { "If-Match": current.etag } : {}),
      },
      body: JSON.stringify(manifest),
      signal: AbortSignal.timeout(8000),
    }),
  );
}
export async function deleteMonitor(
  session: Session,
  current: VersionedResource,
): Promise<void> {
  const response = await apiFetch(
    `/api/v1/resources/${encodeURIComponent(current.resource.manifest.kind)}/${encodeURIComponent(current.resource.manifest.metadata.name)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      headers: { "X-CSRF-Token": session.csrfToken, "If-Match": current.etag },
      signal: AbortSignal.timeout(8000),
    },
  );
  if (response.status !== 204) throw resourceError(response.status);
}

export interface ConfigBatch {
  manifests: ResourceManifest[];
  expectedVersions?: Record<string, string | null>;
  requestId?: string;
}
export interface ApplyTarget {
  implicit?: boolean;
  kind: string;
  name: string;
  uid: string;
  resourceVersion: string;
  effectiveRevision: string;
  ready: boolean;
  reason: string;
}
export interface Operation {
  id: string;
  kind: string;
  state: "accepted" | "running" | "completed" | "failed";
  startedAt: string;
  finishedAt: string | null;
  error: { code: string; message: string } | null;
  result: { desiredCommitted: boolean; targets: ApplyTarget[] };
}
export interface OperationPage {
  items: Operation[];
  nextCursor: string | null;
}
export async function applyConfiguration(
  session: Session,
  batch: ConfigBatch,
): Promise<Operation> {
  const response = await apiFetch("/api/v1/config/apply", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify(batch),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    if (response.status === 428)
      throw new Error(
        "Для наявних Monitor потрібні поточні expectedVersions. Завантажте ресурси знову.",
      );
    if (response.status === 409)
      throw new Error(
        "Конфлікт create-only або requestId уже належить іншому запиту. Перевірте пакет і версії.",
      );
    throw resourceError(response.status);
  }
  return response.json() as Promise<Operation>;
}
export async function validateBatch(
  session: Session,
  batch: ConfigBatch,
): Promise<ConfigValidation> {
  const response = await apiFetch("/api/v1/config/validate", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": session.csrfToken,
    },
    body: JSON.stringify(batch),
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<ConfigValidation>;
}
export async function operation(id: string): Promise<Operation> {
  const response = await apiFetch(`/api/v1/operations/${encodeURIComponent(id)}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  const value = await response.json() as Operation;
  if (value.kind !== "config.apply") throw new Error("Ця операція належить іншому розділу.");
  return value;
}
export async function operations(
  state: string,
  cursor: string | null,
): Promise<OperationPage> {
  const params = new URLSearchParams({ limit: "20" });
  if (state) params.set("state", state);
  if (cursor) params.set("cursor", cursor);
  const response = await apiFetch(`/api/v1/operations?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  const value = await response.json() as OperationPage;
  value.items = value.items.filter(item => item.kind === "config.apply");
  return value;
}

export interface AgentView {
  failureDomain?: string;
  regularMonitoringAssigned?: boolean;
  agentId: string;
  name: string;
  onlineState: "online" | "offline" | "degraded" | "revoked";
  networkZone: string;
  capabilities?: string[];
  appliedRevision: string | null;
  lastHeartbeat: string | null;
  certExpiresAt: string | null;
  maxConcurrentProbes: number | null;
  activeProbes: number | null;
}
export interface AgentPage {
  items: AgentView[];
  nextCursor: string | null;
}
export async function agents(
  zone: string,
  state: string,
  cursor: string | null,
): Promise<AgentPage> {
  const params = new URLSearchParams({ limit: "50" });
  if (zone) params.set("zone", zone);
  if (state) params.set("state", state);
  if (cursor) params.set("cursor", cursor);
  const response = await apiFetch(`/api/v1/agents?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok)
    throw new Error(
      `Не вдалося оновити агентів (${response.status}). Повторіть запит.`,
    );
  return response.json() as Promise<AgentPage>;
}

export interface DiagnosticExplanation {
  durationMs?: number;
  costUnits: number;
  decisions: {
    actionId: string; agentId: string; failureDomain?: string; attemptNo?: number; phase?: string;
    hypothesesBefore: string[] | null; gain: number; score: number;
    executionStatus: string; probeOutcome: string;
  }[];
  confirmation?: {
    requiredActions: string[]; observedActions: string[];
    observations: number; distinctFailureDomains: number; satisfied: boolean;
    evidenceDomains?: Record<string, string>;
  };
  blockedSteps?: string[];
  skippedSteps?: string[];
}
export interface DiagnosticSession {
  explanation?: DiagnosticExplanation;
  configRevision?: string;
  id: string;
  monitorId: string;
  profileRef?: string;
  mode: "static" | "fixed" | "adaptive";
  state: "created" | "running" | "completed" | "inconclusive" | "cancelled";
  startedAt: string;
  finishedAt: string | null;
  remainingHypotheses?: string[];
  budgetUsed: { additionalChecks: number };
  evidenceIds: string[];
  stopReason: string | null;
}
export interface DiagnosticTask {
  id: string;
  sessionId: string;
  actionId: string;
  agentId: string;
  state: string;
  attemptNo: number;
  deadline: string;
  observationIds: string[];
}
export interface DiagnosticPage { items: DiagnosticSession[]; nextCursor: string | null }
async function diagnosticRead<T>(path: string): Promise<T> {
  const response = await apiFetch(path, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) });
  if (!response.ok) throw new Error(`Діагностика недоступна (${response.status}). Оновіть запит.`);
  return response.json() as Promise<T>;
}
export function diagnosticSessions(monitor: string, state: string, cursor: string | null): Promise<DiagnosticPage> {
  const query = new URLSearchParams({ monitor, limit: "20" });
  if (state) query.set("state", state);
  if (cursor) query.set("cursor", cursor);
  return diagnosticRead(`/api/v1/diagnostics?${query}`);
}
export function diagnosticSession(id: string): Promise<DiagnosticSession> {
  return diagnosticRead(`/api/v1/diagnostics/${encodeURIComponent(id)}`);
}
export function diagnosticTasks(id: string, cursor: string | null): Promise<{items: DiagnosticTask[]; nextCursor: string | null}> {
  const query = new URLSearchParams({ limit: "20" });
  if (cursor) query.set("cursor", cursor);
  return diagnosticRead(`/api/v1/diagnostics/${encodeURIComponent(id)}/tasks?${query}`);
}

export async function startDiagnostic(session: Session, monitor: string, profileRef: string, requestId: string): Promise<DiagnosticSession> {
  const response = await apiFetch(`/api/v1/monitors/${encodeURIComponent(monitor)}/diagnostics`, {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken },
    body: JSON.stringify({ profileRef, reason: "manual", requestId }), signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw new Error(`Запуск не підтверджено (${response.status}). Оновіть стан; для відновлення повторіть незмінний запит.`);
  return response.json();
}
export async function cancelDiagnostic(session: Session, id: string): Promise<DiagnosticSession> {
  const response = await apiFetch(`/api/v1/diagnostics/${encodeURIComponent(id)}/cancel`, {
    method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": session.csrfToken }, signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw new Error(`Скасування не підтверджено (${response.status}). Оновіть стан сесії.`);
  return response.json();
}

export interface AlertView {
  id: string; monitorId: string; ruleRef: string; zone: string | null;
  state: "normal" | "pending" | "firing" | "resolved";
  severity: "info" | "warning" | "critical";
  reasonCodes: string[]; evidenceIds: string[];
  startedAt?: string; firedAt: string | null; resolvedAt: string | null;
  acknowledgedAt: string | null;
}
export interface AlertPage { items: AlertView[]; nextCursor: string | null }
export interface AlertHistoryPage { items: {id: string; from: string; to: string; at: string; configRevision: string; suppressed: boolean}[]; nextCursor: string | null }
export interface DeliveryPage { items: {id: string; eventId: string; state: string; attempts: number; lastHttpStatus: number | null; lastAttemptAt: string | null}[]; nextCursor: string | null }
async function alertRead<T>(path: string, label = "Історія алертів"): Promise<T> {
  const response = await apiFetch(path, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) });
  if (!response.ok) throw new Error(`${label} недоступна (${response.status}). Оновіть запит.`);
  return response.json() as Promise<T>;
}
export function alerts(monitor: string, state: string, cursor: string | null): Promise<AlertPage> {
  const q = new URLSearchParams({ monitor, limit: "20" }); if (state) q.set("state", state); if (cursor) q.set("cursor", cursor);
  return alertRead(`/api/v1/alerts?${q}`);
}
export interface VersionedAlert { alert: AlertView; etag: string }
async function alertVersion(response: Response): Promise<VersionedAlert> {
  if (!response.ok) {
    if (response.status === 412) throw new Error("Алерт змінився. Оновіть деталі перед підтвердженням.");
    if (response.status === 409) throw new Error("Алерт більше не активний або вже підтверджений з іншою приміткою.");
    throw new Error(`Операцію з алертом не підтверджено (${response.status}).`);
  }
  const etag = response.headers.get("ETag");
  if (!etag) throw new Error("Відсутня версія алерта.");
  return { alert: await response.json() as AlertView, etag };
}
export async function alertDetail(id: string): Promise<VersionedAlert> {
  return alertVersion(await apiFetch(`/api/v1/alerts/${encodeURIComponent(id)}`, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) }));
}
export async function acknowledgeAlert(session: Session, id: string, etag: string, note: string): Promise<VersionedAlert> {
  return alertVersion(await apiFetch(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`, {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken, "If-Match": etag },
    body: JSON.stringify({ note }), signal: AbortSignal.timeout(8000),
  }));
}
export function alertHistory(id: string, cursor: string | null): Promise<AlertHistoryPage> {
  const q = new URLSearchParams({ limit: "20" }); if (cursor) q.set("cursor", cursor);
  return alertRead(`/api/v1/alerts/${encodeURIComponent(id)}/history?${q}`);
}
export function webhookDeliveries(state: string, cursor: string | null): Promise<DeliveryPage> {
  const q = new URLSearchParams({ limit: "20" }); if (state) q.set("state", state); if (cursor) q.set("cursor", cursor);
  return alertRead(`/api/v1/webhook/deliveries?${q}`);
}

export interface WebhookTestOperation {
  id: string; kind: "webhook.test"; state: "accepted" | "running" | "completed" | "failed";
  startedAt: string; finishedAt: string | null;
  error: { code: string; message: string } | null;
  result: { eventId: string; deliveryState: string; attempts: number; lastHttpStatus: number | null };
}
export async function testWebhook(session: Session): Promise<WebhookTestOperation> {
  const response = await apiFetch("/api/v1/webhook/test", {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken },
    body: "{}", signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw new Error(`Тест не поставлено в чергу (${response.status}). Перевірте поточну операцію та конфігурацію webhook.`);
  return response.json() as Promise<WebhookTestOperation>;
}
export async function webhookTestOperation(id: string): Promise<WebhookTestOperation> {
  const value = await alertRead<WebhookTestOperation>(`/api/v1/operations/${encodeURIComponent(id)}`);
  if (value.kind !== "webhook.test") throw new Error("Операція не є тестом webhook.");
  return value;
}

export interface DashboardSummary {
  observedAt: string;
  monitorStates?: Record<string, number>;
  coverageStates?: Record<string, number>;
  agentCounts?: Record<string, number>;
  firingAlerts?: number;
  recentDiagnostics?: DiagnosticSession[];
}
export function dashboardSummary(): Promise<DashboardSummary> {
  return alertRead("/api/v1/dashboard/summary", "Оглядова панель");
}

export async function loadObservation(id: string, signal: AbortSignal): Promise<ObservationRecord> {
  const response = await apiFetch(`/api/v1/observations/${encodeURIComponent(id)}`, {
    credentials: "same-origin", cache: "no-store",
    signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
  });
  if (response.status === 401) throw new AccessError("Сесію завершено. Увійдіть повторно.");
  if (response.status === 403) throw new Error("Немає права читання спостережень.");
  if (response.status === 404) throw new Error("Спостереження вже недоступне.");
  if (!response.ok) throw new Error(`Доказ тимчасово недоступний (${response.status}).`);
  return response.json() as Promise<ObservationRecord>;
}

export interface GroupAssessment {
  groupName: string;
  mode: "context" | "derived";
  state: "healthy" | "degraded" | "down" | "unknown" | null;
  coverageState: "sufficient" | "degraded" | "unknown";
  members: { monitorId: string; state: "healthy" | "degraded" | "down" | "unknown"; coverageState: string; revision: string }[];
  evidenceIds: string[];
}
export async function groupAssessment(name: string): Promise<GroupAssessment> {
  const response = await apiFetch(`/api/v1/resource-groups/${encodeURIComponent(name)}/assessment`, {
    credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<GroupAssessment>;
}

// Export exact observation JSON text so int64 values survive browser download.
// Sequential reads; never infer absent observations or replay a mutation.
export async function exportDiagnostic(id: string, signal: AbortSignal): Promise<string> {
  const deadline = AbortSignal.any([signal, AbortSignal.timeout(120000)]);
  let bytes = 0;
  async function read(path: string, missingAllowed = false): Promise<string | null> {
    const response = await apiFetch(path, { credentials: "same-origin", cache: "no-store", signal: deadline });
    if (missingAllowed && response.status === 404) return null;
    if (!response.ok) throw new Error(`Експорт не завершено (${response.status}).`);
    const text = await response.text();
    bytes += new TextEncoder().encode(text).length;
    if (bytes > 32 * 1024 * 1024) throw new Error("Пакет перевищує 32 MiB.");
    return text;
  }
  const path = `/api/v1/diagnostics/${encodeURIComponent(id)}`;
  const first = (await read(path))!;
  const session = JSON.parse(first) as DiagnosticSession;
  if (session.id !== id || !["completed", "inconclusive", "cancelled"].includes(session.state))
    throw new Error("Для експорту потрібна завершена сесія.");
  const tasks: DiagnosticTask[] = [];
  const taskIds = new Set<string>();
  const expectedTasks = new Map<string, DiagnosticTask>();
  const cursors = new Set<string>();
  const ids = new Set(session.evidenceIds);
  let cursor: string | null = null;
  do {
    const query = new URLSearchParams({ limit: "200" });
    if (cursor) query.set("cursor", cursor);
    const page = JSON.parse((await read(`${path}/tasks?${query}`))!) as { items: DiagnosticTask[]; nextCursor: string | null };
    for (const task of page.items) {
      if (!task.id || task.sessionId !== id || taskIds.has(task.id)) throw new Error("Невідповідність завдання сесії.");
      taskIds.add(task.id); tasks.push(task);
      task.observationIds.forEach(value => {
        if (expectedTasks.has(value) && expectedTasks.get(value)!.id !== task.id) throw new Error("Доказ належить різним завданням.");
        expectedTasks.set(value, task); ids.add(value);
      });
    }
    if (tasks.length > 1000 || ids.size > 10000) throw new Error("Завелика кількість завдань або доказів.");
    cursor = page.nextCursor;
    if (cursor !== null) {
      if (!cursor || cursors.has(cursor) || !page.items.length) throw new Error("Сторінки завдань повторюються.");
      cursors.add(cursor);
    }
  } while (cursor !== null);
  const observations: string[] = [];
  const missing: string[] = [];
  for (const observationId of ids) {
    const raw = await read(`/api/v1/observations/${encodeURIComponent(observationId)}`, true);
    if (raw === null) { missing.push(observationId); continue; }
    const value = JSON.parse(raw) as ObservationRecord;
    if (value.observationId !== observationId || value.monitorId !== session.monitorId || value.diagnosticSessionId !== id)
      throw new Error("Походження доказу не відповідає сесії.");
    const task = expectedTasks.get(observationId);
    if (task && (value.diagnosticTaskId !== task.id || value.agentId !== task.agentId)) throw new Error("Походження доказу не відповідає фізичному завданню.");
    observations.push(raw);
  }
  if (first !== await read(path)) throw new Error("Сесія змінилася під час експорту. Повторіть запит.");
  const result = `{"schema":1,"collectedAt":${JSON.stringify(new Date().toISOString())},"complete":${missing.length === 0},"session":${first},"tasks":${JSON.stringify(tasks)},"observations":[${observations.join(",")}],"missingObservationIds":${JSON.stringify(missing)}}`;
  if (new TextEncoder().encode(result).length > 32 * 1024 * 1024) throw new Error("Пакет перевищує 32 MiB.");
  return result;
}
