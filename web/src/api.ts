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
