import { useState } from "react";
import { agents, type AgentPage } from "./api";
const labels = {
  online: "Підключений",
  offline: "Немає підключення",
  degraded: "Очікує свіжого підтвердження",
  revoked: "Відкликаний",
};
export function Agents() {
  const [page, setPage] = useState<AgentPage | null>(null);
  const [zone, setZone] = useState("");
  const [state, setState] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [at, setAt] = useState("");
  async function update(cursor: string | null) {
    setBusy(true);
    setError("");
    try {
      setPage(await agents(zone, state, cursor));
      setAt(new Date().toLocaleString("uk-UA"));
    } catch (e) {
      setPage(null);
      setError(e instanceof Error ? e.message : "Стан агентів недоступний.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h2>Агенти</h2>
      <p>
        Знімок фіксованого roster та чинних mTLS-сесій. Стан агента й
        доступність контрольованого сервісу оцінюються окремо.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void update(null);
        }}
      >
        <label>
          Зона
          <input
            value={zone}
            placeholder="Усі зони"
            disabled={busy}
            onChange={(e) => {
              setZone(e.target.value);
              setPage(null);
            }}
          />
        </label>
        <label>
          Підключення
          <select
            value={state}
            disabled={busy}
            onChange={(e) => {
              setState(e.target.value);
              setPage(null);
            }}
          >
            <option value="">Усі стани</option>
            {Object.entries(labels).map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <button disabled={busy}>Оновити агентів</button>
      </form>
      {at && page && <p>Стан на час оновлення: {at}</p>}
      {busy && <p role="status">Оновлення…</p>}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {page && (
        <>
          <div className="table-scroll">
            <table>
              <caption>Актуальні дані сесій на час знімка</caption>
              <thead>
                <tr>
                  <th>Агент / зона</th>
                  <th>Стан / heartbeat</th>
                  <th>Активні probes / ліміт</th>
                  <th>Ревізія / сертифікат</th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((a) => (
                  <tr key={a.agentId}>
                    <td>
                      {a.name}
                      <br />
                      <small>
                        {a.agentId} · {a.networkZone}
                      </small>
                      <small>
                        {a.capabilities?.join(", ") ||
                          "Capabilities відсутні у поточній сесії"}
                      </small>
                    </td>
                    <td>
                      {labels[a.onlineState]}
                      <br />
                      <small>
                        {a.lastHeartbeat
                          ? new Date(a.lastHeartbeat).toLocaleString("uk-UA")
                          : "Немає поточного heartbeat"}
                      </small>
                    </td>
                    <td>
                      {a.activeProbes ?? "—"} / {a.maxConcurrentProbes ?? "—"}
                    </td>
                    <td>
                      <small>
                        {a.appliedRevision || "Немає підтвердженого плану"}
                      </small>
                      <small>
                        Сертифікат до:{" "}
                        {a.certExpiresAt
                          ? new Date(a.certExpiresAt).toLocaleString("uk-UA")
                          : "—"}
                      </small>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {page.items.length === 0 && <p>Агентів за цим фільтром немає.</p>}
          <button
            type="button"
            disabled={busy || !page.nextCursor}
            onClick={() => void update(page.nextCursor)}
          >
            Наступна сторінка
          </button>
        </>
      )}
      <small>
        Оновлення виконується за запитом. Відсутні дані показані як невідомі.
        Capabilities є оголошенням агента, а не підтвердженням справності
        runner.
      </small>
    </section>
  );
}
