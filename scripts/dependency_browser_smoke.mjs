// Headless Chromium UI use cases against production assets and a local API fixture.
// This does not claim a live Controller/authentication integration test.
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm, mkdir, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { once } from 'node:events';
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const output = resolve(process.argv[2] ?? resolve(root, '.local/dependency-browser-r28.json'));
const screenshot = resolve(process.argv[3] ?? resolve(root, '.local/dependency-browser-r28.png'));
const pause = ms => new Promise(r => setTimeout(r, ms));
const ref = (kind, name) => ({ kind, name });
const rows = [
  ['web-needs-dns', ref('Monitor', 'web'), ref('Target', 'dns'), 'requires', 'hard', 'Сервіс використовує resolver'],
  ['dns-backup', ref('Target', 'dns'), ref('Target', 'backup-dns'), 'relatedTo', 'soft', '<img src=x onerror="window.fixtureInjection=true">'],
  ['backup-dns-return', ref('Target', 'backup-dns'), ref('Target', 'dns'), 'relatedTo', 'soft', 'Окремий зворотний зв’язок'],
  ['web-host', ref('Monitor', 'web'), ref('Monitor', 'host'), 'hostedOn', 'soft', 'Контекст розміщення'],
];
function resource([name, from, to, relation, strength, description]) {
  return { manifest: { apiVersion: 'netmonorc.io/v1alpha1', kind: 'Dependency', metadata: { name }, spec: { from, to, relation, strength, description } }, uid: 'fixture-'+name, generation: 1, resourceVersion: 'fixture:1', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
}
let failNext = false, empty = false, scoped = true, mutations = 0;
const cursors = [], errors = [], paths = [];
const server = createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  paths.push(url.pathname);
  const send = (status, body, headers={}) => { res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', ...headers }); res.end(JSON.stringify(body)); };
  if (req.method !== 'GET') { mutations++; send(405, {message:'Read-only fixture'}); return; }
  if (url.pathname === '/api/v1/auth/me') return send(200, { userId:'fixture-viewer', username:'fixture-viewer', role:'viewer', csrfToken:'fixture-only', expiresAt:new Date(Date.now()+3600000).toISOString(), scopes:scoped?['resources:read']:['agents:read'] });
  if (url.pathname === '/api/v1/resources/Dependency') {
    cursors.push(url.searchParams.get('cursor'));
    if (failNext) { failNext = false; return send(503, {message:'Controlled fixture error'}); }
    if (empty) return send(200, {items:[],nextCursor:null});
    return send(200, url.searchParams.has('cursor') ? { items:[resource(['web-storage',ref('Monitor','web'),ref('Target','storage'),'requires','soft','Друга сторінка'])], nextCursor:null } : { items:rows.map(resource), nextCursor:'fixture-page-2' });
  }
  if (url.pathname === '/api/v1/resources/Monitor/research-web') return send(200, {manifest:{apiVersion:'netmonorc.io/v1alpha1',kind:'Monitor',metadata:{name:'research-web'},spec:{target:{address:'192.0.2.1'}}},uid:'fixture-monitor',generation:1,resourceVersion:'fixture:1'}, {ETag:'"fixture:1"'});
  if (url.pathname.endsWith('/assessment')) return send(200, { monitorId:'research-web',state:'unknown',coverageState:'unknown',zoneStates:[],reasonCodes:[],evidenceIds:[],evaluatedAt:new Date().toISOString(),revision:'fixture-only' });
  if (url.pathname.startsWith('/api/')) return send(200, {items:[],nextCursor:null});
  let path;
  if (url.pathname === '/') path = resolve(root, 'web/dist/index.html');
  else if (/^\/assets\/[a-zA-Z0-9_.-]+\.(js|css)$/.test(url.pathname)) path=resolve(root,'web/dist'+url.pathname);
  else return send(404, {message:'Not found'});
  try { const data=await readFile(path);res.writeHead(200,{'Content-Type':path.endsWith('.js')?'text/javascript':path.endsWith('.css')?'text/css':'text/html'});res.end(data); }
  catch { send(404,{message:'Asset unavailable; run npm run build --prefix web'}); }
});
await new Promise(r => server.listen(0, '127.0.0.1', r));
const origin='http://127.0.0.1:'+server.address().port;
const profile=await mkdtemp(resolve(tmpdir(),'nmo-dependency-browser-'));
let chrome, socket;
try {
  chrome=spawn(process.env.NMO_CHROMIUM ?? '/usr/bin/chromium', ['--headless=new','--no-sandbox','--disable-dev-shm-usage','--disable-background-networking','--disable-component-update','--disable-extensions','--no-first-run','--no-default-browser-check','--remote-debugging-address=127.0.0.1','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'], {stdio:['ignore','ignore','pipe']});
  let stderr='';chrome.stderr.on('data',b=>{stderr=(stderr+b).slice(-3000);});
  let port;
  for(let i=0;i<100;i++){try{port=Number((await readFile(resolve(profile,'DevToolsActivePort'),'utf8')).split('\n')[0]);break;}catch{if(chrome.exitCode!==null)throw Error('Chromium exited: '+stderr);await pause(100);}}
  assert.ok(port,'Chromium startup timed out');
  const targets=await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const target=targets.find(t=>t.type==='page');assert.ok(target);
  socket=new WebSocket(target.webSocketDebuggerUrl);await once(socket,'open');
  let seq=0;const pending=new Map();
  socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);clearTimeout(p.timer);m.error?p.reject(Error(m.error.message)):p.resolve(m.result);}}else if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description??m.params.exceptionDetails.text);});
  function call(method,params={}){return new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout '+method));},12000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));});}
  async function evaluate(expression){const result=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(result.exceptionDetails)throw Error(result.exceptionDetails.text);return result.result.value;}
  async function wait(expression){for(let i=0;i<100;i++){if(await evaluate(expression))return;await pause(100);}throw Error('UI condition timed out: '+expression+' '+JSON.stringify({exceptions:errors,paths,body:await evaluate('document.body?.innerText?.slice(0,4000)')}));}
  const section="document.querySelector('section[aria-labelledby=\"dependencies-heading\"]')";
  const button=text=>`[...${section}.querySelectorAll('button')].find(b=>b.textContent===${JSON.stringify(text)})`;
  async function click(text){await evaluate(`${button(text)}.click()`);}
  await call('Page.enable');await call('Runtime.enable');
  await call('Emulation.setDeviceMetricsOverride',{width:1280,height:1000,deviceScaleFactor:1,mobile:false});
  await call('Page.navigate',{url:origin});
  await wait(`${section}?.querySelectorAll('tbody tr').length===4`);
  await pause(2200); // App updates its clock every second: sibling keys must stay unique.
  assert.equal(await evaluate(`document.querySelectorAll('section[aria-labelledby="dependencies-heading"]').length`),1);
  assert.equal(await evaluate(`[...document.querySelectorAll('h2')].filter(h=>h.textContent==='Конфігурація моніторингу').length`),1);
  assert.equal(await evaluate(`${section}.querySelectorAll('svg .dependency-edge').length`),4);
  assert.equal(await evaluate(`${section}.querySelectorAll('svg [marker-end]').length`),4);
  // Real DOM keyboard event selects a node and shows both incoming and outgoing edges.
  await evaluate(`${section}.querySelector('[aria-label="Вузол Target/dns"]').focus()`);
  await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
  await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
  await wait(`${section}.querySelector('select').value==='Target/dns'&&${section}.querySelectorAll('tbody tr').length===3`);
  await pause(2200); // App's clock must not duplicate sibling panels or reset selection.
  assert.equal(await evaluate(`document.querySelectorAll('section[aria-labelledby="dependencies-heading"]').length`),1);
  assert.equal(await evaluate(`[...document.querySelectorAll('h2')].filter(h=>h.textContent==='Конфігурація моніторингу').length`),1);
  assert.equal(await evaluate(`${section}.querySelector('select').value`),'Target/dns');
  await click('Скинути фільтри');
  await click('dns-backup');
  await wait(`${section}.querySelector('[aria-label="Деталі зв’язку"]')?.textContent.includes('relatedTo')`);
  assert.equal(await evaluate(`${section}.querySelector('[aria-label="Деталі зв’язку"] img')===null&&window.fixtureInjection===undefined`),true);
  await evaluate(`(()=>{const input=${section}.querySelector('input');Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'hostedOn');input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  await wait(`${section}.querySelectorAll('tbody tr').length===1&&${section}.textContent.includes('web-host')`);
  await click('Скинути фільтри');await wait(`${section}.querySelectorAll('tbody tr').length===4`);
  // Capture the actual rendered panel. Fixture text contains no credentials.
  await evaluate(`${section}.scrollIntoView({block:'start'})`);
  const clip=await evaluate(`(()=>{const r=${section}.getBoundingClientRect();return {x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1};})()`);
  await mkdir(dirname(screenshot),{recursive:true});
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});await writeFile(screenshot,Buffer.from(shot.data,'base64'));
  await click('Наступна сторінка зв’язків');await wait(`${section}.querySelectorAll('tbody tr').length===1&&${section}.textContent.includes('web-storage')`);
  assert.ok(cursors.includes('fixture-page-2'));
  assert.equal(await evaluate(`${button('Наступна сторінка зв’язків')}.disabled`),true);
  failNext=true;await click('Оновити граф');await wait(`${section}.querySelector('[role="alert"]')!==null`);
  assert.equal(await evaluate(`${section}.querySelector('svg')===null`),true);
  await click('Оновити граф');await wait(`${section}.querySelectorAll('tbody tr').length===4`);
  empty=true;await click('Оновити граф');await wait(`${section}.textContent.includes('У каталозі немає зв’язків')`);
  assert.equal(await evaluate(`${section}.querySelector('svg')===null`),true);
  scoped=false;await call('Page.reload');await wait(`document.body.textContent.includes('fixture-viewer')&&!${section}`);
  assert.equal(mutations,0);assert.deepEqual(errors,[]);
  const result={status:'passed',browser:'Chromium headless',backend:'local read-only API fixture; production React assets',browserInteractionTested:true,liveControllerIntegrationTested:false,stablePanelsAcrossClockUpdates:true,stableSectionsAcrossClockUpdates:true,directedArrows:true,keyboardNodeSelection:true,incomingAndOutgoingLinks:true,relationStrengthDetails:true,descriptionEscaped:true,currentPageSearch:true,opaqueCursorPagination:true,errorClearsStaleGraph:true,manualRetryRecovers:true,emptyCatalog:true,scopeVisibilityOnly:true,mutationRequests:mutations,consoleExceptions:errors.length,screenshotContainsFixtureDataOnly:true};
  await mkdir(dirname(output),{recursive:true});await writeFile(output,JSON.stringify(result,null,2)+'\n',{flag:'wx',mode:0o600});
  console.log('PASS: Chromium dependency graph user flows; fixture API only, no real credentials');
} finally {
  if(socket)socket.close();
  if(chrome&&chrome.exitCode===null){chrome.kill('SIGTERM');await Promise.race([once(chrome,'exit'),pause(3000)]);if(chrome.exitCode===null){chrome.kill('SIGKILL');await once(chrome,'exit');}}
  await new Promise(r=>server.close(r));await rm(profile,{recursive:true,force:true});
}
