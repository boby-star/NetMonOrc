import { useState } from "react";
import { dashboardSummary, type DashboardSummary } from "./api";
export function Overview() {
  const [data,setData]=useState<DashboardSummary|null>(null);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  async function refresh(){setBusy(true);setError("");try{setData(await dashboardSummary())}catch(e){setError(e instanceof Error?e.message:"Огляд недоступний.")}finally{setBusy(false)}}
  return <section>
    <h2>Огляд моніторингу</h2>
    <button disabled={busy} onClick={() => void refresh()}>Оновити огляд</button>
    {error && <p role="alert" className="error">{error}</p>}
    {data && <>
      <p>Зріз: {data.observedAt}. Неперевірені, застарілі та невибрані Monitor враховані як unknown.</p>
      <div className="row">
        {data.monitorStates && <div><h3>Ресурси</h3><ul>{Object.entries(data.monitorStates).map(([state,n])=><li key={state}>{state}: {n}</li>)}</ul></div>}
        {data.coverageStates && <div><h3>Повнота доказів</h3><ul>{Object.entries(data.coverageStates).map(([state,n])=><li key={state}>{state}: {n}</li>)}</ul></div>}
        {data.agentCounts && <div><h3>Агенти</h3><ul>{Object.entries(data.agentCounts).map(([state,n])=><li key={state}>{state}: {n}</li>)}</ul></div>}
      </div>
      {data.firingAlerts !== undefined && <p>Активних алертів: {data.firingAlerts}</p>}
      {data.recentDiagnostics && <div className="table-scroll"><table><caption>Останні діагностичні сесії</caption><thead><tr><th>Monitor</th><th>Початок</th><th>Режим</th><th>Результат</th></tr></thead><tbody>{data.recentDiagnostics.map(s=><tr key={s.id}><td>{s.monitorId}</td><td>{s.startedAt}</td><td>{s.mode}</td><td>{s.state} · {s.stopReason}</td></tr>)}</tbody></table></div>}
    </>}
    {busy && <p role="status">Завантаження…</p>}
  </section>
}
