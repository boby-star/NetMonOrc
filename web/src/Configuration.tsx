import { useEffect, useState } from "react";
import {
  monitorResource,
  checkConfiguration,
  planConfiguration,
  saveMonitor,
  deleteMonitor,
  type Session,
  type VersionedResource,
  type ResourceManifest,
} from "./api";

export function Configuration({
  session,
  monitor,
}: {
  session: Session;
  monitor: string;
}) {
  const [name, setName] = useState(monitor);
  const [current, setCurrent] = useState<VersionedResource | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
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
      });
    return () => {
      active = false;
    };
  }, [monitor]);
  async function action(
    kind: "load" | "validate" | "plan" | "save" | "create" | "delete",
  ) {
    setBusy(true);
    setError("");
    setResult("");
    try {
      if (kind === "load") {
        const loaded = await monitorResource(name);
        setCurrent(loaded);
        setText(JSON.stringify(loaded.resource.manifest, null, 2));
        return;
      }
      if (kind === "delete") {
        if (!current) return;
        await deleteMonitor(session, current);
        setCurrent(null);
        setResult("Monitor видалено. Історія змін збережена сервером.");
        return;
      }
      const manifest = JSON.parse(text) as ResourceManifest;
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
        throw new Error("Спочатку завантажте Monitor для заміни.");
      const saved = await saveMonitor(
        session,
        manifest,
        kind === "create" ? null : current,
      );
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
      <h2>Конфігурація Monitor</h2>
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
          Назва Monitor
          <input
            value={name}
            required
            disabled={busy}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <button disabled={busy}>Завантажити</button>
      </form>
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
            Створити новий Monitor
          </button>
          <button
            type="button"
            disabled={busy || !current}
            onClick={() => void action("delete")}
          >
            Видалити Monitor
          </button>
        </div>
      )}
      <small>
        Для нового Monitor змініть metadata.name у маніфесті. Активний Monitor
        захищений від видалення. Збереження поточного використовує серверну
        версію; після конфлікту завантажте актуальний запис. Статус плану не є
        оцінкою доступності ресурсу.
      </small>
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
