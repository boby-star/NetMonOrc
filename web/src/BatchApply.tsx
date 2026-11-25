import { useEffect, useState } from "react";
import {
  applyConfigurationSource,
  validateConfigurationSource,
  planConfigurationSource,
  prepareConfigurationSource,
  operation,
  operations,
  type Session,
  type VersionedResource,
  type ConfigPlan,
  type Operation,
  type OperationPage,
} from "./api";

const states = {
  accepted: "Бажану конфігурацію збережено",
  running: "Застосування плану",
  completed: "Застосування завершено",
  failed: "Операція не завершила застосування",
};
const reasons: Record<string, string> = {
  dependency_catalogued:
    "Зв’язок перевірено та збережено; стан ресурсів не успадковується",
  group_catalogued: "Групу перевірено та збережено; оцінка доступна окремо",
  profile_catalogued: "Профіль збережено в каталозі",
  awaiting_reconciliation: "Очікує узгодження",
  webhook_configuration_loaded: "Конфігурацію webhook завантажено",
  awaiting_webhook_configuration: "Очікується завантаження webhook",
  alert_rule_evaluated: "Правило оброблено Controller",
  awaiting_alert_evaluation: "Очікується оцінювання правила",
  insufficient_placement_agents:
    "Недостатньо допустимих агентів для повного призначення",
  placement_policy_catalogued: "Політику розміщення перевірено та збережено",
  assessment_policy_catalogued: "Політику оцінювання перевірено та збережено",
  component_catalogued: "Component перевірено та збережено",
  check_profile_catalogued: "CheckProfile перевірено та збережено",
  target_catalogued: "Target перевірено та збережено",
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
  onDirtyChange,
}: {
  session: Session;
  current: VersionedResource | null;
  draft: string;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [text, setText] = useState("");
  const [appliedText, setAppliedText] = useState("");
  useEffect(() => {
    onDirtyChange?.(!!text && text !== appliedText);
  }, [text, appliedText, onDirtyChange]);
  const [busy, setBusy] = useState(false);
  const [prepared, setPrepared] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [preview, setPreview] = useState<ConfigPlan | null>(null);
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
  async function fill() {
    setPrepared(false);
    setError("");
    setNotice("");
    setPreview(null);
    setBusy(true);
    try {
      const normalized = await prepareConfigurationSource(session, draft);
      if (normalized.documents.length !== 1)
        throw new Error(
          "Вставте кілька документів безпосередньо в пакетний редактор.",
        );
      const document = normalized.documents[0];
      const key = document.kind + "/" + document.name;
      const expected =
        current &&
        current.resource.manifest.kind === document.kind &&
        current.resource.manifest.metadata.name === document.name
          ? current.resource.resourceVersion
          : null;
      setText(`{
  "manifests": [${document.source}],
  "expectedVersions": ${JSON.stringify({ [key]: expected })},
  "requestId": "${normalized.requestId}"
}`);
      setPrepared(true);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Не вдалося прочитати маніфест.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function submit(action: "validate" | "plan" | "apply") {
    setBusy(true);
    setError("");
    setNotice("");
    setPreview(null);
    try {
      if (action === "validate") {
        setNotice(
          JSON.stringify(
            await validateConfigurationSource(session, text),
            null,
            2,
          ),
        );
        return;
      }
      if (action === "plan") {
        const plan = await planConfigurationSource(session, text);
        setPreview(plan);
        setNotice(JSON.stringify(plan, null, 2));
        return;
      }
      if (!prepared)
        throw new Error(
          "Спочатку підготуйте пакет і перевірте версії ресурсів.",
        );
      setSelected(null);
      const accepted = await applyConfigurationSource(session, text);
      setSelected(accepted);
      setAppliedText(text);
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
  async function prepare() {
    setBusy(true);
    setError("");
    setNotice("");
    setPreview(null);
    setPrepared(false);
    try {
      const result = await prepareConfigurationSource(session, text);
      setText(result.source);
      setPrepared(true);
      setNotice(
        `Підготовлено ресурсів: ${result.resourceCount}. Перевірте версії та план перед застосуванням.`,
      );
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Не вдалося підготувати пакет.",
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
    <details aria-busy={busy}>
      <summary>Пакетне застосування й операції</summary>
      {canWrite && (
        <>
          <p>
            До 20 ресурсів Component, Dependency, ResourceGroup,
            PlacementPolicy, AssessmentPolicy, CheckProfile, Target, Monitor,
            DiagnosticProfile, AlertRule і WebhookEndpoint в одному YAML або
            JSON-пакеті. YAML-документи розділяйте ---; посилання можуть вести
            на наступні документи. Для наявного ресурсу задайте його
            resourceVersion, для нового — null. Збереження всіх ресурсів
            виконується однією транзакцією.
          </p>
          <button
            type="button"
            disabled={busy || !draft}
            onClick={() => void fill()}
          >
            Підготувати пакет із відкритого маніфесту
          </button>
          <label>
            Пакет YAML / JSON
            <textarea
              className="manifest-editor"
              value={text}
              disabled={busy}
              spellCheck={false}
              onChange={(e) => {
                setText(e.target.value);
                setPrepared(false);
                setPreview(null);
                setNotice("");
              }}
            />
          </label>
          <div className="row">
            <button
              type="button"
              disabled={busy || !text}
              onClick={() => void prepare()}
            >
              Підготувати пакет
            </button>
            <button
              type="button"
              disabled={busy || !text}
              onClick={() => void submit("validate")}
            >
              Перевірити пакет
            </button>
            <button
              type="button"
              disabled={busy || !text}
              onClick={() => void submit("plan")}
            >
              Показати план пакета
            </button>
            <button
              type="button"
              disabled={busy || !text || !prepared}
              onClick={() => void submit("apply")}
            >
              Застосувати пакет
            </button>
          </div>
          <small>
            Підготовка заповнює відсутні версії, а явно задані умови зберігає.
            Якщо відповідь втрачена, повторіть незмінний пакет із тим самим
            requestId. Для іншого пакета потрібний новий requestId. Поточний
            Controller виконує придатні Monitor із каталогу.
          </small>
        </>
      )}
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {preview && (preview.placements?.length ?? 0) > 0 && (
        <div>
          <h3>Заплановані точки перевірок</h3>
          <ul>
            {(preview.placements ?? []).map((placement) => (
              <li key={`${placement.monitorId}/${placement.checkId ?? ""}`}>
                {placement.monitorId}
                {placement.checkId ? ` / ${placement.checkId}` : ""}
                {placement.distinctBy ? ` (${placement.distinctBy})` : ""}:{" "}
                {placement.pending
                  ? `очікує ${placement.replicas} допустимих агентів; регулярні перевірки не запускаються`
                  : placement.selectedAgents
                      .map((agent) => `${agent.agentId} (${agent.networkZone})`)
                      .join(", ")}
              </li>
            ))}
          </ul>
          <small>
            Попередній план не підтверджує доступність агентів або стан сервісу.
          </small>
        </div>
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
                {t.kind}/{t.name}
                {t.implicit ? " (залежний ресурс)" : ""}:{" "}
                {reasons[t.reason] ?? t.reason}
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
