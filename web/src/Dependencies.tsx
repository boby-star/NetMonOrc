import { useEffect, useRef, useState } from "react";
import { dependencyPage, referenceKey, type DependencyLink } from "./dependency-view";
import { DependencyGraph } from "./DependencyGraph";

export function Dependencies() {
  const [links, setLinks] = useState<DependencyLink[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [receivedAt, setReceivedAt] = useState("");
  const [query, setQuery] = useState("");
  const [node, setNode] = useState("");
  const [selected, setSelected] = useState("");
  const request = useRef<AbortController | null>(null);
  async function load(next: string | null = null) {
    request.current?.abort();
    const controller = new AbortController(); request.current = controller;
    setBusy(true); setError(""); setLinks([]); setCursor(null); setReceivedAt(""); setNode(""); setSelected(""); setQuery("");
    try {
      const page = await dependencyPage(next, controller.signal);
      if (request.current !== controller || controller.signal.aborted) return;
      setLinks(page.items); setCursor(page.nextCursor); setReceivedAt(new Date().toLocaleString("uk-UA"));
    } catch(e) {
      if (!controller.signal.aborted && request.current === controller) setError(e instanceof Error ? e.message : "Граф тимчасово недоступний. Оновіть сторінку графа.");
    } finally { if (request.current === controller) setBusy(false); }
  }
  useEffect(() => { void load(); return () => { request.current?.abort(); }; }, []);
  const nodes = [...new Set(links.flatMap(link => [referenceKey(link.from), referenceKey(link.to)]))].sort();
  const visible = links.filter(link => (!node || referenceKey(link.from) === node || referenceKey(link.to) === node)
    && [link.name, referenceKey(link.from), referenceKey(link.to), link.description, link.relation, link.strength].join(" ").toLocaleLowerCase("uk-UA").includes(query.toLocaleLowerCase("uk-UA")));
  const detail = visible.find(link => link.name === selected);
  function selectNode(key: string) { setNode(key); setSelected(""); }
  return <section aria-labelledby="dependencies-heading">
    <h2 id="dependencies-heading">Залежності ресурсів</h2>
    <p>Стрілка <strong>from → to</strong> означає «from залежить від to». Колір позначає тип вузла, а не здоров’я сервісу. Зв’язок не доводить причину несправності.</p>
    <div className="row">
      <button type="button" disabled={busy} onClick={() => void load()}>Оновити граф</button>
      <button type="button" disabled={busy || !cursor} onClick={() => void load(cursor)}>Наступна сторінка зв’язків</button>
    </div>
    {error && <p role="alert" className="error">{error}</p>}
    {busy && <p role="status">Завантаження зв’язків…</p>}
    {receivedAt && <>
      <small>Отримано: {receivedAt}. Оновлення за запитом. На цій сторінці: {links.length} зв’язків; показано після фільтрів: {visible.length}.{cursor ? " Є наступна сторінка; це не повний граф каталогу." : ""}</small>
      <div className="row dependency-filters">
        <label>Пошук на поточній сторінці<input value={query} onChange={e => { setQuery(e.target.value); setSelected(""); }} /></label>
        <label>Зв’язки вузла<select value={node} onChange={e => selectNode(e.target.value)}><option value="">Усі вузли сторінки</option>{nodes.map(key => <option key={key} value={key}>{key}</option>)}</select></label>
        <button type="button" onClick={() => { setQuery(""); setNode(""); setSelected(""); }}>Скинути фільтри</button>
      </div>
      {visible.length > 0 ? <>
        <small>Суцільна лінія — requires, пунктир — інший relation. Виберіть вузол або зв’язок мишею чи клавішами Tab та Enter.</small>
        <DependencyGraph links={visible} selectedNode={node} selectedLink={selected} onNode={selectNode} onLink={setSelected} />
        <div className="table-scroll"><table><caption>Зв’язки поточної сторінки</caption><thead><tr><th>Назва</th><th>From → To</th><th>Relation / strength</th></tr></thead>
          <tbody>{visible.map(link => <tr key={link.name}><td><button type="button" aria-pressed={selected === link.name} onClick={() => setSelected(link.name)}>{link.name}</button></td><td><code>{referenceKey(link.from)}</code> → <code>{referenceKey(link.to)}</code></td><td>{link.relation} / {link.strength}</td></tr>)}</tbody>
        </table></div>
      </> : <p>{links.length ? "За цими фільтрами зв’язків немає." : "У каталозі немає зв’язків на цій сторінці."}</p>}
      {detail && <div className="dependency-detail" role="region" aria-label="Деталі зв’язку"><h3>{detail.name}</h3><p><code>{referenceKey(detail.from)}</code> → <code>{referenceKey(detail.to)}</code></p><p>Relation: {detail.relation}; strength: {detail.strength}.</p><p>{detail.description || "Опис не задано."}</p></div>}
      <small>Редагування — у конфігураційному редакторі через ресурс Dependency. Фільтри й вибір вузла стосуються лише завантаженої сторінки.</small>
    </>}
  </section>;
}
