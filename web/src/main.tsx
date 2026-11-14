import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  loadAssessment,
  login,
  changePassword,
  logout,
  sessionInfo,
  AccessError,
  type Session,
  type Assessment,
} from "./api";
import "./style.css";
import { History } from "./History";
import { Tokens } from "./Tokens";
import { Users } from "./Users";
import { Diagnostics } from "./Diagnostics";
import { Agents } from "./Agents";
import { Configuration } from "./Configuration";
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
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [passwordNotice, setPasswordNotice] = useState("");
  const [mode, setMode] = useState("password");
  const [session, setSession] = useState<Session | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [restoring, setRestoring] = useState(true);
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
          if (e instanceof AccessError) {
            setConnection(null);
            setSession(null);
          }
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
  useEffect(() => {
    let active = true;
    void sessionInfo()
      .then((current) => {
        if (active && current) {
          setSession(current);
          setConnection({ monitor: "research-web", token: "" });
        }
      })
      .catch(() => {})
      .finally(() => {
        if (active) setRestoring(false);
      });
    return () => {
      active = false;
    };
  }, []);
  async function connect() {
    setSubmitting(true);
    setError("");
    setAssessment(null);
    try {
      if (mode === "password") {
        const current = await login(username, password);
        setSession(current);
        setConnection({ monitor, token: "" });
      } else {
        setSession(null);
        setConnection({ monitor, token });
      }
      setToken("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Помилка входу");
    } finally {
      setPassword("");
      setSubmitting(false);
    }
  }
  async function disconnect() {
    setSubmitting(true);
    try {
      if (session) await logout(session);
      setCurrentPassword("");
      setNewPassword("");
      setPasswordNotice("");
      setSession(null);
      setConnection(null);
      setAssessment(null);
      setToken("");
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Помилка виходу");
    } finally {
      setSubmitting(false);
    }
  }
  async function updatePassword() {
    if (!session) return;
    setSubmitting(true);
    setPasswordNotice("");
    try {
      setSession(await changePassword(session, currentPassword, newPassword));
      setPasswordNotice("Пароль змінено. Попередні сесії відкликано.");
    } catch (e) {
      setPasswordNotice(
        e instanceof Error ? e.message : "Не вдалося змінити пароль.",
      );
    } finally {
      setCurrentPassword("");
      setNewPassword("");
      setSubmitting(false);
    }
  }
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
          <h2>Вхід до Controller</h2>
          {session && (
            <p>
              {session.username} · {session.role} · сесія до {session.expiresAt}
            </p>
          )}
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void connect();
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
                Спосіб входу
                <select
                  value={mode}
                  disabled={submitting || restoring}
                  onChange={(e) => setMode(e.target.value)}
                >
                  <option value="password">Обліковий запис</option>
                  <option value="token">Bootstrap токен читання</option>
                </select>
              </label>
            )}
            {!connection && mode === "password" && (
              <>
                <label>
                  Користувач
                  <input
                    value={username}
                    required
                    maxLength={64}
                    autoComplete="username"
                    onChange={(e) => setUsername(e.target.value)}
                  />
                </label>
                <label>
                  Пароль
                  <input
                    type="password"
                    value={password}
                    required
                    maxLength={256}
                    autoComplete="current-password"
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </label>
              </>
            )}
            {!connection && mode === "token" && (
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
              <button disabled={submitting || restoring}>Увійти</button>
            ) : (
              <button
                type="button"
                disabled={submitting || restoring}
                onClick={() => void disconnect()}
              >
                Від’єднатися
              </button>
            )}
          </form>
          <small>
            Пароль не зберігається панеллю. Сесія використовує захищену HttpOnly
            cookie; токен читання — лише пам’ять вкладки. Перегляд одного
            Monitor.
          </small>
        </section>
        {session && (
          <section>
            <details>
              <summary>Змінити власний пароль</summary>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void updatePassword();
                }}
              >
                <label>
                  Поточний пароль
                  <input
                    type="password"
                    required
                    maxLength={256}
                    autoComplete="current-password"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                  />
                </label>
                <label>
                  Новий пароль
                  <input
                    type="password"
                    required
                    maxLength={256}
                    autoComplete="new-password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                  />
                </label>
                <button disabled={submitting}>Зберегти пароль</button>
              </form>
              <small>
                Від 16 до 256 байтів UTF-8. Зміна завершить усі попередні сесії
                цього користувача.
              </small>
              {passwordNotice && <p role="status">{passwordNotice}</p>}
            </details>
          </section>
        )}
        {session?.role === "admin" &&
          session.scopes.includes("system:admin") && (
            <Users
              key={session.userId}
              session={session}
              onOwnChange={() => {
                setSession(null);
                setConnection(null);
                setAssessment(null);
                setCurrentPassword("");
                setNewPassword("");
                setPasswordNotice("");
                setError("Права облікового запису змінено. Увійдіть повторно.");
              }}
            />
          )}
        {session && connection && (
          <Configuration
            key={session.userId + connection.monitor}
            session={session}
            monitor={connection.monitor}
          />
        )}
        {session?.scopes.includes("agents:read") && (
          <Agents key={session.userId} />
        )}
        {session?.scopes.includes("diagnostics:read") && connection && (
          <Diagnostics key={session.userId + connection.monitor} monitor={connection.monitor} session={session} />
        )}
        {session && <Tokens key={session.userId} session={session} />}
        {session && connection && (
          <History
            key={session.userId + connection.monitor}
            monitor={connection.monitor}
          />
        )}
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
