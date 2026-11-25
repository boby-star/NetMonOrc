import { useState } from "react";
import { resourceCatalog, groupAssessment, type GroupAssessment } from "./api";
export function ResourceGroups() {
  const [groups, setGroups] = useState<string[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [view, setView] = useState<GroupAssessment | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function list(next: string | null = null) {
    setBusy(true); setError("");
    try { const page = await resourceCatalog("ResourceGroup", next); setGroups(page.items.map(v => v.manifest.metadata.name)); setCursor(page.nextCursor); }
    catch(e) { setError(e instanceof Error ? e.message : "Групи недоступні"); }
    finally { setBusy(false); }
  }
  async function open(name: string) {
    setBusy(true); setError(""); setView(null);
    try { setView(await groupAssessment(name)); }
    catch(e) { setError(e instanceof Error ? e.message : "Оцінка групи недоступна"); }
    finally { setBusy(false); }
  }
  return <section aria-busy={busy}><h2>Групи ресурсів</h2>
    <button disabled={busy} onClick={() => void list()}>Оновити групи</button>
    {error && <p role="alert" className="error">{error}</p>}
    <ul>{groups.map(name => <li key={name}><button disabled={busy} onClick={() => void open(name)}>{name}</button></li>)}</ul>
    {cursor && <button disabled={busy} onClick={() => void list(cursor)}>Наступна сторінка</button>}
    {view && <div><h3>{view.groupName}</h3><p>{view.mode === "context" ? "Контекстна група — власний стан не обчислюється" : `Стан: ${view.state}; повнота даних: ${view.coverageState}`}</p>
      <table><thead><tr><th>Monitor</th><th>Стан</th><th>Повнота даних</th></tr></thead><tbody>{view.members.map(m => <tr key={m.monitorId}><td>{m.monitorId}</td><td>{m.state}</td><td>{m.coverageState}</td></tr>)}</tbody></table>
      {view.members.length === 0 && <p>Група не має учасників.</p>}
      <button disabled={busy} onClick={() => void open(view.groupName)}>Оновити оцінку</button>
      <small>Відсутні актуальні результати показані як unknown. Група не запускає власних перевірок.</small>
    </div>}
  </section>;
}
