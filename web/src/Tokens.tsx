import { useEffect, useState } from "react";
import {
  tokens,
  issueToken,
  revokeToken,
  type Session,
  type TokenPage,
  type IssuedToken,
} from "./api";

export function Tokens({ session }: { session: Session }) {
  const [page, setPage] = useState<TokenPage | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["resources:read"]);
  const [expiry, setExpiry] = useState("");
  const [issued, setIssued] = useState<IssuedToken | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setLoading(true);
    setPage(null);
    void tokens(cursor, controller.signal)
      .then((p) => {
        if (active) setPage(p);
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : "Список недоступний.");
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [cursor, refresh]);
  async function create() {
    setBusy(true);
    setError("");
    setIssued(null);
    try {
      setIssued(
        await issueToken(
          session,
          name.trim(),
          scopes,
          expiry ? new Date(expiry).toISOString() : null,
        ),
      );
      setName("");
      setCursor(null);
      setRefresh((x) => x + 1);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Видача не підтверджена.");
    } finally {
      setBusy(false);
    }
  }
  async function revoke(id: string) {
    setBusy(true);
    setError("");
    try {
      await revokeToken(session, id);
      if (issued?.id === id) setIssued(null);
      setRefresh((x) => x + 1);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Відкликання не підтверджено.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h2>Особисті API-токени</h2>
      <p>
        Для CLI. Секрет доступний лише після створення; збережіть його у
        захищеному місці. Не додавайте токен у Git або командний рядок.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void create();
        }}
      >
        <label>
          Назва
          <input
            value={name}
            required
            maxLength={128}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label>
          Строк дії
          <input
            type="datetime-local"
            value={expiry}
            onChange={(e) => setExpiry(e.target.value)}
          />
          <small>Порожнє поле — без строку дії.</small>
        </label>
        <fieldset>
          <legend>Дозволи</legend>
          {session.scopes.map((scope) => (
            <label className="scope-option" key={scope}>
              <input
                type="checkbox"
                checked={scopes.includes(scope)}
                onChange={(e) =>
                  setScopes((prev) =>
                    e.target.checked
                      ? [...prev, scope]
                      : prev.filter((s) => s !== scope),
                  )
                }
              />
              {scope}
            </label>
          ))}
        </fieldset>
        <button disabled={busy || loading || !!issued}>Створити токен</button>
      </form>
      {issued && (
        <div role="status">
          <p>Секрет токена «{issued.name}». Сервер не покаже його повторно.</p>
          <pre className="token-secret">{issued.token}</pre>
          <button type="button" onClick={() => setIssued(null)}>
            Приховати секрет
          </button>
        </div>
      )}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {loading && <p role="status">Оновлення списку…</p>}
      {page && (
        <div className="table-scroll">
          <table>
            <caption>Метадані ваших токенів; секретів у списку немає</caption>
            <thead>
              <tr>
                <th>Назва / ID</th>
                <th>Дозволи</th>
                <th>Строк / стан</th>
                <th>Дія</th>
              </tr>
            </thead>
            <tbody>
              {page.items.map((t) => (
                <tr key={t.id}>
                  <td>
                    {t.name}
                    <br />
                    <small>{t.id}</small>
                  </td>
                  <td>{t.scopes.join(", ") || "Без дозволів до ресурсів"}</td>
                  <td>
                    {t.expiresAt
                      ? new Date(t.expiresAt).toLocaleString("uk-UA")
                      : "Без строку"}
                    <br />
                    {t.revokedAt
                      ? "Відкликаний"
                      : t.expiresAt && Date.parse(t.expiresAt) <= Date.now()
                        ? "Прострочений"
                        : "Чинний"}
                  </td>
                  <td>
                    <button
                      type="button"
                      disabled={busy || !!t.revokedAt}
                      onClick={() => void revoke(t.id)}
                    >
                      Відкликати
                    </button>
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
          disabled={busy || loading}
          onClick={() => {
            setCursor(null);
            setRefresh((x) => x + 1);
            setError("");
          }}
        >
          Оновити список
        </button>
        <button
          type="button"
          disabled={busy || loading || !page?.nextCursor}
          onClick={() => setCursor(page?.nextCursor ?? null)}
        >
          Наступна сторінка
        </button>
      </div>
    </section>
  );
}
