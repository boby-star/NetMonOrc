import { useEffect, useRef, useState, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import {
  loadAssessment,
  resourceCatalog,
  login,
  changePassword,
  logout,
  sessionInfo,
  AccessError,
  type Session,
  type Assessment,
} from "./api";
import "./style.css";
import { Dependencies } from "./Dependencies";
import { ResourceGroups } from "./ResourceGroups";
import { Overview } from "./Overview";
import { History } from "./History";
import { EvidenceLinks } from "./EvidenceLinks";
import { Alerts } from "./Alerts";
import { Tokens } from "./Tokens";
import { Users } from "./Users";
import { Diagnostics } from "./Diagnostics";
import { Fallbacks } from "./Fallbacks";
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
const pages = [
  {
    id: "overview",
    label: "Огляд",
    icon: "overview",
    group: "Моніторинг",
    hint: "Стан інфраструктури в одному місці",
  },
  {
    id: "monitor",
    label: "Стан Monitor",
    icon: "pulse",
    group: "Моніторинг",
    hint: "Доступність, покриття та докази з мережевих зон",
  },
  {
    id: "agents",
    label: "Агенти",
    icon: "server",
    group: "Моніторинг",
    hint: "Підключення та призначення джерел перевірок",
    scope: "agents:read",
  },
  {
    id: "history",
    label: "Історія перевірок",
    icon: "clock",
    group: "Моніторинг",
    hint: "Спостереження та первинні докази",
    scope: "resources:read",
  },
  {
    id: "alerts",
    label: "Алерти",
    icon: "bell",
    group: "Дослідження",
    hint: "Активні сповіщення, підтвердження та доставка",
    scope: "alerts:read",
  },
  {
    id: "diagnostics",
    label: "Діагностика",
    icon: "search",
    group: "Дослідження",
    hint: "Сесії, фізичні завдання та пояснення результатів",
    scope: "diagnostics:read",
  },
  {
    id: "fallbacks",
    label: "Резервні спроби",
    icon: "retry",
    group: "Дослідження",
    hint: "Технічні помилки виконавців та резервні перевірки",
    scope: "resources:read",
  },
  {
    id: "dependencies",
    label: "Залежності",
    icon: "graph",
    group: "Конфігурація",
    hint: "Граф зв’язків між ресурсами",
    scope: "resources:read",
  },
  {
    id: "groups",
    label: "Групи ресурсів",
    icon: "layers",
    group: "Конфігурація",
    hint: "Склад і узагальнені оцінки груп",
    scope: "resources:read",
  },
  {
    id: "configuration",
    label: "YAML-конфігурація",
    icon: "code",
    group: "Конфігурація",
    hint: "Перевірка, планування та застосування маніфестів",
    scope: "resources:read",
  },
  {
    id: "users",
    label: "Користувачі",
    icon: "users",
    group: "Адміністрування",
    hint: "Облікові записи та права доступу",
    scope: "system:admin",
  },
  {
    id: "tokens",
    label: "API-токени",
    icon: "key",
    group: "Адміністрування",
    hint: "Особистий доступ для CLI та інтеграцій",
  },
  {
    id: "account",
    label: "Обліковий запис",
    icon: "user",
    group: "Адміністрування",
    hint: "Безпека вашого облікового запису",
  },
];
function Icon({ name }: { name: string }) {
  const paths: Record<string, string> = {
    overview: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
    pulse: "M2 12h5l3-8 4 16 3-8h5",
    server: "M4 3h16v7H4z M4 14h16v7H4z M7 6h.01 M7 17h.01",
    clock: "M12 8v5l3 2 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
    bell: "M5 17h14l-2-3V9a5 5 0 0 0-10 0v5z M10 21h4",
    search: "m21 21-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
    retry: "M3 11a9 9 0 1 1 2 7 M3 4v7h7",
    graph: "M9 4h6v6H9z M2 17h6v5H2z M16 17h6v5h-6z M12 10v4 M5 17v-3h14v3",
    layers: "m12 2 10 6-10 6L2 8z M2 12l10 6 10-6 M2 16l10 6 10-6",
    code: "m8 6-6 6 6 6 M16 6l6 6-6 6 M14 3l-4 18",
    users:
      "M16 21v-3a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v3 M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0 M17 3a4 4 0 0 1 0 8 M22 21v-3a4 4 0 0 0-3-4",
    key: "M14 8a5 5 0 1 1-10 0 5 5 0 0 1 10 0 M12 12l10 10 M18 18l3-3",
    user: "M20 21v-3a6 6 0 0 0-6-6h-4a6 6 0 0 0-6 6v3 M16 5a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
    menu: "M3 6h18 M3 12h18 M3 18h18",
    exit: "M10 3H4v18h6 M14 8l5 4-5 4 M8 12h11",
  };
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={paths[name] ?? paths.overview} />
    </svg>
  );
}
function initialPage() {
  const id = window.location.hash.slice(1);
  return pages.some((p) => p.id === id) ? id : "overview";
}
function App() {
  const [view, setView] = useState(initialPage);
  const [visited, setVisited] = useState(() => new Set([initialPage()]));
  const [menuOpen, setMenuOpen] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [monitorOptions, setMonitorOptions] = useState<string[]>([]);
  const [refreshTick, setRefreshTick] = useState(0);
  const heading = useRef<HTMLHeadingElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const [monitor, setMonitor] = useState(() => {
    try {
      const saved = sessionStorage.getItem("netmonorc.monitor");
      return saved && /^[a-z][a-z0-9-]{0,62}$/.test(saved)
        ? saved
        : "research-web";
    } catch {
      return "research-web";
    }
  });
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
    try {
      sessionStorage.setItem("netmonorc.monitor", connection.monitor);
    } catch {
      /* Browsers may disable storage; the live session still works. */
    }
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
          setLoadError("");
        }
      } catch (e) {
        if (!closed) {
          if (e instanceof AccessError) {
            setConnection(null);
            setSession(null);
            setAssessment(null);
            setError("Сесія завершилася. Увійдіть знову.");
          }
          setLoadError(
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
  }, [connection, refreshTick]);
  useEffect(() => {
    let active = true;
    void sessionInfo()
      .then((current) => {
        if (active && current) {
          setSession(current);
          setConnection({ monitor, token: "" });
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
    setLoadError("");
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
    if (
      dirty &&
      !window.confirm("Вийти та втратити незбережені зміни маніфесту?")
    )
      return;
    setSubmitting(true);
    try {
      if (session) await logout(session);
      setCurrentPassword("");
      setNewPassword("");
      setPasswordNotice("");
      setSession(null);
      setConnection(null);
      setDirty(false);
      setVisited(new Set(["overview"]));
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
      // Abort reads carrying the revoked cookie before polling the rotated session.
      setConnection((current) => (current ? { ...current } : null));
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
  const allowed = pages.filter(
    (p) =>
      connection &&
      (session
        ? (!p.scope || session.scopes.includes(p.scope)) &&
          (p.id !== "users" || session.role === "admin")
        : p.id === "monitor"),
  );
  const active = allowed.find((p) => p.id === view) ?? allowed[0];
  const age = assessment
    ? (now - Date.parse(assessment.evaluatedAt)) / 1000
    : Infinity;
  const stale = !!loadError || !Number.isFinite(age) || age > 10 || age < -5;
  function navigate(id: string) {
    window.location.hash = id;
    setView(id);
    setVisited((v) => new Set([...v, id]));
    setMenuOpen(false);
  }
  useEffect(() => {
    const change = () => {
      const id = window.location.hash.slice(1);
      if (!pages.some((p) => p.id === id)) return;
      setView(id);
      setVisited((v) => new Set([...v, id]));
      setMenuOpen(false);
    };
    window.addEventListener("hashchange", change);
    return () => window.removeEventListener("hashchange", change);
  }, []);
  useEffect(() => {
    if (active && active.id !== view) navigate(active.id);
  }, [active?.id, view]);
  useEffect(() => {
    heading.current?.focus();
  }, [view]);
  useEffect(() => {
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape" && menuOpen) {
        setMenuOpen(false);
        menuButton.current?.focus();
      }
    };
    window.addEventListener("keydown", escape);
    return () => window.removeEventListener("keydown", escape);
  }, [menuOpen]);
  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => {
      if (dirty) e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  useEffect(() => {
    if (!session?.scopes.includes("resources:read")) return;
    let alive = true;
    void resourceCatalog("Monitor", null)
      .then((p) => {
        if (!alive) return;
        const names = p.items.map((x) => x.manifest.metadata.name);
        setMonitorOptions(names);
        if (
          names.length &&
          !names.includes(monitor) &&
          monitor === "research-web"
        ) {
          setMonitor(names[0]);
          setAssessment(null);
          setConnection({ monitor: names[0], token: "" });
        }
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [session?.userId]);
  function panel(id: string, children: ReactNode) {
    return visited.has(id) || active?.id === id ? (
      <div data-page={id} hidden={active?.id !== id}>
        {children}
      </div>
    ) : null;
  }
  function switchMonitor() {
    if (
      !connection ||
      monitor === connection.monitor ||
      !/^[a-z][a-z0-9-]{0,62}$/.test(monitor)
    )
      return;
    if (
      dirty &&
      !window.confirm("Змінити Monitor та втратити незбережений маніфест?")
    )
      return;
    setDirty(false);
    setAssessment(null);
    setLoadError("");
    setConnection({ monitor, token: connection.token });
  }
  return (
    <div className={connection ? "app-shell" : "auth-shell"}>
      <a
        className="skip-link"
        href="#main-content"
        onClick={(e) => {
          e.preventDefault();
          document.getElementById("main-content")?.focus();
        }}
      >
        Перейти до вмісту
      </a>
      {connection && (
        <>
          {menuOpen && (
            <button
              className="nav-backdrop"
              aria-label="Закрити навігацію"
              onClick={() => {
                setMenuOpen(false);
                menuButton.current?.focus();
              }}
            />
          )}
          <aside
            id="app-navigation"
            className={"sidebar" + (menuOpen ? " is-open" : "")}
            aria-label="Бокова панель"
          >
            <a
              className="brand"
              href="#overview"
              onClick={(e) => {
                e.preventDefault();
                navigate(session ? "overview" : "monitor");
              }}
            >
              <span className="brand-mark">
                <Icon name="pulse" />
              </span>
              <span>
                NetMonOrc<small>NETWORK OPERATIONS</small>
              </span>
            </a>
            <nav aria-label="Основна навігація">
              {[...new Set(allowed.map((p) => p.group))].map((group) => (
                <div className="nav-group" key={group}>
                  <p>{group}</p>
                  {allowed
                    .filter((p) => p.group === group)
                    .map((p) => (
                      <a
                        key={p.id}
                        href={"#" + p.id}
                        aria-current={active?.id === p.id ? "page" : undefined}
                        onClick={(e) => {
                          e.preventDefault();
                          navigate(p.id);
                        }}
                      >
                        <Icon name={p.icon} />
                        <span>{p.label}</span>
                        {p.id === "configuration" && dirty && (
                          <span
                            className="draft-dot"
                            title="Незбережені зміни"
                            aria-label="Незбережені зміни"
                          />
                        )}
                      </a>
                    ))}
                </div>
              ))}
            </nav>
            <div className="sidebar-footer">
              <span className="avatar">
                {(session?.username ?? "R").slice(0, 2).toUpperCase()}
              </span>
              <div>
                <strong>{session?.username ?? "Читання"}</strong>
                <small>
                  {session?.role === "admin" ? "Адміністратор" : "Перегляд"}
                </small>
              </div>
              <button
                className="icon-button"
                aria-label="Вийти з облікового запису"
                disabled={submitting}
                onClick={() => void disconnect()}
              >
                <Icon name="exit" />
              </button>
            </div>
          </aside>
          <header className="topbar">
            <div className="topbar-context">
              <button
                ref={menuButton}
                className="icon-button menu-toggle"
                aria-label={
                  menuOpen ? "Закрити навігацію" : "Відкрити навігацію"
                }
                aria-controls="app-navigation"
                aria-expanded={menuOpen}
                onClick={() => setMenuOpen((v) => !v)}
              >
                <Icon name="menu" />
              </button>
              <span>
                Робочий простір <span className="breadcrumb-divider">/</span>{" "}
                <strong>{active?.label}</strong>
              </span>
            </div>
            <span
              className={
                "connection-pill " + (loadError ? "unknown" : "healthy")
              }
            >
              <i />
              {loadError ? "Зв’язок потребує уваги" : "Сесію відкрито"}
            </span>
          </header>
        </>
      )}
      <main id="main-content" tabIndex={-1}>
        {!connection ? (
          <div className="auth-layout">
            <div className="auth-intro">
              <span className="eyebrow">NETMONORC / CONTROL CENTER</span>
              <h1>
                Вся мережа.
                <br />
                <span>Зрозуміла картина.</span>
              </h1>
              <p>
                Перевірки з різних зон, діагностика та докази — в одному
                робочому просторі.
              </p>
              <div className="auth-diagram" aria-hidden="true">
                <span>Agent / office</span>
                <i />
                <b>Controller</b>
                <i />
                <span>Agent / server</span>
              </div>
              <small>Захищене з’єднання · Декларативна конфігурація</small>
            </div>
            <section className="login-card">
              <span className="brand-mark">
                <Icon name="pulse" />
              </span>
              <h2>Вхід до Controller</h2>
              <p>Увійдіть, щоб відкрити робочий простір.</p>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void connect();
                }}
              >
                <label>
                  Спосіб входу
                  <select
                    value={mode}
                    disabled={submitting || restoring}
                    onChange={(e) => {
                      setMode(e.target.value);
                      setError("");
                    }}
                  >
                    <option value="password">Обліковий запис</option>
                    <option value="token">Bootstrap токен читання</option>
                  </select>
                </label>
                {mode === "password" ? (
                  <>
                    <label>
                      Користувач
                      <input
                        value={username}
                        required
                        maxLength={64}
                        autoComplete="username"
                        onChange={(e) => setUsername(e.target.value)}
                        disabled={submitting || restoring}
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
                        disabled={submitting || restoring}
                      />
                    </label>
                  </>
                ) : (
                  <>
                    <label>
                      Monitor
                      <input
                        value={monitor}
                        required
                        pattern="[a-z][a-z0-9-]{0,62}"
                        onChange={(e) => setMonitor(e.target.value)}
                      />
                    </label>
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
                  </>
                )}
                <button disabled={submitting || restoring}>
                  {restoring
                    ? "Перевірка сесії…"
                    : submitting
                      ? "Вхід…"
                      : "Увійти"}
                </button>
              </form>
              {error && (
                <p role="alert" className="error">
                  {error}
                </p>
              )}
              <small>
                Пароль не зберігається панеллю. Сесія захищена HttpOnly cookie.
              </small>
            </section>
          </div>
        ) : (
          <>
            <div className="page-heading">
              <div>
                <span className="eyebrow">РОБОЧИЙ ПРОСТІР</span>
                <h1 ref={heading} tabIndex={-1}>
                  {active?.label}
                </h1>
                <p>{active?.hint}</p>
              </div>
              <div className="refresh-note">
                <span className="live-dot" />
                Оцінка оновлюється кожні 5 с
              </div>
            </div>
            <section className="monitor-toolbar" aria-label="Вибір Monitor">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  switchMonitor();
                }}
              >
                <label>
                  Monitor
                  <input
                    list="monitor-options"
                    value={monitor}
                    required
                    pattern="[a-z][a-z0-9-]{0,62}"
                    maxLength={63}
                    onChange={(e) => setMonitor(e.target.value)}
                  />
                  <datalist id="monitor-options">
                    {monitorOptions.map((name) => (
                      <option key={name} value={name} />
                    ))}
                  </datalist>
                </label>
                <button
                  disabled={
                    submitting ||
                    monitor === connection.monitor ||
                    !/^[a-z][a-z0-9-]{0,62}$/.test(monitor)
                  }
                >
                  Відкрити Monitor
                </button>
              </form>
              <span className="toolbar-meta">
                Перегляд: <strong>{connection.monitor}</strong>
                {dirty && <small>Є незбережений маніфест</small>}
              </span>
            </section>
            {error && (
              <p role="alert" className="error">
                {error}
              </p>
            )}
            {loadError && (
              <p role="alert" className="error">
                {loadError}{" "}
                {assessment &&
                  "Показано останній знімок; він не підтверджує поточну доступність."}{" "}
                <button onClick={() => setRefreshTick((v) => v + 1)}>
                  Повторити запит
                </button>
              </p>
            )}
            {panel(
              "overview",
              session && (
                <Overview
                  canReadAgents={session.scopes.includes("agents:read")}
                  canReadDiagnostics={session.scopes.includes(
                    "diagnostics:read",
                  )}
                />
              ),
            )}
            {panel(
              "monitor",
              <>
                {!assessment && !loadError && (
                  <p role="status">Отримання оцінки…</p>
                )}
                {assessment && (
                  <>
                    <section>
                      <div className="row">
                        <h2>{assessment.monitorId}</h2>
                        <strong
                          className={
                            "badge " + (stale ? "unknown" : assessment.state)
                          }
                        >
                          {stale
                            ? "Застарілий знімок"
                            : labels[assessment.state]}
                        </strong>
                      </div>
                      <div className="assessment-metrics">
                        <div>
                          <span>Повнота доказів</span>
                          <strong>
                            {
                              {
                                sufficient: "Достатня",
                                degraded: "Погіршена",
                                unknown: "Невідома",
                              }[assessment.coverageState]
                            }
                          </strong>
                        </div>
                        <div>
                          <span>Мережеві зони</span>
                          <strong>{assessment.zoneStates.length}</strong>
                        </div>
                        <div>
                          <span>Остання оцінка</span>
                          <strong>
                            <time dateTime={assessment.evaluatedAt}>
                              {new Date(
                                assessment.evaluatedAt,
                              ).toLocaleTimeString("uk-UA")}
                            </time>
                          </strong>
                        </div>
                      </div>
                      {stale && (
                        <p role="alert" className="notice">
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
                      <div className="table-scroll">
                        <table>
                          <thead>
                            <tr>
                              <th>Зона</th>
                              <th>Стан</th>
                              <th>H</th>
                              <th>Q</th>
                              <th>Покриття</th>
                              <th>Причина та докази</th>
                            </tr>
                          </thead>
                          <tbody>
                            {assessment.zoneStates.map((z) => (
                              <tr key={z.zone}>
                                <td>
                                  <strong>{z.zone}</strong>
                                </td>
                                <td>
                                  <span className={"badge " + z.state}>
                                    {labels[z.state]}
                                  </span>
                                </td>
                                <td>{percent(z.h)}</td>
                                <td>{percent(z.q)}</td>
                                <td>
                                  {z.coverage === "complete"
                                    ? "Повне"
                                    : "Неповне"}
                                </td>
                                <td>
                                  {z.reason}
                                  <details>
                                    <summary>
                                      {z.evidence.length} доказів
                                    </summary>
                                    <EvidenceLinks
                                      key={assessment.revision + ":" + z.zone}
                                      ids={z.evidence}
                                      canRead={
                                        session?.scopes.includes(
                                          "resources:read",
                                        ) === true
                                      }
                                    />
                                  </details>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                      <small>
                        H і Q обчислює Controller. Втрата покриття сама по собі
                        не означає відмову сервісу.
                      </small>
                    </section>
                  </>
                )}
              </>,
            )}
            {session && (
              <>
                {session.scopes.includes("resources:read") &&
                  panel(
                    "configuration",
                    <Configuration
                      key={session.userId + connection.monitor}
                      session={session}
                      monitor={connection.monitor}
                      onDirtyChange={setDirty}
                    />,
                  )}
                {session.scopes.includes("agents:read") &&
                  panel("agents", <Agents />)}
                {session.scopes.includes("resources:read") && (
                  <>
                    {panel(
                      "history",
                      <History
                        key={connection.monitor}
                        monitor={connection.monitor}
                      />,
                    )}
                    {panel(
                      "dependencies",
                      <Dependencies
                        key={connection.monitor}
                        monitor={connection.monitor}
                      />,
                    )}
                    {panel("groups", <ResourceGroups />)}
                    {panel(
                      "fallbacks",
                      <Fallbacks
                        key={connection.monitor}
                        monitor={connection.monitor}
                      />,
                    )}
                  </>
                )}
                {session.scopes.includes("diagnostics:read") &&
                  panel(
                    "diagnostics",
                    <Diagnostics
                      key={connection.monitor}
                      monitor={connection.monitor}
                      session={session}
                    />,
                  )}
                {session.scopes.includes("alerts:read") &&
                  panel(
                    "alerts",
                    <Alerts
                      key={connection.monitor}
                      monitor={connection.monitor}
                      session={session}
                    />,
                  )}
                {session.role === "admin" &&
                  session.scopes.includes("system:admin") &&
                  panel(
                    "users",
                    <Users
                      session={session}
                      onOwnChange={() => {
                        setSession(null);
                        setConnection(null);
                        setAssessment(null);
                        setDirty(false);
                        setError(
                          "Права облікового запису змінено. Увійдіть повторно.",
                        );
                      }}
                    />,
                  )}
                {panel("tokens", <Tokens session={session} />)}
                {panel(
                  "account",
                  <section>
                    <h2>Обліковий запис</h2>
                    <p>
                      <strong>{session.username}</strong> ·{" "}
                      {session.role === "admin" ? "Адміністратор" : "Перегляд"}
                    </p>
                    <p>
                      Сесія до{" "}
                      {session.expiresAt
                        ? new Date(session.expiresAt).toLocaleString("uk-UA")
                        : "—"}
                    </p>
                    <h3>Змінити власний пароль</h3>
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
                      <button disabled={submitting}>
                        {submitting ? "Збереження…" : "Зберегти пароль"}
                      </button>
                    </form>
                    <small>
                      16–256 байтів UTF-8. Зміна завершить попередні сесії цього
                      користувача.
                    </small>
                    {passwordNotice && (
                      <p role="status" className="notice">
                        {passwordNotice}
                      </p>
                    )}
                  </section>,
                )}
              </>
            )}
            <footer className="workspace-footer">
              <span>NetMonOrc · комплексний моніторинг</span>
              <span>Оцінки та докази обчислює Controller</span>
            </footer>
          </>
        )}
      </main>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
