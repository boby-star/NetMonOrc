import { useEffect, useState } from "react";
import { BatchApply } from "./BatchApply";
import {
  monitorResource,
  resourceCatalog,
  validateConfigurationSource,
  planConfigurationSource,
  prepareConfigurationSource,
  saveResourceSource,
  deleteMonitor,
  type Session,
  type VersionedResource,
  type MonitorResource,
} from "./api";

export function Configuration({
  session,
  monitor,
  onDirtyChange,
}: {
  session: Session;
  monitor: string;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [resourceKind, setResourceKind] = useState("Monitor");
  const [name, setName] = useState(monitor);
  const [current, setCurrent] = useState<VersionedResource | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  const [page, setPage] = useState<{
    items: MonitorResource[];
    nextCursor: string | null;
  } | null>(null);
  const canWrite =
    session.role === "admin" && session.scopes.includes("resources:write");
  useEffect(() => {
    let active = true;
    void monitorResource(monitor)
      .then((v) => {
        if (active) {
          setCurrent(v);
          setText(v.source);
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
  const [batchDirty, setBatchDirty] = useState(false);
  const dirty = text !== (current?.source ?? "");
  useEffect(() => {
    onDirtyChange?.(dirty || batchDirty);
  }, [dirty, batchDirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);
  function discardDraft() {
    return (
      !dirty || window.confirm("Є незбережений маніфест. Відкинути зміни?")
    );
  }
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
    if (kind === "load" && !discardDraft()) return;
    if (
      kind === "delete" &&
      !window.confirm(
        `Видалити ${current?.resource.manifest.kind} «${current?.resource.manifest.metadata.name}»? Історія залишиться на сервері.`,
      )
    )
      return;
    setBusy(true);
    setError("");
    setResult("");
    try {
      if (kind === "load") {
        const loaded = await monitorResource(targetName, resourceKind);
        setName(targetName);
        setCurrent(loaded);
        setText(loaded.source);
        return;
      }
      if (kind === "delete") {
        if (!current) return;
        await deleteMonitor(session, current);
        setCurrent(null);
        setText("");
        setResult("Ресурс видалено. Історія змін збережена сервером.");
        return;
      }
      if (kind === "validate") {
        setResult(
          JSON.stringify(
            await validateConfigurationSource(session, text),
            null,
            2,
          ),
        );
        return;
      }
      if (kind === "plan") {
        setResult(
          JSON.stringify(await planConfigurationSource(session, text), null, 2),
        );
        return;
      }
      if (kind === "save" && !current)
        throw new Error("Спочатку завантажте ресурс для заміни.");
      const prepared = await prepareConfigurationSource(session, text);
      if (prepared.documents.length !== 1)
        throw new Error(
          "Для кількох документів використайте пакетне застосування нижче.",
        );
      const document = prepared.documents[0];
      if (
        kind === "save" &&
        current &&
        (document.kind !== current.resource.manifest.kind ||
          document.name !== current.resource.manifest.metadata.name)
      )
        throw new Error("Тип і назву ресурсу не можна змінити під час заміни.");
      const saved = await saveResourceSource(
        session,
        text,
        document,
        kind === "create" ? null : current,
      );
      setResourceKind(saved.resource.manifest.kind);
      setCurrent(saved);
      setName(saved.resource.manifest.metadata.name);
      setText(saved.source);
      setResult(
        "Збережено. Controller завантажить нову ревізію Monitor і передасть плани агентам; оновіть запис за кілька секунд. AlertRule підхоплюються чинним runtime автоматично. WebhookEndpoint застосовується через перезавантаження конфігурації Controller.",
      );
    } catch (e) {
      setError(
        e instanceof SyntaxError
          ? "Некоректний YAML/JSON. Перевірте синтаксис."
          : e instanceof Error
            ? e.message
            : "Операцію не підтверджено.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-busy={busy}>
      <h2>Конфігурація моніторингу</h2>
      {!canWrite && (
        <p className="notice">
          Режим перегляду. Для зміни конфігурації потрібні права адміністратора.
        </p>
      )}
      {(dirty || batchDirty) && (
        <p className="notice" role="status">
          Незбережені зміни. Перемикання розділів зберігає вашу чернетку.
        </p>
      )}
      <p>
        Один декларативний маніфест у YAML або JSON. Перевірку та компіляцію
        виконує сервер. Controller виконує Monitor із каталогу у фіксованих
        мережевих зонах; вибір Monitor у панелі змінює перегляд.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action("load");
        }}
      >
        <label>
          Тип ресурсу
          <select
            value={resourceKind}
            disabled={busy}
            onChange={(e) => {
              if (!discardDraft()) return;
              setResourceKind(e.target.value);
              setCurrent(null);
              setText("");
              setResult("");
              setPage(null);
              setError("");
              setName(e.target.value === "Monitor" ? monitor : "");
            }}
          >
            <option value="Dependency">Dependency</option>
            <option value="ResourceGroup">ResourceGroup</option>
            <option value="Component">Component</option>
            <option value="PlacementPolicy">PlacementPolicy</option>
            <option value="AssessmentPolicy">AssessmentPolicy</option>
            <option value="CheckProfile">CheckProfile</option>
            <option value="Target">Target</option>
            <option value="Monitor">Monitor</option>
            <option value="DiagnosticProfile">DiagnosticProfile</option>
            <option value="AlertRule">AlertRule</option>
            <option value="WebhookEndpoint">WebhookEndpoint</option>
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
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    void action("load", item.manifest.metadata.name)
                  }
                >
                  {item.manifest.metadata.name}
                </button>{" "}
                · {item.status?.state ?? "невідомо"}
              </li>
            ))}
          </ul>
          <button
            type="button"
            disabled={busy || !page.nextCursor}
            onClick={() => void list(page.nextCursor)}
          >
            Наступна сторінка
          </button>
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
        Маніфест YAML / JSON
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
            className="secondary"
            onClick={() => void action("validate")}
          >
            Перевірити
          </button>
          <button
            type="button"
            disabled={busy || !text}
            className="secondary"
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
            className="danger"
            disabled={busy || !current}
            onClick={() => void action("delete")}
          >
            Видалити ресурс
          </button>
        </div>
      )}
      <small>
        Для нового ресурсу змініть metadata.name у маніфесті. Зміни профілю
        оновлюють ревізії залежних Monitor. Профіль з увімкненими залежностями
        та активний Monitor захищені від видалення. Збереження поточного
        використовує серверну версію; після конфлікту завантажте актуальний
        запис. Статус плану не є оцінкою доступності ресурсу.
      </small>
      <BatchApply
        session={session}
        current={current}
        draft={text}
        onDirtyChange={setBatchDirty}
      />
      {busy && <p role="status">Виконання…</p>}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {result && (
        <pre role="status" className="config-result">
          {result}
        </pre>
      )}
    </section>
  );
}
