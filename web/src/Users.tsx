import { useEffect, useState } from "react";
import {
  users,
  user,
  createUser,
  patchUser,
  type Session,
  type UserPage,
  type VersionedUser,
} from "./api";

export function Users({
  session,
  onOwnChange,
}: {
  session: Session;
  onOwnChange: () => void;
}) {
  const [page, setPage] = useState<UserPage | null>(null);
  const [cursor, setCursor] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [username, setUsername] = useState("");
  const [role, setRole] = useState("viewer");
  const [password, setPassword] = useState("");
  const [selected, setSelected] = useState<VersionedUser | null>(null);
  const [editRole, setEditRole] = useState("viewer");
  const [disabled, setDisabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    setLoading(true);
    setPage(null);
    void users(cursor, controller.signal)
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
    setNotice("");
    try {
      const created = await createUser(session, username, role, password);
      setUsername("");
      setNotice(
        `Створено ${created.user.username}. Передайте початковий пароль захищеним каналом.`,
      );
      setCursor(null);
      setRefresh((x) => x + 1);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Створення не підтверджене.");
    } finally {
      setPassword("");
      setBusy(false);
    }
  }
  async function select(id: string) {
    setBusy(true);
    setError("");
    setNotice("");
    setSelected(null);
    try {
      const current = await user(id);
      setSelected(current);
      setEditRole(current.user.role);
      setDisabled(current.user.disabled);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Запис недоступний.");
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (!selected) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const updated = await patchUser(session, selected, editRole, disabled);
      if (
        updated.user.id === session.userId &&
        (updated.user.role !== selected.user.role ||
          (!selected.user.disabled && updated.user.disabled))
      ) {
        onOwnChange();
        return;
      }
      setSelected(updated);
      setRefresh((x) => x + 1);
      setNotice(
        "Зміни збережені. Зміна ролі або блокування відкликає попередні сесії та API-токени.",
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Зміни не підтверджені.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h2>Користувачі</h2>
      <p>
        Доступ адміністратора. Останнього активного адміністратора система
        захищає від блокування та зниження ролі.
      </p>
      <details>
        <summary>Створити користувача</summary>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void create();
          }}
        >
          <label>
            Ім’я
            <input
              value={username}
              required
              maxLength={64}
              disabled={busy}
              autoComplete="off"
              onChange={(e) => setUsername(e.target.value)}
            />
          </label>
          <label>
            Роль
            <select
              value={role}
              disabled={busy}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="viewer">viewer — перегляд</option>
              <option value="admin">admin — керування</option>
            </select>
          </label>
          <label>
            Початковий пароль
            <input
              type="password"
              value={password}
              required
              maxLength={256}
              disabled={busy}
              autoComplete="new-password"
              onChange={(e) => setPassword(e.target.value)}
            />
          </label>
          <button disabled={busy || loading}>Створити користувача</button>
        </form>
        <small>
          Пароль: 16..256 байтів UTF-8. Сервер не повертає його у відповідях.
        </small>
      </details>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {loading && <p role="status">Завантаження користувачів…</p>}
      {page && (
        <div className="table-scroll">
          <table>
            <caption>Користувачі системи</caption>
            <thead>
              <tr>
                <th>Користувач</th>
                <th>Роль</th>
                <th>Стан</th>
                <th>Дія</th>
              </tr>
            </thead>
            <tbody>
              {page.items.map((u) => (
                <tr key={u.id}>
                  <td>
                    {u.username}
                    {u.id === session.userId ? " (ви)" : ""}
                    <br />
                    <small>{u.id}</small>
                  </td>
                  <td>{u.role}</td>
                  <td>{u.disabled ? "Заблокований" : "Активний"}</td>
                  <td>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void select(u.id)}
                    >
                      Редагувати
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
      {selected && (
        <div>
          <h3>Зміна: {selected.user.username}</h3>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <label>
              Роль
              <select
                value={editRole}
                disabled={busy}
                onChange={(e) => setEditRole(e.target.value)}
              >
                <option value="viewer">viewer</option>
                <option value="admin">admin</option>
              </select>
            </label>
            <label className="scope-option">
              <input
                type="checkbox"
                checked={disabled}
                disabled={busy}
                onChange={(e) => setDisabled(e.target.checked)}
              />
              Заблокувати обліковий запис
            </label>
            <button disabled={busy}>Зберегти зміни</button>
            <button
              type="button"
              disabled={busy}
              onClick={() => setSelected(null)}
            >
              Закрити
            </button>
          </form>
          <small>
            Роль і блокування перевіряє сервер. Повторне ввімкнення не відновлює
            старі сесії чи токени.
          </small>
        </div>
      )}
    </section>
  );
}
