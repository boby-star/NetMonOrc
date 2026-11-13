import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { loadAssessment, type Assessment } from "./api";
import "./style.css";
const labels = {
  healthy: "Працює",
  degraded: "Погіршення",
  down: "Відмова",
  unknown: "Недостатньо доказів",
};
const percent = (x: number | null) =>
  typeof x === "number" ? `${(x * 100).toFixed(0)}%` : "—";
function App() {
  const [monitor, setMonitor] = useState("research-web");
  const [token, setToken] = useState("");
  const [connection, setConnection] = useState<{
    monitor: string;
    token: string;
  } | null>(null);
  const [assessment, setAssessment] = useState<Assessment | null>(null);
  const [error, setError] = useState("");
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    if (!connection) return;
    let closed = false;
    let busy = false;
    const controller = new AbortController();
    const update = async () => {
      if (busy) return;
      busy = true;
      try {
        const data = await loadAssessment(
          connection.monitor,
          connection.token,
          controller.signal,
        );
        if (!closed) {
          setAssessment(data);
          setError("");
        }
      } catch (e) {
        if (!closed) {
          setAssessment(null);
          setError(
            e instanceof Error ? e.message : "Не вдалося отримати оцінку",
          );
        }
      } finally {
        busy = false;
      }
    };
    void update();
    const timer = setInterval(() => void update(), 5000);
    return () => {
      closed = true;
      controller.abort();
      clearInterval(timer);
    };
  }, [connection]);
  const age = assessment
    ? (now - Date.parse(assessment.evaluatedAt)) / 1000
    : Infinity;
  const stale = !Number.isFinite(age) || age > 10 || age < -5;
  return (
    <>
      <header>
        <h1>NetMonOrc</h1>
        <p>Комплексний моніторинг корпоративних мереж</p>
      </header>
      <main>
        <section>
          <h2>Підключення до Controller</h2>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setAssessment(null);
              setError("");
              setConnection({ monitor, token });
              setToken("");
            }}
          >
            <label>
              Monitor
              <input
                value={monitor}
                required
                pattern="[a-z][a-z0-9-]*"
                onChange={(e) => setMonitor(e.target.value)}
                disabled={!!connection}
              />
            </label>
            {!connection && (
              <label>
                Токен читання
                <input
                  type="password"
                  value={token}
                  required
                  minLength={32}
                  autoComplete="off"
                  onChange={(e) => setToken(e.target.value)}
                />
              </label>
            )}
            {!connection ? (
              <button>Підключитися</button>
            ) : (
              <button
                type="button"
                onClick={() => {
                  setConnection(null);
                  setAssessment(null);
                  setError("");
                  setToken("");
                }}
              >
                Від’єднатися
              </button>
            )}
          </form>
          <small>
            Токен зберігається лише в пам’яті вкладки. Цей етап підтримує
            перегляд оцінки одного Monitor.
          </small>
        </section>
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        {connection && !assessment && !error && (
          <p role="status">Отримання оцінки…</p>
        )}
        {assessment && (
          <>
            <section>
              <div className="row">
                <h2>{assessment.monitorId}</h2>
                <strong
                  className={"badge " + (stale ? "unknown" : assessment.state)}
                >
                  {stale ? "Застарілий знімок" : labels[assessment.state]}
                </strong>
              </div>
              <p>
                Покриття:{" "}
                {
                  {
                    sufficient: "Достатнє",
                    degraded: "Погіршене",
                    unknown: "Невідоме",
                  }[assessment.coverageState]
                }
              </p>
              <p>
                Оцінено: <time>{assessment.evaluatedAt}</time>
              </p>
              {stale && (
                <p role="alert">
                  Перевірте доступність Controller і час пристрою.
                </p>
              )}
              <details>
                <summary>Ефективна ревізія</summary>
                <code>{assessment.revision}</code>
              </details>
            </section>
            <section>
              <h2>Мережеві зони</h2>
              <div className="scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Зона</th>
                      <th>Стан</th>
                      <th>H</th>
                      <th>Q</th>
                      <th>Покриття</th>
                      <th>Причина</th>
                    </tr>
                  </thead>
                  <tbody>
                    {assessment.zoneStates.map((z) => (
                      <tr key={z.zone}>
                        <td>{z.zone}</td>
                        <td>{labels[z.state]}</td>
                        <td>{percent(z.h)}</td>
                        <td>{percent(z.q)}</td>
                        <td>
                          {z.coverage === "complete" ? "Повне" : "Неповне"}
                        </td>
                        <td>
                          {z.reason}
                          <details>
                            <summary>{z.evidence.length} доказів</summary>
                            {z.evidence.map((id) => (
                              <p key={id}>
                                <code>{id}</code>
                              </p>
                            ))}
                          </details>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <small>
                H і Q обчислені Controller. Втрата покриття сама по собі не
                означає відмову сервісу.
              </small>
            </section>
          </>
        )}
      </main>
    </>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
