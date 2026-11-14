import { useEffect, useState } from "react";
import { BatchApply } from "./BatchApply";
import {
  monitorResource,
  resourceCatalog,
  checkConfiguration,
  planConfiguration,
  saveMonitor,
  deleteMonitor,
  type Session,
  type VersionedResource,
  type ResourceManifest,
  type MonitorResource,
} from "./api";

export function Configuration({
  session,
  monitor,
}: {
  session: Session;
  monitor: string;
}) {
  const [resourceKind, setResourceKind] = useState("Monitor");
  const [name, setName] = useState(monitor);
  const [current, setCurrent] = useState<VersionedResource | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [page, setPage] = useState<{ items: MonitorResource[]; nextCursor: string | null } | null>(null);
  const canWrite =
    session.role === "admin" && session.scopes.includes("resources:write");
  useEffect(() => {
    let active = true;
    void monitorResource(monitor)
      .then((v) => {
        if (active) {
          setCurrent(v);
          setText(JSON.stringify(v.resource.manifest, null, 2));
        }
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : "Конфігурація недоступна.");
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
    };
  }, [monitor]);
  async function list(cursor: string | null) {
    setBusy(true);
    setError("");
    try {
      setPage(await resourceCatalog(resourceKind, cursor));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Каталог недоступний.");
    } finally {
      setBusy(false);
    }
  }
  async function action(
    kind: "load" | "validate" | "plan" | "save" | "create" | "delete",
    targetName = name,
  ) {
    setBusy(true);
    setError("");
    setResult("");
    try {
      if (kind === "load") {
        const loaded = await monitorResource(targetName, resourceKind);
        setName(targetName);
        setCurrent(loaded);
        setText(JSON.stringify(loaded.resource.manifest, null, 2));
        return;
      }
      if (kind === "delete") {
        if (!current) return;
        await deleteMonitor(session, current);
        setCurrent(null);
        setResult("Ресурс видалено. Історія змін збережена сервером.");
        return;
      }
      const manifest = JSON.parse(text) as ResourceManifest;
      if (
        kind === "save" && current &&
        manifest.kind !== current.resource.manifest.kind
      )
        throw new Error("Тип ресурсу не можна змінити під час заміни.");
      if (kind === "validate") {
        setResult(
          JSON.stringify(await checkConfiguration(session, manifest), null, 2),
        );
        return;
      }
      if (kind === "plan") {
        setResult(
          JSON.stringify(await planConfiguration(session, manifest), null, 2),
        );
        return;
      }
      if (kind === "save" && !current)
        throw new Error("Спочатку завантажте ресурс для заміни.");
      const saved = await saveMonitor(
        session,
        manifest,
        kind === "create" ? null : current,
      );
      setResourceKind(saved.resource.manifest.kind);
      setCurrent(saved);
      setName(saved.resource.manifest.metadata.name);
      setText(JSON.stringify(saved.resource.manifest, null, 2));
      setResult(
        "Збережено. Для активного Monitor Controller завантажить нову ревізію; оновіть запис за кілька секунд. Інші Monitor очікують вибору Controller.",
      );
    } catch (e) {
      setError(
        e instanceof SyntaxError
          ? "Некоректний JSON. Перевірте синтаксис."
          : e instanceof Error
            ? e.message
            : "Операцію не підтверджено.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h2>Конфігурація моніторингу</h2>
      <p>
        JSON-подання декларативного маніфесту. Перевірку та компіляцію виконує
        сервер. Зараз Controller виконує один вибраний Monitor із фіксованими
        зонами.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action("load");
        }}
      >
        <label>
          Тип ресурсу
          <select value={resourceKind} disabled={busy} onChange={(e) => {
            setResourceKind(e.target.value);
            setCurrent(null);
            setText("");
            setResult("");
            setPage(null);
            setError("");
            setName(e.target.value === "Monitor" ? monitor : "");
          }}>
            <option value="Monitor">Monitor</option>
            <option value="DiagnosticProfile">DiagnosticProfile</option>
          </select>
        </label>
        <label>
          Назва ресурсу
          <input
            value={name}
            required
            disabled={busy}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <button disabled={busy}>Завантажити</button>
      </form>
      <button type="button" disabled={busy} onClick={() => void list(null)}>
        Оновити каталог {resourceKind}
      </button>
      {page && (
        <div>
          {page.items.length === 0 && <p>Ресурсів цього типу немає.</p>}
          <ul>
            {page.items.map((item) => (
              <li key={item.uid}>
                <button type="button" disabled={busy}
                  onClick={() => void action("load", item.manifest.metadata.name)}>
                  {item.manifest.metadata.name}
                </button>{" "}· {item.status?.state ?? "невідомо"}
              </li>
            ))}
          </ul>
          <button type="button" disabled={busy || !page.nextCursor}
            onClick={() => void list(page.nextCursor)}>Наступна сторінка</button>
        </div>
      )}
      {current && (
        <p>
          Версія: <code>{current.resource.resourceVersion}</code> · generation{" "}
          {current.resource.generation}
          <br />
          План Controller: {current.resource.status?.state ?? "невідомо"} ·{" "}
          {current.resource.status?.desiredRevision}
        </p>
      )}
      <label>
        Маніфест JSON
        <textarea
          className="manifest-editor"
          value={text}
          readOnly={!canWrite}
          disabled={busy}
          spellCheck={false}
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      {canWrite && (
        <div className="row">
          <button
            type="button"
            disabled={busy || !text}
            onClick={() => void action("validate")}
          >
            Перевірити
          </button>
          <button
            type="button"
            disabled={busy || !text}
            onClick={() => void action("plan")}
          >
            Показати план
          </button>
          <button
            type="button"
            disabled={busy || !current}
            onClick={() => void action("save")}
          >
            Зберегти поточний
          </button>
          <button
            type="button"
            disabled={busy || !text}
            onClick={() => void action("create")}
          >
            Створити новий ресурс
          </button>
          <button
            type="button"
            disabled={busy || !current}
            onClick={() => void action("delete")}
          >
            Видалити ресурс
          </button>
        </div>
      )}
      <small>
        Для нового ресурсу змініть metadata.name у маніфесті. Зміни профілю
        оновлюють ревізії залежних Monitor. Профіль з увімкненими залежностями та активний Monitor
        захищені від видалення. Збереження поточного використовує серверну
        версію; після конфлікту завантажте актуальний запис. Статус плану не є
        оцінкою доступності ресурсу.
      </small>
      <BatchApply session={session} current={current} draft={text} />
      {busy && <p role="status">Виконання…</p>}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {result && <pre className="config-result">{result}</pre>}
    </section>
  );
}
