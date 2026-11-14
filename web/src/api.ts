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
export async function loadAssessment(
  monitor: string,
  token: string,
  signal: AbortSignal,
): Promise<Assessment> {
  const response = await fetch(
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
  const response = await fetch("/api/v1/auth/me", {
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
  const response = await fetch("/api/v1/auth/login", {
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
  const response = await fetch("/api/v1/auth/logout", {
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
  const response = await fetch("/api/v1/auth/change-password", {
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
  const response = await fetch(
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
  const response = await fetch(`/api/v1/auth/tokens?${params}`, {
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
  const response = await fetch("/api/v1/auth/tokens", {
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
  const response = await fetch(
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
  const response = await fetch(`/api/v1/users?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
  });
  if (!response.ok) throw userError(response.status);
  return response.json() as Promise<UserPage>;
}
export async function user(id: string): Promise<VersionedUser> {
  return versionedUser(
    await fetch(`/api/v1/users/${encodeURIComponent(id)}`, {
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
    await fetch("/api/v1/users", {
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
    await fetch(`/api/v1/users/${encodeURIComponent(selected.user.id)}`, {
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
    await fetch(`/api/v1/resources/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`, {
      credentials: "same-origin",
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
    }),
  );
}
export async function resourceCatalog(
  kind: string,
  cursor: string | null = null,
): Promise<{ items: MonitorResource[]; nextCursor: string | null }> {
  const query = new URLSearchParams({ limit: "20" });
  if (cursor) query.set("cursor", cursor);
  const response = await fetch(`/api/v1/resources/${encodeURIComponent(kind)}?${query}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json();
}
export async function checkConfiguration(
  session: Session,
  manifest: ResourceManifest,
): Promise<ConfigValidation> {
  const response = await fetch("/api/v1/config/validate", {
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
  const response = await fetch("/api/v1/config/plan", {
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
    await fetch(path, {
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
  const response = await fetch(
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
  const response = await fetch("/api/v1/config/apply", {
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
  const response = await fetch("/api/v1/config/validate", {
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
  const response = await fetch(`/api/v1/operations/${encodeURIComponent(id)}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<Operation>;
}
export async function operations(
  state: string,
  cursor: string | null,
): Promise<OperationPage> {
  const params = new URLSearchParams({ limit: "20" });
  if (state) params.set("state", state);
  if (cursor) params.set("cursor", cursor);
  const response = await fetch(`/api/v1/operations?${params}`, {
    credentials: "same-origin",
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw resourceError(response.status);
  return response.json() as Promise<OperationPage>;
}

export interface AgentView {
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
  const response = await fetch(`/api/v1/agents?${params}`, {
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

export interface DiagnosticSession {
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
  const response = await fetch(path, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) });
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
  const response = await fetch(`/api/v1/monitors/${encodeURIComponent(monitor)}/diagnostics`, {
    method: "POST", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken },
    body: JSON.stringify({ profileRef, reason: "manual", requestId }), signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) throw new Error(`Запуск не підтверджено (${response.status}). Оновіть стан; для відновлення повторіть незмінний запит.`);
  return response.json();
}
export async function cancelDiagnostic(session: Session, id: string): Promise<DiagnosticSession> {
  const response = await fetch(`/api/v1/diagnostics/${encodeURIComponent(id)}/cancel`, {
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
async function alertRead<T>(path: string): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) });
  if (!response.ok) throw new Error(`Історія алертів недоступна (${response.status}). Оновіть запит.`);
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
  return alertVersion(await fetch(`/api/v1/alerts/${encodeURIComponent(id)}`, { credentials: "same-origin", cache: "no-store", signal: AbortSignal.timeout(8000) }));
}
export async function acknowledgeAlert(session: Session, id: string, etag: string, note: string): Promise<VersionedAlert> {
  return alertVersion(await fetch(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`, {
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
