import { useRef, useState } from "react";
import { startDiagnostic, cancelDiagnostic, type Session, diagnosticSessions, diagnosticSession, diagnosticTasks,
  type DiagnosticSession, type DiagnosticTask, type DiagnosticPage } from "./api";

export function Diagnostics({ monitor, session }: { monitor: string; session: Session }) {
  const [profile, setProfile] = useState("");
  const request = useRef<{ profile: string; id: string } | null>(null);
  const canRun = session.role === "admin" && session.scopes.includes("diagnostics:run");
  const [page, setPage] = useState<DiagnosticPage | null>(null);
  const [selected, setSelected] = useState<DiagnosticSession | null>(null);
  const [tasks, setTasks] = useState<DiagnosticTask[]>([]);
  const [taskCursor, setTaskCursor] = useState<string | null>(null);
  const [state, setState] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function list(cursor: string | null) {
    setBusy(true); setError("");
    try { setPage(await diagnosticSessions(monitor, state, cursor)); }
    catch (e) { setError(e instanceof Error ? e.message : "Історія недоступна."); }
    finally { setBusy(false); }
  }
  async function open(id: string, cursor: string | null = null) {
    setBusy(true); setError("");
    try {
      // Sequential reads respect the Controller's bounded read admission.
      const session = await diagnosticSession(id);
      const next = await diagnosticTasks(id, cursor);
      setSelected(session); setTasks(next.items); setTaskCursor(next.nextCursor);
    } catch (e) { setError(e instanceof Error ? e.message : "Сесія недоступна."); }
    finally { setBusy(false); }
  }
  async function run() {
    setBusy(true); setError("");
    if (!request.current || request.current.profile !== profile)
      request.current = { profile, id: crypto.randomUUID() };
    try {
      const started = await startDiagnostic(session, monitor, profile, request.current.id);
      setSelected(started); setTasks([]); setTaskCursor(null);
    } catch (e) { setError(e instanceof Error ? e.message : "Запуск не підтверджено."); }
    finally { setBusy(false); }
  }
  async function cancel() {
    if (!selected) return;
    setBusy(true); setError("");
    try { setSelected(await cancelDiagnostic(session, selected.id)); }
    catch (e) { setError(e instanceof Error ? e.message : "Скасування не підтверджено."); }
    finally { setBusy(false); }
  }
  return <section>
    <h2>Діагностичні сесії</h2>
    <p>Збережені результати для {monitor}. Стан діагностики не замінює оцінку доступності ресурсу.</p>
    {canRun && <form onSubmit={e => { e.preventDefault(); void run(); }}>
      <label>Профіль для ручного запуску
        <input value={profile} required disabled={busy} onChange={e => { setProfile(e.target.value); request.current = null; }} />
      </label>
      <button disabled={busy || !profile}>Запустити / відновити запит</button>
      <button type="button" disabled={busy} onClick={() => { request.current = null; setError(""); }}>Новий запит</button>
      <small>Повтор зберігає ID запиту. Новий запуск також підпорядкований enabled, incident budget і cooldown.</small>
    </form>}
    <form onSubmit={e => { e.preventDefault(); void list(null); }}>
      <label>Стан сесії
        <select value={state} disabled={busy} onChange={e => { setState(e.target.value); setPage(null); }}>
          <option value="">Усі</option>
          <option value="running">Виконується</option>
          <option value="completed">Завершено з висновком</option>
          <option value="inconclusive">Недостатньо доказів</option>
          <option value="cancelled">Скасовано</option>
        </select>
      </label>
      <button disabled={busy}>Оновити список</button>
    </form>
    {error && <p role="alert" className="error">{error}</p>}
    {page && <>
      {page.items.length === 0 ? <p>Сесій за цим фільтром немає.</p> : <div className="table-scroll"><table>
        <caption>Історія діагностики</caption>
        <thead><tr><th>Час</th><th>Режим</th><th>Стан / причина</th><th>Бюджет</th><th>Деталі</th></tr></thead>
        <tbody>{page.items.map(s => <tr key={s.id}>
          <td>{s.startedAt}</td><td>{s.mode}</td><td>{s.state}<br/>{s.stopReason}</td>
          <td>{s.budgetUsed.additionalChecks} фізичних запусків зарезервовано</td>
          <td><button type="button" disabled={busy} onClick={() => void open(s.id)}>Відкрити</button></td>
        </tr>)}</tbody>
      </table></div>}
      <button type="button" disabled={busy || !page.nextCursor} onClick={() => void list(page.nextCursor)}>Наступна сторінка сесій</button>
    </>}
    {selected && <div>
      <h3>Сесія {selected.id}</h3>
      {canRun && selected.state === "running" && <button type="button" disabled={busy} onClick={() => void cancel()}>Скасувати сесію</button>}
      <p>{selected.profileRef ?? "Історичний профіль не збережено"} · {selected.mode} · {selected.state}</p>
      <p>Початок: {selected.startedAt}<br/>Завершення: {selected.finishedAt ?? "ще не завершено"}<br/>Причина: {selected.stopReason ?? "ще не визначена"}</p>
      <p>Сумісні гіпотези: {selected.remainingHypotheses === undefined ? "остаточний звіт відсутній" : selected.remainingHypotheses.join(", ") || "немає"}</p>
      <p>Спостереження фізичних діагностичних задач: {selected.evidenceIds.length}. Резерв бюджету не гарантує отримання результату.</p>
      <button type="button" disabled={busy} onClick={() => void open(selected.id)}>Оновити деталі</button>
      <div className="table-scroll"><table>
        <caption>Фізичні задачі Agent</caption>
        <thead><tr><th>Дія</th><th>Agent</th><th>Спроба</th><th>Стан</th><th>Докази</th></tr></thead>
        <tbody>{tasks.map(t => <tr key={t.id}><td>{t.actionId}<br/><small>{t.id}</small></td><td>{t.agentId}</td><td>{t.attemptNo}</td><td>{t.state}</td><td>{t.observationIds.join(", ") || "не отримано"}</td></tr>)}</tbody>
      </table></div>
      <button type="button" disabled={busy || !taskCursor} onClick={() => void open(selected.id, taskCursor)}>Наступна сторінка задач</button>
    </div>}
    {busy && <p role="status">Завантаження…</p>}
  </section>;
}
