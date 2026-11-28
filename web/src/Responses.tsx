import { useEffect, useState } from "react";
import { loadResponses, type ResponseEvent } from "./api";

const labels: Record<string, string> = {
  queued: "Команду збережено", applied: "Правило встановлено",
  expired: "Строк правила минув", unacknowledged: "Підтвердження не отримано",
  failed: "Помилка виконання", rejected: "Відхилено локальною політикою",
};

export function Responses({ monitor }: { monitor: string }) {
  const [items, setItems] = useState<ResponseEvent[]>([]);
  const [refresh, setRefresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setBusy(true); setError(""); setItems([]);
    void loadResponses(monitor, controller.signal).then((rows) => { if (active) setItems(rows); })
      .catch((e) => { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : "Історія недоступна"); })
      .finally(() => { if (!controller.signal.aborted) setBusy(false); });
    return () => { active = false; controller.abort(); };
  }, [monitor, refresh]);
  return <section aria-busy={busy}>
    <h2>Захисні реакції</h2>
    <p>Останні 50 команд для {monitor}. Одна реакція на епізод відхилення; повторний дозвіл — після healthy з повним покриттям.</p>
    <p>Строк правила визначає автоматичне припинення блокування. Це не підтвердження усунення причини події чи мережевої доступності.</p>
    <button disabled={busy} onClick={() => setRefresh((v) => v + 1)}>Оновити реакції</button>
    {busy && <p role="status">Завантаження реакцій…</p>}
    {error && <p role="alert">{error}</p>}
    {!busy && !error && items.length === 0 && <p>Команд реагування для цього монітора немає.</p>}
    {items.map((item) => <article key={item.id}>
      <h3>{item.ruleId} · {labels[item.status] ?? item.status}</h3>
      <p>Agent: {item.agent} · TCP {item.rule.port} · джерело: {item.rule.sourceCIDR}</p>
      <p>Створено: {new Date(item.createdAt).toLocaleString()} · блокує до: {new Date(item.blockUntil).toLocaleString()}</p>
      <p>Причина: {item.reason || "Очікується відповідь агента"} · стан монітора: {item.assessment.state}</p>
      <details><summary>Докази та ревізія</summary><pre>{JSON.stringify(item, null, 2)}</pre></details>
    </article>)}
  </section>;
}
