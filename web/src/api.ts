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
