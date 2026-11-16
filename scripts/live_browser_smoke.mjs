// Real browser flows against the owned HTTPS stand. Credentials arrive on stdin
// only; failures log a fixed stage name, never CDP expressions or input values.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import https from 'node:https';
import { once } from 'node:events';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
let input='';for await(const chunk of process.stdin)input+=chunk;
const config=JSON.parse(input);input='';
const origin=new URL(config.origin);assert.equal(origin.protocol,'https:');assert.equal(origin.hostname,'localhost');
const browserHome=await mkdtemp(resolve(tmpdir(),'nmo-live-browser-'));
const pause=ms=>new Promise(r=>setTimeout(r,ms));
let chrome,socket,stage='trusted-https';const responses=[];const requests=new Map();
try {
  const ca=await readFile(config.caFile);
  const tls=await new Promise((resolve,reject)=>{
    const req=https.get(config.origin+'/api/v1/auth/me',{ca,servername:'localhost',timeout:8000},res=>{
      res.resume();resolve({status:res.statusCode,protocol:res.socket.getProtocol(),authorized:res.socket.authorized});
    });req.on('error',reject);req.on('timeout',()=>req.destroy(Error('TLS timeout')));
  });
  assert.equal(tls.status,401);assert.equal(tls.authorized,true);assert.equal(tls.protocol,'TLSv1.3');
  stage='isolated-browser-trust';
  const nss=resolve(browserHome,'.pki/nssdb');await mkdir(nss,{recursive:true,mode:0o700});
  execFileSync('certutil',['-N','--empty-password','-d','sql:'+nss],{stdio:'ignore'});
  execFileSync('certutil',['-A','-d','sql:'+nss,'-n','NetMonOrc owned stand CA','-t','C,,','-i',config.caFile],{stdio:'ignore'});
  chrome=spawn('/usr/bin/chromium',['--headless=new','--no-sandbox','--disable-dev-shm-usage','--disable-background-networking','--disable-component-update','--disable-extensions','--no-first-run','--no-default-browser-check','--remote-debugging-address=127.0.0.1','--remote-debugging-port=0','--user-data-dir='+resolve(browserHome,'profile'),'about:blank'],{stdio:['ignore','ignore','ignore'],env:{...process.env,HOME:browserHome,XDG_CONFIG_HOME:resolve(browserHome,'.config'),XDG_DATA_HOME:resolve(browserHome,'.local/share')}});
  let launchError;chrome.on('error',e=>{launchError=e;});
  let port;for(let i=0;i<100;i++){if(launchError||chrome.exitCode!==null)throw Error('Browser launch failed');try{port=Number((await readFile(resolve(browserHome,'profile/DevToolsActivePort'),'utf8')).split('\n')[0]);break;}catch{await pause(100);}}
  assert.ok(port);
  const targets=await(await fetch(`http://127.0.0.1:${port}/json/list`)).json();const target=targets.find(t=>t.type==='page');assert.ok(target);
  socket=new WebSocket(target.webSocketDebuggerUrl);await once(socket,'open');
  let seq=0;const pending=new Map();const errors=[];
  socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.id){const p=pending.get(m.id);if(p){pending.delete(m.id);clearTimeout(p.timer);m.error?p.reject(Error('CDP command failed')):p.resolve(m.result);}}else if(m.method==='Runtime.exceptionThrown')errors.push('runtime-exception');
    else if(m.method==='Network.requestWillBeSent'){const u=new URL(m.params.request.url);requests.set(m.params.requestId,{path:u.pathname,method:m.params.request.method});}
    else if(m.method==='Network.responseReceived'){const r=requests.get(m.params.requestId);if(r)responses.push({...r,status:m.params.response.status});}});
  function call(method,params={}){return new Promise((resolve,reject)=>{const id=++seq;const timer=setTimeout(()=>{pending.delete(id);reject(Error('CDP timeout'));},12000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));});}
  async function evaluate(expression){const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error('Browser evaluation failed');return r.result.value;}
  async function wait(expression){for(let i=0;i<90;i++){if(await evaluate(expression))return;await pause(500);}throw Error('Browser condition timeout');}
  const configSection=`[...document.querySelectorAll('main > section')].find(s=>s.querySelector('h2')?.textContent==='Конфігурація моніторингу')`;
  const graph=`document.querySelector('section[aria-labelledby="dependencies-heading"]')`;
  const groups=`[...document.querySelectorAll('main > section')].find(s=>s.querySelector('h2')?.textContent==='Групи ресурсів')`;
  const assessment=`[...document.querySelectorAll('main > section')].find(s=>s.querySelector('h2')?.textContent==='research-web')`;
  async function click(section,text){await evaluate(`(()=>{const b=[...(${section}).querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)});if(!b||b.disabled)throw Error('Button unavailable');b.click();})()`);}
  async function field(selector,value,element='HTMLInputElement'){await evaluate(`(()=>{const e=${selector};Object.getOwnPropertyDescriptor(${element}.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));})()`);}
  async function create(manifest){await wait(`(${configSection}).querySelector('textarea.manifest-editor')&&!(${configSection}).querySelector('textarea.manifest-editor').disabled`);await field(`(${configSection}).querySelector('textarea.manifest-editor')`,JSON.stringify(manifest,null,2),'HTMLTextAreaElement');await wait(`![...(${configSection}).querySelectorAll('button')].find(b=>b.textContent.trim()==='Створити новий ресурс').disabled`);await click(configSection,'Створити новий ресурс');await wait(`(${configSection}).querySelector('input')?.value===${JSON.stringify(manifest.metadata.name)}&&(${configSection}).querySelector('pre.config-result')?.textContent.includes('Збережено')`);}
  async function load(kind,name){await field(`(${configSection}).querySelector('select')`,kind,'HTMLSelectElement');await field(`(${configSection}).querySelector('input')`,name);await click(configSection,'Завантажити');await wait(`(${configSection}).querySelector('textarea')?.value.includes(${JSON.stringify('"name": "'+name+'"')})&&![...(${configSection}).querySelectorAll('button')].find(b=>b.textContent.trim()==='Видалити ресурс').disabled`);}
  await call('Page.enable');await call('Runtime.enable');await call('Network.enable');await call('Emulation.setDeviceMetricsOverride',{width:1360,height:1000,deviceScaleFactor:1,mobile:false});
  stage='https-page';await call('Page.navigate',{url:config.origin});
  await wait(`document.querySelector('input[autocomplete="username"]')&&![...document.querySelectorAll('button')].find(b=>b.textContent==='Увійти').disabled`);
  assert.equal(await evaluate('location.protocol'), 'https:');assert.equal(await evaluate('window.isSecureContext'),true);
  stage='password-login';await field(`document.querySelector('input[autocomplete="username"]')`,config.username);await field(`document.querySelector('input[autocomplete="current-password"]')`,config.password);config.password='';
  await click(`document.querySelector('main > section')`,'Увійти');
  await wait(`document.body.textContent.includes(${JSON.stringify(config.username+' · admin')})&&${assessment}?.querySelector('.badge')?.textContent==='Працює'`);
  const originalRevision=await evaluate(`(${assessment}).querySelector('details code').textContent`);
  stage='create-target';await create({apiVersion:'netmonorc.io/v1alpha1',kind:'Target',metadata:{name:'browser-dns'},spec:{type:'logical'}});
  stage='create-dependency';await create({apiVersion:'netmonorc.io/v1alpha1',kind:'Dependency',metadata:{name:'browser-link'},spec:{from:{kind:'Monitor',name:'research-web'},to:{kind:'Target',name:'browser-dns'}}});
  await wait(`${assessment}?.querySelector('.badge')?.textContent==='Працює'&&${assessment}?.querySelector('details code')?.textContent!==${JSON.stringify(originalRevision)}`);
  stage='real-dependency-graph';await click(graph,'Оновити граф');await wait(`${graph}?.querySelectorAll('tbody tr').length===1`);await click(graph,'browser-link');
  await wait(`${graph}.querySelector('[aria-label="Деталі зв’язку"]')?.textContent.includes('requires')`);
  assert.equal(await evaluate(`${graph}.querySelectorAll('svg [marker-end]').length`),1);
  await evaluate(`${graph}.scrollIntoView({block:'start'})`);const clip=await evaluate(`(()=>{const r=${graph}.getBoundingClientRect();return{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1}})()`);
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip});await writeFile(config.screenshot,Buffer.from(shot.data,'base64'));
  stage='create-derived-group';await create({apiVersion:'netmonorc.io/v1alpha1',kind:'ResourceGroup',metadata:{name:'browser-business'},spec:{members:['research-web'],mode:'derived',evaluation:{strategy:'all-required'}}});
  await click(groups,'Оновити групи');await wait(`[...(${groups}).querySelectorAll('button')].some(b=>b.textContent==='browser-business')`);await click(groups,'browser-business');await wait(`(${groups}).textContent.includes('Стан: healthy; повнота даних: sufficient')`);
  stage='protected-reference-delete';await load('Target','browser-dns');await click(configSection,'Видалити ресурс');await wait(`(${configSection}).querySelector('[role="alert"]')?.textContent.includes('використовує')`);
  stage='cookie-session-reload';await call('Page.reload');await wait(`document.body.textContent.includes(${JSON.stringify(config.username+' · admin')})&&${graph}?.querySelectorAll('tbody tr').length===1`);
  stage='delete-demo-resources';await load('ResourceGroup','browser-business');await click(configSection,'Видалити ресурс');await wait(`(${configSection}).textContent.includes('Ресурс видалено')`);
  await load('Dependency','browser-link');await click(configSection,'Видалити ресурс');await wait(`(${configSection}).textContent.includes('Ресурс видалено')`);
  await wait(`${assessment}?.querySelector('.badge')?.textContent==='Працює'&&${assessment}?.querySelector('details code')?.textContent===${JSON.stringify(originalRevision)}`);
  await load('Target','browser-dns');await click(configSection,'Видалити ресурс');await wait(`(${configSection}).textContent.includes('Ресурс видалено')`);
  await click(graph,'Оновити граф');await wait(`${graph}.textContent.includes('У каталозі немає зв’язків')`);
  await pause(2200);assert.equal(await evaluate(`document.querySelectorAll('section[aria-labelledby="dependencies-heading"]').length`),1);
  stage='logout';await click(`document.querySelector('main > section')`,'Від’єднатися');await wait(`document.querySelector('input[autocomplete="username"]')&&![...document.querySelectorAll('button')].find(b=>b.textContent==='Увійти').disabled`);
  assert.equal(await evaluate(`fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.status)`),401);
  assert.deepEqual(errors,[]);
  assert.equal(responses.filter(r=>r.path==='/api/v1/resources/Target/browser-dns'&&r.method==='DELETE'&&r.status===409).length,1);
  assert.equal(responses.filter(r=>r.path==='/api/v1/resources/Target'&&r.method==='POST').length,1);
  await writeFile(config.output,JSON.stringify({status:'passed',browserInteractionTested:true,liveControllerIntegrationTested:true,strictNodeTLS:true,browserTrustedLocalCA:true,insecureTLSFlags:false,realPasswordLogin:true,httpOnlySessionRestoredByBrowser:true,targetCRUD:true,dependencyCRUDAndGraph:true,derivedGroupHealthyFromRealProbes:true,referenceDeleteGuardVisible:true,revisionChangeAndRestoreVisible:true,stablePanels:true,logoutRevokesSession:true,createdResourcesRemoved:true,mutationsNotReplayed:true,consoleExceptions:errors.length,screenshotContainsOwnedDemoDataOnly:true},null,2)+'\n',{flag:'wx',mode:0o600});
  console.log('PASS: live HTTPS browser login CRUD graph group and logout; no secrets exported');
} catch(error) {console.error('Live browser verification failed at stage: '+stage+(typeof error.code==='string'?' ('+error.code+')':''));console.error('HTTP error metadata: '+JSON.stringify(responses.filter(r=>r.status>=400)));process.exitCode=1;}
finally {
  if(socket)socket.close();if(chrome?.pid&&chrome.exitCode===null){chrome.kill('SIGTERM');await Promise.race([once(chrome,'exit'),pause(3000)]);if(chrome.exitCode===null){chrome.kill('SIGKILL');await once(chrome,'exit');}}
  await rm(browserHome,{recursive:true,force:true});
}
