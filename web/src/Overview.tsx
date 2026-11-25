import { useEffect, useState } from "react";
import { dashboardSummary, type DashboardSummary } from "./api";
const names: Record<string, string> = {
  healthy: "Працюють",
  degraded: "Потребують уваги",
  down: "Відмова",
  unknown: "Недостатньо доказів",
  sufficient: "Достатнє покриття",
  online: "Підключені",
  offline: "Без підключення",
  revoked: "Відкликані",
};
export function Overview({
  canReadAgents,
  canReadDiagnostics,
}: {
  canReadAgents: boolean;
  canReadDiagnostics: boolean;
}) {
  const [data, setData] = useState<DashboardSummary | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function refresh() {
    setBusy(true);
    setError("");
    try {
      setData(await dashboardSummary());
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Огляд недоступний. Повторіть запит.",
      );
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  const total = data?.monitorStates
    ? Object.values(data.monitorStates).reduce((a, b) => a + b, 0)
    : undefined;
  const metrics = [
    {
      label: "Monitor у каталозі",
      value: total,
      hint: "Ресурси поточної інсталяції",
      icon: "◈",
    },
    {
      label: "Працюють",
      value: data?.monitorStates?.healthy,
      hint: "Підтверджена доступність",
      icon: "✓",
    },
    {
      label: "Агенти online",
      value: data?.agentCounts?.online,
      hint: "Активні джерела перевірок",
      icon: "⇄",
    },
    {
      label: "Активні алерти",
      value: data?.firingAlerts,
      hint: "Потребують уваги оператора",
      icon: "!",
    },
  ];
  return (
    <div aria-busy={busy}>
      <div className="overview-header">
        <h2>Огляд моніторингу</h2>
        <button onClick={() => void refresh()} disabled={busy}>
          {busy ? "Оновлення…" : "Оновити огляд"}
        </button>
      </div>
      {error && (
        <p role="alert" className="error">
          {error} {data && "Залишено останній успішний знімок."}
        </p>
      )}
      {busy && !data && <p role="status">Завантаження огляду…</p>}
      <div className="metric-grid">
        {metrics.map((m) => (
          <article className="metric-card" key={m.label}>
            <div className="metric-label">
              {m.label}
              <span className="metric-icon" aria-hidden="true">
                {m.icon}
              </span>
            </div>
            <strong>{m.value ?? "—"}</strong>
            <small>{m.hint}</small>
          </article>
        ))}
      </div>
      <div className="overview-columns">
        <section>
          <h2>Стан Monitor</h2>
          {data?.monitorStates ? (
            Object.entries(data.monitorStates).map(([state, count]) => (
              <div className="state-row" key={state}>
                <span className={"badge " + state}>
                  {names[state] ?? state}
                </span>
                <strong>{count}</strong>
              </div>
            ))
          ) : (
            <p className="empty-state">Дані ще не отримано.</p>
          )}
          <small>
            Відсутність свіжих доказів враховується як unknown, а не як
            підтверджена відмова.
          </small>
          <a href="#monitor">Переглянути поточний Monitor →</a>
        </section>
        <section>
          <h2>Джерела та покриття</h2>
          {data?.agentCounts &&
            Object.entries(data.agentCounts).map(([state, count]) => (
              <div className="state-row" key={state}>
                <span>{names[state] ?? state}</span>
                <strong>{count}</strong>
              </div>
            ))}
          {data?.coverageStates && (
            <>
              <h3>Повнота доказів</h3>
              {Object.entries(data.coverageStates).map(([state, count]) => (
                <div className="state-row" key={state}>
                  <span>{names[state] ?? state}</span>
                  <strong>{count}</strong>
                </div>
              ))}
            </>
          )}
          <small>
            Стан Agent та доступність цільового сервісу оцінюються окремо.
          </small>
          {canReadAgents && <a href="#agents">Переглянути агентів →</a>}
        </section>
      </div>
      <section className="overview-recent">
        <div className="row">
          <h2>Останні діагностичні сесії</h2>
          {canReadDiagnostics && <a href="#diagnostics">Усі сесії →</a>}
        </div>
        {data?.recentDiagnostics?.length ? (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Monitor</th>
                  <th>Початок</th>
                  <th>Режим</th>
                  <th>Результат</th>
                </tr>
              </thead>
              <tbody>
                {data.recentDiagnostics.map((s) => (
                  <tr key={s.id}>
                    <td>
                      <strong>{s.monitorId}</strong>
                    </td>
                    <td>{new Date(s.startedAt).toLocaleString("uk-UA")}</td>
                    <td>{s.mode}</td>
                    <td>
                      {s.state} · {s.stopReason}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="empty-state">
            Діагностичних сесій у цьому зрізі немає.
          </p>
        )}
        {data && (
          <small>
            Зріз Controller:{" "}
            <time dateTime={data.observedAt}>
              {new Date(data.observedAt).toLocaleString("uk-UA")}
            </time>
            . Дані розділу оновлюються кнопкою «Оновити огляд».
          </small>
        )}
      </section>
    </div>
  );
}
