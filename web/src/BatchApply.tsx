import { useEffect, useState } from "react";
import {
  applyConfiguration,
  validateBatch,
  operation,
  operations,
  type Session,
  type VersionedResource,
  type ConfigBatch,
  type Operation,
  type OperationPage,
} from "./api";

const states = {
  accepted: "Бажану конфігурацію збережено",
  running: "Застосування плану",
  completed: "Потрібні агенти підтвердили план",
  failed: "Операція не завершила застосування",
};
const reasons: Record<string, string> = {
  awaiting_reconciliation: "Очікує узгодження",
  controller_not_selected: "Monitor не вибрано Controller",
  awaiting_controller_revision: "Очікує нову ревізію Controller",
  awaiting_agent_acknowledgements: "Очікує підтвердження агентів",
  required_agents_applied: "Агенти підтвердили план",
  superseded: "Ресурс видалено або змінено наступним запитом",
};
export function BatchApply({
  session,
  current,
  draft,
}: {
  session: Session;
  current: VersionedResource | null;
  draft: string;
}) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [selected, setSelected] = useState<Operation | null>(null);
  const [progressError, setProgressError] = useState("");
  const [page, setPage] = useState<OperationPage | null>(null);
  const [filter, setFilter] = useState("");
  const canWrite =
    session.role === "admin" && session.scopes.includes("resources:write");
  useEffect(() => {
    if (
      !selected ||
      selected.state === "completed" ||
      selected.state === "failed"
    )
      return;
    let active = true;
    let loading = false;
    const update = async () => {
      if (loading) return;
      loading = true;
      try {
        const next = await operation(selected.id);
        if (active) {
          setSelected(next);
          setProgressError("");
        }
      } catch {
        if (active)
          setProgressError(
            "Статус тимчасово недоступний. Повторимо запит автоматично.",
          );
      } finally {
        loading = false;
      }
    };
    const timer = setInterval(() => void update(), 2000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [selected?.id, selected?.state]);
  function fill() {
    setError("");
    try {
      const manifest = JSON.parse(draft);
      const key = manifest.kind + "/" + manifest.metadata.name;
      const expected =
        current &&
        current.resource.manifest.metadata.name === manifest.metadata.name
          ? current.resource.resourceVersion
          : null;
      setText(
        JSON.stringify(
          {
            manifests: [manifest],
            expectedVersions: { [key]: expected },
            requestId: crypto.randomUUID(),
          },
          null,
          2,
        ),
      );
    } catch {
      setError("Спочатку перевірте JSON відкритого маніфесту.");
    }
  }
  async function submit(validate: boolean) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const batch = JSON.parse(text) as ConfigBatch;
      if (!batch.requestId) {
        batch.requestId = crypto.randomUUID();
        setText(JSON.stringify(batch, null, 2));
      }
      if (validate) {
        setNotice(JSON.stringify(await validateBatch(session, batch), null, 2));
        return;
      }
      setSelected(null);
      const accepted = await applyConfiguration(session, batch);
      setSelected(accepted);
      setProgressError("");
      setNotice(
        "Пакет збережено атомарно. Відстежуйте підтвердження агентів нижче.",
      );
    } catch (e) {
      setError(
        e instanceof SyntaxError
          ? "Некоректний JSON пакета."
          : e instanceof Error
            ? e.message
            : "Відповідь не отримана. Повторіть той самий пакет із тим самим requestId.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function list(cursor: string | null) {
    setBusy(true);
    setError("");
    try {
      setPage(await operations(filter, cursor));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Список недоступний.");
    } finally {
      setBusy(false);
    }
  }
  async function open(id: string) {
    setBusy(true);
    try {
      setSelected(await operation(id));
      setProgressError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Операція недоступна.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <details>
      <summary>Пакетне застосування й операції</summary>
      {canWrite && (
        <>
          <p>
            До 20 Monitor в одному JSON-пакеті. Для наявного ресурсу задайте
            його resourceVersion, для нового — null. Збереження всіх ресурсів
            виконується однією транзакцією.
          </p>
          <button type="button" disabled={busy || !draft} onClick={fill}>
            Підготувати пакет із відкритого маніфесту
          </button>
          <label>
            ConfigBatch JSON
            <textarea
              className="manifest-editor"
              value={text}
              disabled={busy}
              spellCheck={false}
              onChange={(e) => setText(e.target.value)}
            />
          </label>
          <div className="row">
            <button
              type="button"
              disabled={busy || !text}
              onClick={() => void submit(true)}
            >
              Перевірити пакет
            </button>
            <button
              type="button"
              disabled={busy || !text}
              onClick={() => void submit(false)}
            >
              Застосувати пакет
            </button>
          </div>
          <small>
            Якщо відповідь втрачена, повторіть незмінний пакет із тим самим
            requestId. Для іншого пакета потрібний новий requestId. Поточний
            Controller виконує один вибраний Monitor; інші очікують вибору.
          </small>
        </>
      )}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {notice && <pre className="config-result">{notice}</pre>}
      {selected && (
        <div>
          <h3>{states[selected.state]}</h3>
          <p>
            Операція {selected.id} · {selected.startedAt}
          </p>
          {selected.error && (
            <p role="alert" className="error">
              {selected.error.code}: {selected.error.message}
            </p>
          )}
          <ul>
            {selected.result.targets.map((t) => (
              <li key={t.uid}>
                {t.kind}/{t.name}: {reasons[t.reason] ?? t.reason}
                <br />
                <small>
                  Версія {t.resourceVersion} · {t.effectiveRevision}
                </small>
              </li>
            ))}
          </ul>
          {progressError && <p role="status">{progressError}</p>}
          <small>
            Підтвердження конфігурації не означає healthy ресурсу. Доступність
            показує окремий Assessment.
          </small>
        </div>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void list(null);
        }}
      >
        <label>
          Стан операцій
          <select
            value={filter}
            disabled={busy}
            onChange={(e) => {
              setFilter(e.target.value);
              setPage(null);
            }}
          >
            <option value="">Усі</option>
            {Object.entries(states).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <button disabled={busy}>Оновити список операцій</button>
      </form>
      {page && (
        <>
          <div className="table-scroll">
            <table>
              <caption>Збережені операції</caption>
              <thead>
                <tr>
                  <th>Операція</th>
                  <th>Стан</th>
                  <th>Дія</th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((op) => (
                  <tr key={op.id}>
                    <td>
                      {op.id}
                      <br />
                      <small>{op.startedAt}</small>
                    </td>
                    <td>{states[op.state]}</td>
                    <td>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => void open(op.id)}
                      >
                        Переглянути
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <button
            type="button"
            disabled={busy || !page.nextCursor}
            onClick={() => void list(page.nextCursor)}
          >
            Наступна сторінка
          </button>
        </>
      )}
    </details>
  );
}
