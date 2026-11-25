// Real browser flows against the owned HTTPS stand. Credentials arrive on stdin
// only; failures log a fixed stage name, never CDP expressions or input values.
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import https from "node:https";
import { once } from "node:events";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const config = JSON.parse(input);
input = "";
const origin = new URL(config.origin);
assert.equal(origin.protocol, "https:");
assert.equal(origin.hostname, "localhost");
const browserHome = await mkdtemp(resolve(tmpdir(), "nmo-live-browser-"));
const pause = (ms) => new Promise((r) => setTimeout(r, ms));
let chrome,
  socket,
  stage = "trusted-https";
let debugCapture;
let dialogAccept = false;
const dialogs = [];
const clicks = [];
let downloads = 0;
let pageLoads = 0;
const responses = [];
const failedRequests = [];
const requests = new Map();
setInterval(() => console.log("Browser stage:", stage), 15000).unref();
try {
  const ca = await readFile(config.caFile);
  const tls = await new Promise((resolve, reject) => {
    const req = https.get(
      config.origin + "/api/v1/auth/me",
      { ca, servername: "localhost", timeout: 8000 },
      (res) => {
        res.resume();
        resolve({
          status: res.statusCode,
          protocol: res.socket.getProtocol(),
          authorized: res.socket.authorized,
        });
      },
    );
    req.on("error", reject);
    req.on("timeout", () => req.destroy(Error("TLS timeout")));
  });
  assert.equal(tls.status, 401);
  assert.equal(tls.authorized, true);
  assert.equal(tls.protocol, "TLSv1.3");
  stage = "isolated-browser-trust";
  const nss = resolve(browserHome, ".pki/nssdb");
  await mkdir(nss, { recursive: true, mode: 0o700 });
  execFileSync("certutil", ["-N", "--empty-password", "-d", "sql:" + nss], {
    stdio: "ignore",
  });
  execFileSync(
    "certutil",
    [
      "-A",
      "-d",
      "sql:" + nss,
      "-n",
      "NetMonOrc owned stand CA",
      "-t",
      "C,,",
      "-i",
      config.caFile,
    ],
    { stdio: "ignore" },
  );
  chrome = spawn(
    "/usr/bin/chromium",
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-dev-shm-usage",
      "--disable-background-networking",
      "--disable-component-update",
      "--disable-extensions",
      "--no-first-run",
      "--no-default-browser-check",
      "--remote-debugging-address=127.0.0.1",
      "--remote-debugging-port=0",
      "--user-data-dir=" + resolve(browserHome, "profile"),
      "about:blank",
    ],
    {
      stdio: ["ignore", "ignore", "ignore"],
      env: {
        ...process.env,
        HOME: browserHome,
        XDG_CONFIG_HOME: resolve(browserHome, ".config"),
        XDG_DATA_HOME: resolve(browserHome, ".local/share"),
      },
    },
  );
  let launchError;
  chrome.on("error", (e) => {
    launchError = e;
  });
  let port;
  for (let i = 0; i < 100; i++) {
    if (launchError || chrome.exitCode !== null)
      throw Error("Browser launch failed");
    try {
      port = Number(
        (
          await readFile(
            resolve(browserHome, "profile/DevToolsActivePort"),
            "utf8",
          )
        ).split("\n")[0],
      );
      break;
    } catch {
      await pause(100);
    }
  }
  assert.ok(port);
  const targets = await (
    await fetch(`http://127.0.0.1:${port}/json/list`)
  ).json();
  const target = targets.find((t) => t.type === "page");
  assert.ok(target);
  socket = new WebSocket(target.webSocketDebuggerUrl);
  await once(socket, "open");
  let seq = 0;
  const pending = new Map();
  const errors = [];
  socket.addEventListener("message", (event) => {
    const m = JSON.parse(event.data);
    if (m.id) {
      const p = pending.get(m.id);
      if (p) {
        pending.delete(m.id);
        clearTimeout(p.timer);
        m.error ? p.reject(Error("CDP command failed")) : p.resolve(m.result);
      }
    } else if (m.method === "Page.loadEventFired") pageLoads++;
    else if (m.method === "Runtime.exceptionThrown")
      errors.push("runtime-exception");
    else if (m.method === "Page.javascriptDialogOpening") {
      dialogs.push(m.params.type);
      void call("Page.handleJavaScriptDialog", { accept: dialogAccept }).catch(
        () => {},
      );
      dialogAccept = false;
    } else if (
      m.method === "Browser.downloadProgress" &&
      m.params.state === "completed"
    ) {
      downloads++;
    } else if (m.method === "Network.requestWillBeSent") {
      const u = new URL(m.params.request.url);
      requests.set(m.params.requestId, {
        path: u.pathname,
        method: m.params.request.method,
      });
    } else if (m.method === "Network.loadingFailed") {
      const r=requests.get(m.params.requestId);if(r?.path.startsWith("/api/"))failedRequests.push({...r,error:m.params.errorText});
    } else if (m.method === "Network.responseReceived") {
      const r = requests.get(m.params.requestId);
      if (r) responses.push({ ...r, status: m.params.response.status });
    }
  });
  function call(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++seq;
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(Error("CDP timeout"));
      }, 12000);
      pending.set(id, { resolve, reject, timer });
      socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async function evaluate(expression) {
    const r = await call("Runtime.evaluate", {
      expression,
      returnByValue: true,
      awaitPromise: true,
    });
    if (r.exceptionDetails) throw Error("Browser evaluation failed");
    return r.result.value;
  }
  async function wait(expression) {
    for (let i = 0; i < 90; i++) {
      if (await evaluate(`Boolean(${expression})`)) return;
      await pause(500);
    }
    throw Error("Browser condition timeout");
  }
  debugCapture = async () => {
    await evaluate(
      `document.querySelectorAll('.token-secret').forEach(e=>e.textContent='[hidden]')`,
    );
    const shot = await call("Page.captureScreenshot", {
      format: "png",
      captureBeyondViewport: false,
    });
    await writeFile(config.failureScreenshot, Buffer.from(shot.data, "base64"));
    await writeFile(config.failureScreenshot+".network.json",JSON.stringify({responses:responses.slice(-40),failedRequests},null,2));
    await writeFile(
      config.failureScreenshot + ".txt",
      await evaluate(`document.body.innerText`),
    );
  };
  const pane = (id) => `document.querySelector('[data-page="${id}"]')`;
  async function point(expression) {
    const r = await evaluate(
      `(()=>{const e=${expression};if(!e||e.disabled)throw Error('Unavailable control');e.scrollIntoView({block:'center'});const r=e.getBoundingClientRect();if(!r.width||!r.height||getComputedStyle(e).visibility==='hidden')throw Error('Invisible control');return {x:r.x+r.width/2,y:r.y+r.height/2};})()`,
    );
    await call("Input.dispatchMouseEvent", {
      type: "mousePressed",
      ...r,
      button: "left",
      clickCount: 1,
    });
    await call("Input.dispatchMouseEvent", {
      type: "mouseReleased",
      ...r,
      button: "left",
      clickCount: 1,
    });
    await pause(160);
  }
  async function button(root, text) {
    await wait(
      `(()=>{const b=[...(${root}).querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)});return !!b&&!b.disabled&&b.getClientRects().length>0;})()`,
    );
    await point(
      `[...(${root}).querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)})`,
    );
    clicks.push(text);
  }
  async function field(expression, value, type = "HTMLInputElement") {
    await evaluate(
      `(()=>{const e=${expression};Object.getOwnPropertyDescriptor(${type}.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));})()`,
    );
  }
  async function nav(id) {
    await point(`document.querySelector('#app-navigation a[href="#${id}"]')`);
    await wait(
      `document.querySelector('#app-navigation a[aria-current="page"]')?.getAttribute('href')==='#${id}'`,
    );
    clicks.push("Навігація: " + id);
  }
  async function ready(id) {
    await wait(`!${pane(id)}.querySelector('[aria-busy="true"]')`);
  }
  async function editor(manifest) {
    await field(
      `${pane("configuration")}.querySelector('textarea.manifest-editor')`,
      typeof manifest === "string"
        ? manifest
        : JSON.stringify(manifest, null, 2),
      "HTMLTextAreaElement",
    );
  }
  async function create(manifest) {
    await ready("configuration");
    await field(
      `${pane("configuration")}.querySelector('select')`,
      manifest.kind,
      "HTMLSelectElement",
    );
    await editor(manifest);
    await button(pane("configuration"), "Перевірити");
    await ready("configuration");
    assert.equal(
      await evaluate(
        `${pane("configuration")}.querySelector('[role="alert"]')?.textContent??''`,
      ),
      "",
    );
    await button(pane("configuration"), "Створити новий ресурс");
    await wait(
      `${pane("configuration")}.querySelector('pre.config-result')?.textContent.includes('Збережено')`,
    );
  }
  const cases = {};
  const inventories = {};
  await call("Page.enable");
  await call("Runtime.enable");
  await call("Network.enable");
  await call("Browser.setDownloadBehavior", {
    behavior: "allow",
    downloadPath: config.downloadDir,
    eventsEnabled: true,
  });
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 1050,
    deviceScaleFactor: 1,
    mobile: false,
  });
  stage = "login-and-local-font";
  await call("Page.navigate", { url: config.origin });
  await wait(
    `document.querySelector('input[autocomplete="username"]')&&!document.querySelector('button[type="submit"]:disabled')`,
  );
  await wait(
    `!![...document.querySelectorAll('button')].find(b=>b.textContent==='Увійти'&&!b.disabled)`,
  );
  await field(
    `document.querySelector('input[autocomplete="username"]')`,
    config.username,
  );
  await field(
    `document.querySelector('input[autocomplete="current-password"]')`,
    config.password,
  );
  config.password = "";
  await button("document", "Увійти");
  await wait(
    `${pane("overview")}?.querySelector('.metric-card strong')?.textContent!=='—'&&!!document.querySelector('.sidebar')`,
  );
  await evaluate("document.fonts.ready");
  assert.equal(await evaluate('document.fonts.check("14px Manrope")'), true);
  cases.localFont = true;
  await button(pane("overview"), "Оновити огляд");
  await ready("overview");
  await call("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: false,
  }).then(async (r) =>
    writeFile(config.screenshot, Buffer.from(r.data, "base64")),
  );
  stage = "navigation-and-disabled-controls";
  const ids = await evaluate(
    `[...document.querySelectorAll('#app-navigation nav a')].map(a=>a.getAttribute('href').slice(1))`,
  );
  assert.equal(ids.length, 13);
  for (const id of ids) {
    await nav(id);
    await ready(id);
    inventories[id] = await evaluate(
      `[...${pane(id)}.querySelectorAll('button')].filter(e=>e.getClientRects().length).map(e=>({label:e.textContent.trim(),disabled:e.disabled}))`,
    );
  }
  await nav("monitor");
  await wait(
    `${pane("monitor")}.querySelector('.badge')?.textContent==='Працює'`,
  );
  await point(`${pane("monitor")}.querySelector('tbody details summary')`);
  await button(pane("monitor"), "Відкрити доказ");
  await wait(
    `${pane("monitor")}.querySelector('aside')?.textContent.includes('completed')`,
  );
  await button(pane("monitor"), "Закрити доказ");
  await nav("agents");
  await button(pane("agents"), "Оновити агентів");
  await wait(`${pane("agents")}.querySelectorAll('tbody tr').length===2`);
  await nav("history");
  await ready("history");
  await button(pane("history"), "Оновити історію");
  await ready("history");
  await button(pane("history"), "Відкрити доказ");
  await wait(
    `${pane("history")}.querySelector('aside')?.textContent.includes('completed')`,
  );
  await button(pane("history"), "Закрити доказ");
  const older = await evaluate(
    `[...${pane("history")}.querySelectorAll('button')].some(b=>b.textContent.trim()==='Старіші записи'&&!b.disabled)`,
  );
  if (older) {
    await button(pane("history"), "Старіші записи");
    await ready("history");
    await button(pane("history"), "До новіших");
    await ready("history");
    cases.historyPagination = true;
  }
  cases.navigationAndEvidence = true;
  stage = "configuration-draft-validation-crud";
  await nav("configuration");
  await ready("configuration");
  const original = await evaluate(
    `${pane("configuration")}.querySelector('textarea').value`,
  );
  await editor(original + "\n ");
  await nav("agents");
  await nav("configuration");
  assert.equal(
    await evaluate(`${pane("configuration")}.querySelector('textarea').value`),
    original + "\n ",
  );
  await field(
    `${pane("configuration")}.querySelector('select')`,
    "Target",
    "HTMLSelectElement",
  );
  await pause(200);
  assert.equal(
    await evaluate(`${pane("configuration")}.querySelector('select').value`),
    "Monitor",
  );
  await button(pane("configuration"), "Перевірити");
  await ready("configuration");
  await button(pane("configuration"), "Показати план");
  await ready("configuration");
  await button(pane("configuration"), "Зберегти поточний");
  await wait(`!document.querySelector('.draft-dot')`);
  await create({
    apiVersion: "netmonorc.io/v1alpha1",
    kind: "Target",
    metadata: { name: "design-target" },
    spec: { type: "logical" },
  });
  await button(pane("configuration"), "Оновити каталог Target");
  await ready("configuration");
  await button(pane("configuration"), "design-target");
  await ready("configuration");
  await button(pane("configuration"), "Видалити ресурс");
  await pause(200);
  assert.equal(
    await evaluate(`${pane("configuration")}.querySelector('input').value`),
    "design-target",
  );
  dialogAccept = true;
  await button(pane("configuration"), "Видалити ресурс");
  await wait(
    `${pane("configuration")}.querySelector('pre')?.textContent.includes('Ресурс видалено')`,
  );
  await editor("kind: Monitor\nwrong: field");
  await button(pane("configuration"), "Перевірити");
  await wait(`!!${pane("configuration")}.querySelector('[role="alert"]')`);
  dialogAccept = true;
  await field(
    `${pane("configuration")}.querySelector('select')`,
    "Monitor",
    "HTMLSelectElement",
  );
  await field(
    `${pane("configuration")}.querySelector('input')`,
    "research-web",
  );
  await button(pane("configuration"), "Завантажити");
  await ready("configuration");
  cases.draftRecoveryAndCRUD = true;
  stage = "monitor-switch";
  await create({
    apiVersion: "netmonorc.io/v1alpha1",
    kind: "Monitor",
    metadata: { name: "design-second" },
    spec: {
      target: { address: "198.18.0.80" },
      checks: [
        {
          id: "http",
          type: "http",
          runner: "native-http",
          interval: "10s",
          timeout: "2s",
          config: { url: "http://198.18.0.80:8080/", expectedStatus: [200] },
        },
      ],
      assessment: {
        requiredZones: ["office", "server"],
        checks: [{ id: "http", weight: 1, required: true }],
      },
      diagnostics: { enabled: false },
    },
  });
  await field(
    `document.querySelector('.monitor-toolbar input')`,
    "design-second",
  );
  await button(
    `document.querySelector('.monitor-toolbar')`,
    "Відкрити Monitor",
  );
  await nav("monitor");
  await wait(
    `${pane("monitor")}.querySelector('h2')?.textContent==='design-second'&&${pane("monitor")}.querySelector('.badge')?.textContent==='Працює'`,
  );
  await field(
    `document.querySelector('.monitor-toolbar input')`,
    "research-web",
  );
  await button(
    `document.querySelector('.monitor-toolbar')`,
    "Відкрити Monitor",
  );
  await nav("configuration");
  await ready("configuration");
  await field(
    `${pane("configuration")}.querySelector('input')`,
    "design-second",
  );
  await button(pane("configuration"), "Завантажити");
  await ready("configuration");
  dialogAccept = true;
  await button(pane("configuration"), "Видалити ресурс");
  await ready("configuration");
  await field(
    `${pane("configuration")}.querySelector('input')`,
    "research-web",
  );
  await button(pane("configuration"), "Завантажити");
  await ready("configuration");
  cases.monitorSwitch = true;

  stage = "batch-apply";
  await point(`${pane("configuration")}.querySelector('details>summary')`);
  const batch = `${pane("configuration")}.querySelector('details')`;
  await button(batch, "Підготувати пакет із відкритого маніфесту");
  await wait(`!!${batch}.querySelector('textarea').value`);
  await button(batch, "Підготувати пакет");
  await ready("configuration");
  await button(batch, "Перевірити пакет");
  await ready("configuration");
  await button(batch, "Показати план пакета");
  await ready("configuration");
  await button(batch, "Застосувати пакет");
  await wait(`${batch}.textContent.includes('Пакет збережено атомарно')`);
  await button(batch, "Оновити список операцій");
  await wait(`${batch}.querySelector('tbody tr')`);
  cases.batchApply = true;
  stage = "dependency-group-fallback-controls";
  await nav("dependencies");
  await button(pane("dependencies"), "Оновити граф");
  await ready("dependencies");
  await button(pane("dependencies"), "Контекст Monitor research-web");
  await ready("dependencies");
  await button(pane("dependencies"), "Скинути фільтри");
  await wait(`${pane("dependencies")}.querySelector('tbody button')`);
  await point(`${pane("dependencies")}.querySelector('tbody button')`);
  await wait(`${pane("dependencies")}.querySelector('.dependency-detail')`);
  await nav("groups");
  await button(pane("groups"), "Оновити групи");
  await ready("groups");
  await button(pane("groups"), "design-group");
  await ready("groups");
  await button(pane("groups"), "Оновити оцінку");
  await ready("groups");
  await nav("fallbacks");
  await button(pane("fallbacks"), "Оновити резервні спроби");
  await ready("fallbacks");
  cases.relatedResources = true;
  stage = "diagnostic-detail-and-downloads";
  await nav("diagnostics");
  await field(
    `${pane("diagnostics")}.querySelector("select")`,
    "completed",
    "HTMLSelectElement",
  );
  await button(pane("diagnostics"), "Оновити список");
  await wait(`${pane("diagnostics")}.querySelector('tbody button')`);
  await button(pane("diagnostics"), "Відкрити");
  await ready("diagnostics");
  await button(pane("diagnostics"), "Оновити деталі");
  await ready("diagnostics");
  await button(pane("diagnostics"), "Завантажити сесію JSON");
  await button(pane("diagnostics"), "Завантажити пакет доказів");
  await ready("diagnostics");
  await button(pane("diagnostics"), "Відкрити доказ");
  await wait(
    `${pane("diagnostics")}.querySelector('aside')?.textContent.includes('completed')`,
  );
  await button(pane("diagnostics"), "Закрити доказ");
  await button(pane("diagnostics"), "Новий запит");
  cases.diagnosticEvidence = true;
  stage = "manual-diagnostic-cancel";
  await field(
    `${pane("diagnostics")}.querySelector('input')`,
    "design-slow-v2",
  );
  await button(pane("diagnostics"), "Запустити / відновити запит");
  await wait(
    `[...${pane("diagnostics")}.querySelectorAll('button')].some(b=>b.textContent.trim()==='Скасувати сесію'&&!b.disabled)`,
  );
  dialogAccept = true;
  await button(pane("diagnostics"), "Скасувати сесію");
  await wait(`${pane("diagnostics")}.textContent.includes('cancelled')`);
  cases.manualStartAndCancel = true;

  stage = "alerts-acknowledgement-webhook";
  await nav("alerts");
  await button(pane("alerts"), "Оновити алерти");
  await wait(`${pane("alerts")}.querySelector('tbody tr')`);
  await point(
    `[...${pane("alerts")}.querySelectorAll('tbody tr')].find(r=>r.textContent.includes('design-alert')).querySelector('button')`,
  );
  await ready("alerts");
  await button(pane("alerts"), "Оновити деталі");
  await ready("alerts");
  await button(pane("alerts"), "Підтвердити отримання алерта");
  await ready("alerts");
  await button(pane("alerts"), "Надіслати тест webhook");
  await ready("alerts");
  await button(pane("alerts"), "Оновити статус тесту");
  await ready("alerts");
  await button(pane("alerts"), "Оновити доставки");
  await ready("alerts");
  cases.alertsAndWebhook = true;
  stage = "users-and-tokens";
  await nav("users");
  await ready("users");
  await point(`${pane("users")}.querySelector('summary')`);
  const userForm = `${pane("users")}.querySelector('details form')`;
  await field(`${userForm}.querySelector('input')`, config.viewerName);
  await field(
    `${userForm}.querySelector('input[type="password"]')`,
    config.viewerPassword,
  );
  await button(pane("users"), "Створити користувача");
  await wait(
    `${pane("users")}.textContent.includes(${JSON.stringify(config.viewerName)})`,
  );
  await point(
    `[...${pane("users")}.querySelectorAll('tbody tr')].find(r=>r.textContent.includes(${JSON.stringify(config.viewerName)})).querySelector('button')`,
  );
  await ready("users");
  await button(pane("users"), "Зберегти зміни");
  await ready("users");
  await button(pane("users"), "Закрити");
  await button(pane("users"), "Оновити список");
  await ready("users");
  await nav("tokens");
  await ready("tokens");
  await field(`${pane("tokens")}.querySelector('input')`, config.tokenName);
  await button(pane("tokens"), "Створити токен");
  await wait(`${pane("tokens")}.querySelector('.token-secret')`);
  await button(pane("tokens"), "Приховати секрет");
  await wait(`!${pane("tokens")}.querySelector('.token-secret')`);
  const revoke = `[...${pane("tokens")}.querySelectorAll('tbody tr')].find(r=>r.textContent.includes(${JSON.stringify(config.tokenName)})).querySelector('button')`;
  await point(revoke);
  dialogAccept = true;
  await point(revoke);
  await ready("tokens");
  cases.usersAndTokenLifecycle = true;
  stage = "password-change-session-restore";
  await nav("account");
  await field(
    `${pane("account")}.querySelector('input[autocomplete="current-password"]')`,
    config.originalPassword,
  );
  config.originalPassword = "";
  await field(
    `${pane("account")}.querySelector('input[autocomplete="new-password"]')`,
    config.newPassword,
  );
  await button(pane("account"), "Зберегти пароль");
  await wait(`${pane("account")}.textContent.includes('Пароль змінено')`);
  const previousLoads = pageLoads;
  await call("Page.reload");
  for (let i = 0; i < 80 && pageLoads <= previousLoads; i++) await pause(100);
  assert.ok(pageLoads > previousLoads);
  await wait(`!!document.querySelector('.sidebar')`);
  await nav("overview");
  await ready("overview");
  cases.passwordAndSessionRestore = true;
  stage = "mobile-keyboard-layout";
  await call("Emulation.setDeviceMetricsOverride", {
    width: 390,
    height: 844,
    deviceScaleFactor: 1,
    mobile: true,
  });
  await wait(
    `getComputedStyle(document.querySelector('.sidebar')).display==='none'`,
  );
  await point(
    `document.querySelector('button[aria-controls="app-navigation"]')`,
  );
  await wait(
    `getComputedStyle(document.querySelector('.sidebar')).display!=='none'`,
  );
  await call("Input.dispatchKeyEvent", {
    type: "keyDown",
    key: "Escape",
    code: "Escape",
  });
  await call("Input.dispatchKeyEvent", {
    type: "keyUp",
    key: "Escape",
    code: "Escape",
  });
  assert.equal(
    await evaluate(
      `document.activeElement===document.querySelector('button[aria-controls="app-navigation"]')`,
    ),
    true,
  );
  assert.equal(
    await evaluate(`document.documentElement.scrollWidth<=innerWidth`),
    true,
  );
  await call("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: false,
  }).then(async (r) =>
    writeFile(config.mobileScreenshot, Buffer.from(r.data, "base64")),
  );
  await point(
    `document.querySelector('button[aria-controls="app-navigation"]')`,
  );
  await nav("monitor");
  await wait(`${pane("monitor")}.querySelector('.badge')`);
  assert.equal(
    await evaluate(`document.documentElement.scrollWidth<=innerWidth`),
    true,
  );
  cases.mobileAndEscape = true;
  stage = "viewer-permissions";
  await call("Emulation.setDeviceMetricsOverride", {
    width: 1440,
    height: 1050,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await point(
    `document.querySelector('button[aria-label="Вийти з облікового запису"]')`,
  );
  await wait(`document.querySelector('input[autocomplete="username"]')`);
  await field(
    `document.querySelector('input[autocomplete="username"]')`,
    config.viewerName,
  );
  await field(
    `document.querySelector('input[autocomplete="current-password"]')`,
    config.viewerPassword,
  );
  config.viewerPassword = "";
  await button("document", "Увійти");
  await wait(`!!document.querySelector('.sidebar')`);
  assert.equal(
    await evaluate(
      `!!document.querySelector('#app-navigation a[href="#users"]')`,
    ),
    false,
  );
  await nav("configuration");
  await ready("configuration");
  assert.equal(
    await evaluate(
      `${pane("configuration")}.querySelector('textarea').readOnly`,
    ),
    true,
  );
  assert.equal(
    await evaluate(
      `[...${pane("configuration")}.querySelectorAll('button')].some(b=>b.textContent.trim()==='Зберегти поточний')`,
    ),
    false,
  );
  cases.viewerPermissions = true;
  await point(
    `document.querySelector('button[aria-label="Вийти з облікового запису"]')`,
  );
  await wait(`!!document.querySelector('input[autocomplete="username"]')`);
  assert.equal(errors.length, 0);
  await writeFile(
    config.output,
    JSON.stringify(
      {
        status: "passed",
        cases,
        clickedControls: clicks,
        initialButtonInventory: inventories,
        dialogs: dialogs.length,
        downloads,
        networkResponses: responses.filter((r) => r.path.startsWith("/api/")),
        failedRequests,
        runtimeExceptions: errors.length,
        browserInteractionTested: true,
        credentialsOmitted: true,
        limits: [
          "No real multi-page dataset for every pagination cursor",
          "Fallback evidence requires a separate executor-failure scenario",
        ],
      },
      null,
      2,
    ) + "\n",
    { mode: 0o600 },
  );
  console.log(
    "PASS: real browser admin navigation, CRUD, validation, batch apply, evidence, accounts, alerts and mobile/viewer controls",
  );
} catch (e) {
  console.error(
    "Browser acceptance failed at stage:",
    stage,
    "reason:",
    e.message,
  );
  try {
    await debugCapture?.();
  } catch {}
  process.exitCode = 1;
} finally {
  if (socket) socket.close();
  if (chrome) {
    chrome.kill("SIGTERM");
    await once(chrome, "exit").catch(() => {});
  }
  await rm(browserHome, { recursive: true, force: true });
}
