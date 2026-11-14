import { useState } from "react";
import { alerts, alertDetail, alertHistory, webhookDeliveries,
  type AlertView, type AlertPage, type AlertHistoryPage, type DeliveryPage } from "./api";

export function Alerts({ monitor }: { monitor: string }) {
  const [page, setPage] = useState<AlertPage | null>(null);
  const [selected, setSelected] = useState<AlertView | null>(null);
  const [history, setHistory] = useState<AlertHistoryPage | null>(null);
  const [deliveries, setDeliveries] = useState<DeliveryPage | null>(null);
  const [state, setState] = useState("");
  const [deliveryState, setDeliveryState] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function act(fn: () => Promise<void>) {
    setBusy(true); setError("");
    try { await fn(); } catch (e) { setError(e instanceof Error ? e.message : "Дані недоступні."); }
    finally { setBusy(false); }
  }
  async function open(id: string, cursor: string | null = null) {
    // Sequential requests share the Controller's bounded read admission.
    const detail = await alertDetail(id);
    const events = await alertHistory(id, cursor);
    setSelected(detail); setHistory(events);
  }
  return <section>
    <h2>Алерти та доставка сповіщень</h2>
    <p>Стан алерта, доступність сервісу й доставка webhook відображаються окремо.</p>
    <form onSubmit={e => { e.preventDefault(); void act(async () => setPage(await alerts(monitor, state, null))); }}>
      <label>Стан алертів для {monitor}
        <select value={state} disabled={busy} onChange={e => { setState(e.target.value); setPage(null); }}>
          <option value="">Усі</option><option value="normal">Нормальний</option><option value="pending">Очікування підтвердження</option><option value="firing">Активний</option><option value="resolved">Вирішений</option>
        </select>
      </label><button disabled={busy}>Оновити алерти</button>
    </form>
    {page && <>
      {page.items.length === 0 ? <p>Алертів за цим фільтром немає.</p> : <div className="table-scroll"><table>
        <caption>Збережені алерти</caption><thead><tr><th>Правило</th><th>Стан</th><th>Важливість</th><th>Деталі</th></tr></thead>
        <tbody>{page.items.map(a => <tr key={a.id}><td>{a.ruleRef}</td><td>{a.state}</td><td>{a.severity}</td><td><button disabled={busy} onClick={() => void act(() => open(a.id))}>Відкрити</button></td></tr>)}</tbody>
      </table></div>}
      <button disabled={busy || !page.nextCursor} onClick={() => void act(async () => setPage(await alerts(monitor, state, page.nextCursor)))}>Наступна сторінка алертів</button>
    </>}
    {selected && <div>
      <h3>{selected.ruleRef} · {selected.state}</h3>
      <p>Початок циклу: {selected.startedAt ?? "історичні дані відсутні"}<br/>Спрацював: {selected.firedAt ?? "немає запису"}<br/>Вирішений: {selected.resolvedAt ?? "немає запису"}</p>
      <p>Причини: {selected.reasonCodes.join(", ") || "немає збережених причин"}</p>
      <details><summary>Докази ({selected.evidenceIds.length})</summary><ul>{selected.evidenceIds.map(id => <li key={id}>{id}</li>)}</ul></details>
      <button disabled={busy} onClick={() => void act(() => open(selected.id))}>Оновити деталі</button>
      {history && <><div className="table-scroll"><table>
        <caption>Історія переходів</caption><thead><tr><th>Час</th><th>Перехід</th><th>Cooldown</th><th>Ревізія</th></tr></thead>
        <tbody>{history.items.map(e => <tr key={e.id}><td>{e.at}</td><td>{e.from} → {e.to}</td><td>{e.suppressed ? "Повідомлення придушено" : "Немає позначки придушення"}</td><td><small>{e.configRevision}</small></td></tr>)}</tbody>
      </table></div><button disabled={busy || !history.nextCursor} onClick={() => void act(() => open(selected.id, history.nextCursor))}>Наступна сторінка історії</button></>}
    </div>}
    <h3>Webhook поточної конфігурації</h3>
    <form onSubmit={e => { e.preventDefault(); void act(async () => setDeliveries(await webhookDeliveries(deliveryState, null))); }}>
      <label>Стан доставки<select value={deliveryState} disabled={busy} onChange={e => { setDeliveryState(e.target.value); setDeliveries(null); }}>
        <option value="">Усі</option><option value="pending">У черзі</option><option value="leased">Спроба виконується</option><option value="delivered">Доставлено</option><option value="failed">Не доставлено</option><option value="cancelled">Скасовано</option>
      </select></label><button disabled={busy}>Оновити доставки</button>
    </form>
    {deliveries && <>
      {deliveries.items.length === 0 ? <p>Подій доставки за цим фільтром немає.</p> : <div className="table-scroll"><table>
        <caption>Події durable outbox</caption><thead><tr><th>Подія</th><th>Стан</th><th>Спроби</th><th>Останній HTTP status</th><th>Початок останньої спроби</th></tr></thead>
        <tbody>{deliveries.items.map(d => <tr key={d.id}><td><small>{d.eventId}</small></td><td>{d.state}</td><td>{d.attempts}</td><td>{d.lastHttpStatus ?? "немає HTTP-відповіді"}</td><td>{d.lastAttemptAt ?? "ще не запускалася"}</td></tr>)}</tbody>
      </table></div>}
      <button disabled={busy || !deliveries.nextCursor} onClick={() => void act(async () => setDeliveries(await webhookDeliveries(deliveryState, deliveries.nextCursor)))}>Наступна сторінка доставок</button>
    </>}
    {error && <p role="alert" className="error">{error}</p>}
    {busy && <p role="status">Завантаження…</p>}
  </section>;
}
