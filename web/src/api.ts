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
export async function loadAssessment(
  monitor: string,
  token: string,
  signal: AbortSignal,
): Promise<Assessment> {
  const response = await fetch(
    `/api/v1/monitors/${encodeURIComponent(monitor)}/assessment`,
    {
      headers: { Authorization: `Bearer ${token}` },
      signal: AbortSignal.any([signal, AbortSignal.timeout(8000)]),
      cache: "no-store",
      credentials: "omit",
    },
  );
  if (!response.ok) {
    if (response.status === 401)
      throw new Error("Доступ відхилено. Перевірте токен читання.");
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
