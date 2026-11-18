import { useEffect, useRef, useState } from "react";
import { ObservationDetail } from "./History";
import { exportDiagnostic, startDiagnostic, cancelDiagnostic, type Session, diagnosticSessions, diagnosticSession, diagnosticTasks,
  type DiagnosticSession, type DiagnosticTask, type DiagnosticPage } from "./api";

export function Diagnostics({ monitor, session }: { monitor: string; session: Session }) {
  const [profile, setProfile] = useState("");
  const request = useRef<{ profile: string; id: string } | null>(null);
  const canRun = session.role === "admin" && session.scopes.includes("diagnostics:run");
  const canReadEvidence = session.scopes.includes("resources:read");
  const [page, setPage] = useState<DiagnosticPage | null>(null);
  const [selected, setSelected] = useState<DiagnosticSession | null>(null);
  const [evidence, setEvidence] = useState<string | null>(null);
  const [tasks, setTasks] = useState<DiagnosticTask[]>([]);
  const [taskCursor, setTaskCursor] = useState<string | null>(null);
  const [state, setState] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const exportAbort = useRef<AbortController | null>(null);
  useEffect(() => () => exportAbort.current?.abort(), []);
  const [exportNotice, setExportNotice] = useState("");
  const [autoUpdate, setAutoUpdate] = useState(true);
  useEffect(() => {
    if (!autoUpdate || busy || !selected || !["created", "running"].includes(selected.state)) return;
    const timer = window.setTimeout(() => void open(selected.id), 3000);
    return () => window.clearTimeout(timer);
  }, [autoUpdate, busy, selected?.id, selected?.state]);
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
      if (selected?.id !== id) { setEvidence(null); setExportNotice(""); }
    } catch (e) { setError(e instanceof Error ? e.message : "Сесія недоступна."); }
    finally { setBusy(false); }
  }
  async function run() {
    setBusy(true); setError("");
    if (!request.current || request.current.profile !== profile)
      request.current = { profile, id: crypto.randomUUID() };
    try {
      const started = await startDiagnostic(session, monitor, profile, request.current.id);
      setSelected(started); setTasks([]); setTaskCursor(null); setEvidence(null);
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
  function download() {
    if (!selected) return;
    const url = URL.createObjectURL(new Blob([JSON.stringify(selected, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url; link.download = `diagnostic-${selected.id}.json`;
    link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  async function downloadEvidence() {
    if (!selected) return;
    setBusy(true); setError(""); setExportNotice("Збирання завдань і доказів…");
    const controller = new AbortController(); exportAbort.current = controller;
    try {
      const text = await exportDiagnostic(selected.id, controller.signal);
      const data = JSON.parse(text) as { complete: boolean; missingObservationIds: string[]; baselineEvidenceKnown: boolean };
      const url = URL.createObjectURL(new Blob([text], { type: "application/json" }));
      const link = document.createElement("a");
      link.href = url; link.download = `diagnostic-evidence-${selected.id}.json`; link.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setExportNotice((data.complete ? "Усі збережені посилання на докази завантажено." : `Пакет завантажено; відсутніх доказів: ${data.missingObservationIds.length}. Див. missingObservationIds.`) + (data.baselineEvidenceKnown ? " Початковий набір доказів збережено." : " Склад початкових доказів історичної сесії невідомий."));
    } catch (e) { setExportNotice(""); setError(e instanceof Error ? e.message : "Експорт не завершено."); }
    finally { exportAbort.current = null; setBusy(false); }
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
      <label><input type="checkbox" checked={autoUpdate} onChange={e => setAutoUpdate(e.target.checked)} /> Автоматично оновлювати активну сесію</label>
      {canRun && selected.state === "running" && <button type="button" disabled={busy} onClick={() => void cancel()}>Скасувати сесію</button>}
      <p>{selected.profileRef ?? "Історичний профіль не збережено"} · {selected.mode} · {selected.state}</p>
      <p>Ревізія сесії: <code>{selected.configRevision || "не збережено"}</code>. Це історичний висновок, а не поточна оцінка ресурсу.</p>
      <p>Початок: {selected.startedAt}<br/>Завершення: {selected.finishedAt ?? "ще не завершено"}<br/>Причина: {selected.stopReason ?? "ще не визначена"}</p>
      <p>Сумісні гіпотези: {selected.remainingHypotheses === undefined ? "остаточний звіт відсутній" : selected.remainingHypotheses.join(", ") || "немає"}</p>
      <p>Спостереження фізичних діагностичних задач: {selected.evidenceIds.length}. Резерв бюджету не гарантує отримання результату.</p>
      <button type="button" disabled={busy} onClick={() => void open(selected.id)}>Оновити деталі</button>
      <button type="button" onClick={download}>Завантажити сесію JSON</button>
      {canReadEvidence && <button type="button" disabled={busy || !["completed", "inconclusive", "cancelled"].includes(selected.state)} onClick={() => void downloadEvidence()}>Завантажити пакет доказів</button>}
      {exportNotice && <p role="status">{exportNotice}</p>}
      {selected.explanation ? <div aria-label="Пояснення діагнозу">
        <h4>Початкові регулярні докази</h4>
        {selected.explanation.baselineEvidence === undefined ? <p>Початкові докази не збережені в історичному звіті; їхній склад невідомий.</p> : <>
          <p>Збережено вхідні спостереження сесії. Вони могли втратити придатність під час діагностики; це не перелік остаточних підтверджень і не додаткові витрати бюджету.</p>
          {selected.explanation.baselineEvidence.length === 0 ? <p>Початкових придатних регулярних доказів не було.</p> : <div className="table-scroll"><table>
            <caption>Походження початкових доказів</caption>
            <thead><tr><th>Дія / перевірка</th><th>Agent / зона / домен</th><th>Результат</th><th>Час / TTL до</th><th>Observation</th></tr></thead>
            <tbody>{selected.explanation.baselineEvidence.map((b, i) => <tr key={i}>
              <td>{b.actionId}<br/>{b.checkId}</td><td>{b.agentId}<br/>{b.zone}<br/>{b.failureDomain || "невідомий"}</td>
              <td>{b.executionStatus} / {b.probeOutcome}</td><td>{b.finishedAt}<br/>{b.validUntil}</td>
              <td>{canReadEvidence && <button type="button" onClick={() => setEvidence(b.observationId)}>Відкрити доказ</button>}<br/><code>{b.observationId}</code><br/><small>Ревізія: {b.configRevision}</small></td>
            </tr>)}</tbody>
          </table></div>}
        </>}
        <h4>Журнал вибору діагностичних дій</h4>
        {selected.explanation.evidencePolicy && <p>Політика доказів сесії: TTL додаткових probes {selected.explanation.evidencePolicy.maxAgeMs} мс; maxSkew {selected.explanation.evidencePolicy.maxSkewMs} мс. Прострочені та неузгоджені докази не визначають поточний діагноз; витрачений бюджет зберігається.</p>}
        <p>Витрачено {selected.explanation.costUnits} costUnits; час діагностики: {selected.explanation.durationMs === undefined ? "не збережено" : `${selected.explanation.durationMs} мс`}. costUnits не є мережевими байтами.</p>
        <p>Gain — кількість розрізнюваних пар гіпотез; score — gain / costUnits. Для Fixed/Static порядок визначається профілем. Технічна помилка не доводить відмову сервісу.</p>
        <div className="table-scroll"><table>
          <caption>Послідовність фактичних рішень Controller</caption>
          <thead><tr><th>Крок / фаза</th><th>Дія / Agent / спроба</th><th>Гіпотези до кроку</th><th>Gain / score</th><th>Виконання / результат</th><th>Час доказу / TTL</th></tr></thead>
          <tbody>{selected.explanation.decisions.map((d, i) => <tr key={i}>
            <td>{i + 1} / {d.phase || "не збережено"}</td>
            <td>{d.actionId}<br/>{d.agentId}<br/>Домен: {d.failureDomain || "невідомий"}<br/>{d.attemptNo ?? "не збережено"}</td>
            <td>{d.hypothesesBefore ? d.hypothesesBefore.join(", ") || "немає" : "не збережено"}</td>
            <td>{d.gain} / {d.score}</td><td>{d.executionStatus} / {d.probeOutcome}</td>
            <td>{d.finishedAt || "не збережено"}<br/>TTL/deadline до: {d.validUntil || "не збережено"}<br/>{d.observationId && <code>{d.observationId}</code>}</td>
          </tr>)}</tbody>
        </table></div>
        {selected.explanation.decisions.length === 0 && <p>Додаткових дій у збереженому журналі немає.</p>}
        {selected.explanation.confirmation ? <div>
          <h4>Підтвердження висновку</h4>
          <p>{selected.explanation.confirmation.satisfied ? "Умови виконано" : "Умови не виконано"}. Зарахованих спостережень: {selected.explanation.confirmation.observations}; доменів відмови: {selected.explanation.confirmation.distinctFailureDomains}.</p>
          <p>Обов’язкові дії: {selected.explanation.confirmation.requiredActions.join(", ")}. Підтверджені дії: {selected.explanation.confirmation.observedActions.join(", ") || "немає"}.</p>
          {selected.explanation.confirmation.evidenceDomains && <ul>{Object.entries(selected.explanation.confirmation.evidenceDomains).map(([proof, domain]) => <li key={proof}><code>{proof}</code> → {domain || "домен невідомий"}</li>)}</ul>}
        </div> : <p>Окреме підтвердження не записане у звіті.</p>}
        {!!selected.explanation.blockedSteps?.length && <p>Заблоковані кроки: {selected.explanation.blockedSteps.join(", ")}</p>}
        {!!selected.explanation.skippedSteps?.length && <p>Пропущені кроки: {selected.explanation.skippedSteps.join(", ")}</p>}
      </div> : <p>Журнал рішень ще не збережено або відсутній в історичному звіті.</p>}
      <div className="table-scroll"><table>
        <caption>Фізичні задачі Agent</caption>
        <thead><tr><th>Дія</th><th>Agent</th><th>Спроба</th><th>Стан</th><th>Докази</th></tr></thead>
        <tbody>{tasks.map(t => <tr key={t.id}><td>{t.actionId}<br/><small>{t.id}</small></td><td>{t.agentId}</td><td>{t.attemptNo}</td><td>{t.state}</td><td>{t.observationIds.length ? t.observationIds.map(id => <p key={id}>{canReadEvidence && <><button type="button" onClick={() => setEvidence(id)}>Відкрити доказ</button><br/></>}<code>{id}</code></p>) : "не отримано"}</td></tr>)}</tbody>
      </table></div>
      <button type="button" disabled={busy || !taskCursor} onClick={() => void open(selected.id, taskCursor)}>Наступна сторінка задач</button>
    </div>}
    {evidence && <ObservationDetail key={evidence} id={evidence} close={() => setEvidence(null)} />}
    {busy && <p role="status">Завантаження…</p>}
  </section>;
}
