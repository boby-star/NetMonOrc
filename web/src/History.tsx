import { useEffect, useState } from "react";
import { loadHistory, type ObservationPage } from "./api";

// Raw observations remain history; this component never calculates resource state.
export function History({ monitor }: { monitor: string }) {
  const [zone, setZone] = useState("");
  const [filter, setFilter] = useState("");
  const [cursor, setCursor] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [page, setPage] = useState<ObservationPage | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setBusy(true);
    setError("");
    setPage(null);
    void loadHistory(monitor, filter, cursor, controller.signal)
      .then((result) => {
        if (active) setPage(result);
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : "Історія недоступна.");
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [monitor, filter, cursor, refresh]);
  return (
    <section>
      <h2>Історія перевірок</h2>
      <p>
        Збережені спостереження, включно з попередніми ревізіями. Поточний стан
        визначає Controller.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setCursor(null);
          setFilter(zone.trim());
          setRefresh((x) => x + 1);
        }}
      >
        <label>
          Мережева зона
          <input
            value={zone}
            maxLength={256}
            placeholder="Усі зони"
            onChange={(e) => setZone(e.target.value)}
          />
        </label>
        <button disabled={busy}>Оновити історію</button>
      </form>
      {busy && <p role="status">Завантаження…</p>}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {page && (
        <>
          {page.items.length === 0 ? (
            <p>Спостережень за цим фільтром немає.</p>
          ) : (
            <div className="table-scroll">
              <table>
                <caption>
                  До 50 записів на сторінку, від новіших до старіших
                </caption>
                <thead>
                  <tr>
                    <th>Час завершення / приймання</th>
                    <th>Перевірка / зона / агент</th>
                    <th>Виконання</th>
                    <th>Результат probe</th>
                    <th>Доказ і ревізія</th>
                  </tr>
                </thead>
                <tbody>
                  {page.items.map((o) => (
                    <tr key={o.observationId}>
                      <td>
                        <time dateTime={o.finishedAt}>
                          {new Date(o.finishedAt).toLocaleString("uk-UA")}
                        </time>
                        <br />
                        <small>
                          {new Date(o.receivedAt).toLocaleString("uk-UA")}
                        </small>
                      </td>
                      <td>
                        {o.checkId}
                        <br />
                        {o.networkZone}
                        <br />
                        <small>{o.agentId}</small>
                      </td>
                      <td>
                        {o.executionStatus}
                        <br />
                        <small>{o.reasonCode || "—"}</small>
                      </td>
                      <td>
                        {o.probeOutcome}
                        <br />
                        <small>{o.durationMs.toFixed(1)} мс</small>
                        {o.diagnosticTaskId && (
                          <p>Діагностика: {o.diagnosticTaskId}</p>
                        )}
                      </td>
                      <td>
                        <code>{o.observationId}</code>
                        <br />
                        <small title={o.configRevision}>
                          {o.configRevision}
                        </small>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="row">
            <button
              type="button"
              disabled={busy || !cursor}
              onClick={() => setCursor(null)}
            >
              До новіших
            </button>
            <button
              type="button"
              disabled={busy || !page.nextCursor}
              onClick={() => setCursor(page.nextCursor)}
            >
              Старіші записи
            </button>
          </div>
        </>
      )}
      <small>
        Execution failure і негативний probe показані окремо. Історія не
        доводить актуальність даних для поточної оцінки.
      </small>
    </section>
  );
}
