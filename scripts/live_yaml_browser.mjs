// Real browser flows against the owned HTTPS stand. Credentials arrive on stdin
// only; failures log a fixed stage name, never CDP expressions or input values.
import assert from 'node:assert/strict';
import {X509Certificate,createHash} from 'node:crypto';
import { spawn } from 'node:child_process';
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
let chrome,socket,debugEvaluate,stage='trusted-https';const responses=[];const requests=new Map();
try {
  const ca=await readFile(config.caFile);
  const tls=await new Promise((resolve,reject)=>{
    const req=https.get(config.origin+'/api/v1/auth/me',{ca,servername:'localhost',timeout:8000},res=>{
      res.resume();resolve({status:res.statusCode,protocol:res.socket.getProtocol(),authorized:res.socket.authorized});
    });req.on('error',reject);req.on('timeout',()=>req.destroy(Error('TLS timeout')));
  });
  assert.equal(tls.status,401);assert.equal(tls.authorized,true);assert.equal(tls.protocol,'TLSv1.3');
  stage='owned-server-pin';
  const certificate=new X509Certificate(await readFile(config.certFile));
  const pin=createHash('sha256').update(certificate.publicKey.export({type:'spki',format:'der'})).digest('base64');
  chrome=spawn('/usr/bin/chromium',['--headless=new','--no-sandbox','--disable-dev-shm-usage','--disable-background-networking','--disable-component-update','--disable-extensions','--ignore-certificate-errors-spki-list='+pin,'--no-first-run','--no-default-browser-check','--remote-debugging-address=127.0.0.1','--remote-debugging-port=0','--user-data-dir='+resolve(browserHome,'profile'),'about:blank'],{stdio:['ignore','ignore','ignore'],env:{...process.env,XDG_CONFIG_HOME:resolve(browserHome,'.config'),XDG_DATA_HOME:resolve(browserHome,'.local/share')}});
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
  debugEvaluate=evaluate;
  async function wait(expression){for(let i=0;i<90;i++){if(await evaluate(`(async()=>{try{return Boolean(await (${expression}))}catch{return false}})()`))return;await pause(500);}throw Error('Browser condition timeout');}
  const batch=`[...document.querySelectorAll('details')].find(s=>s.querySelector('summary')?.textContent==='Пакетне застосування й операції')`;
  async function click(section,text){await wait(`[...(${section}).querySelectorAll('button')].some(b=>b.textContent.trim()===${JSON.stringify(text)}&&!b.disabled)`);await evaluate(`(()=>{const b=[...(${section}).querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)});if(!b||b.disabled)throw Error('Button unavailable');b.click();})()`);}
  async function field(selector,value,element='HTMLInputElement'){await evaluate(`(()=>{const e=${selector};Object.getOwnPropertyDescriptor(${element}.prototype,'value').set.call(e,${JSON.stringify(value)});e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}));})()`);}
  await call('Page.enable');await call('Runtime.enable');await call('Network.enable');await call('Emulation.setDeviceMetricsOverride',{width:1360,height:1000,deviceScaleFactor:1,mobile:false});
  stage='login';await call('Page.navigate',{url:config.origin});
  await wait(`document.querySelector('input[autocomplete="username"]')&&![...document.querySelectorAll('button')].find(b=>b.textContent==='Увійти').disabled`);
  stage='login-fields';await field(`document.querySelector('input[autocomplete="username"]')`,config.username);await field(`document.querySelector('input[autocomplete="current-password"]')`,config.password);config.password='';
  stage='login-submit';await click(`document.querySelector('main > section')`,'Увійти');stage='login-batch';await wait(`${batch}?.querySelector('textarea')`);
  await evaluate(`(${batch}).querySelector('summary').click()`);
  await evaluate(`globalThis.yamlRequest=async(path,source,csrf=true)=>{const me=await fetch('/api/v1/auth/me',{credentials:'same-origin'}).then(r=>r.json());const r=await fetch(path,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/yaml',...(csrf?{'X-CSRF-Token':me.csrfToken}:{})},body:source});return {status:r.status,body:await r.text()};};true`);
  stage='raw-yaml-validation';await field(`(${batch}).querySelector('textarea')`,config.yaml,'HTMLTextAreaElement');
  assert.equal(await evaluate(`[...(${batch}).querySelectorAll('button')].find(b=>b.textContent.trim()==='Застосувати пакет').disabled`),true);
  await click(batch,'Перевірити пакет');await wait(`(${batch}).querySelector('pre.config-result')?.textContent.includes('"valid": true')`);
  stage='prepare';await click(batch,'Підготувати пакет');await wait(`(${batch}).querySelector('pre.config-result')?.textContent.includes('Підготовлено ресурсів: 3')`);
  const prepared=await evaluate(`(${batch}).querySelector('textarea').value`);
  assert.ok(prepared.includes('9007199254740993'));assert.match(prepared,/"ratio": 1\.0/);
  const requestId=JSON.parse(prepared).requestId;assert.ok(requestId);
  assert.equal(await evaluate(`(async()=>{const statuses=[];for(const p of ['/api/v1/resources/Monitor/yaml-web','/api/v1/resources/CheckProfile/yaml-http','/api/v1/resources/Target/yaml-target'])statuses.push(await fetch(p,{credentials:'same-origin'}).then(r=>r.status));return statuses;})()`).then(a=>a.every(x=>x===404)),true);
  stage='plan';await click(batch,'Показати план пакета');await wait(`(${batch}).querySelector('pre.config-result')?.textContent.includes('"planId"')`);
  stage='apply';await click(batch,'Застосувати пакет');await wait(`[...(${batch}).querySelectorAll('h3')].some(h=>h.textContent==='Застосування завершено')`);
  const operationText=await evaluate(`[...(${batch}).querySelectorAll('p')].find(p=>p.textContent.startsWith('Операція ')).textContent`);
  const operationId=operationText.split(' ')[1];assert.ok(operationId);
  stage='healthy-proof';await wait(`fetch('/api/v1/monitors/yaml-web/assessment',{credentials:'same-origin'}).then(r=>r.ok?r.json():null).then(a=>a?.state==='healthy'&&a.evidenceIds.length>0)`);
  const healthy=await evaluate(`fetch('/api/v1/monitors/yaml-web/assessment',{credentials:'same-origin'}).then(r=>r.json())`);
  const stored=await evaluate(`fetch('/api/v1/resources/Target/yaml-target',{credentials:'same-origin'}).then(r=>r.text())`);
  assert.ok(stored.includes('9007199254740993'));assert.match(stored,/"ratio":\s*1\.0/);
  stage='idempotent-ui-replay';const beforeReplay=responses.filter(r=>r.path==='/api/v1/config/apply').length;await click(batch,'Застосувати пакет');
  for(let n=0;n<90&&responses.filter(r=>r.path==='/api/v1/config/apply').length===beforeReplay;n++)await pause(100);
  assert.equal(responses.filter(r=>r.path==='/api/v1/config/apply').at(-1)?.status,202);await wait(`[...(${batch}).querySelectorAll('h3')].some(h=>h.textContent==='Застосування завершено')`);
  assert.equal(await evaluate(`[...(${batch}).querySelectorAll('p')].find(p=>p.textContent.startsWith('Операція ')).textContent.split(' ')[1]`),operationId);
  await evaluate(`(${batch}).scrollIntoView()`);
  const shot=await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:false});await writeFile(config.screenshot,Buffer.from(shot.data,'base64'),{flag:'wx',mode:0o600});
  stage='integer-not-coerced';await field(`(${batch}).querySelector('textarea')`,config.badIntegerYaml,'HTMLTextAreaElement');await click(batch,'Перевірити пакет');await wait(`(${batch}).querySelector('pre.config-result')?.textContent.includes('"valid": false')`);
  stage='duplicate-key';await field(`(${batch}).querySelector('textarea')`,config.yaml+'\nkind: Target\n','HTMLTextAreaElement');await click(batch,'Перевірити пакет');await wait(`(${batch}).querySelector('[role="alert"]')?.textContent.includes('Некоректний YAML/JSON')`);
  stage='explicit-preconditions';const old=await evaluate(`yamlRequest('/api/v1/config/prepare',${JSON.stringify(config.targetYaml)})`);assert.equal(old.status,200);const oldSource=JSON.parse(old.body).source;
  const edited=await evaluate(`yamlRequest('/api/v1/config/prepare',${JSON.stringify(config.targetYaml.replace('criticality: 3','criticality: 4'))})`);assert.equal(edited.status,200);
  const updated=await evaluate(`yamlRequest('/api/v1/config/apply',${JSON.stringify(JSON.parse(edited.body).source)})`);assert.equal(updated.status,202);
  await wait(`fetch('/api/v1/monitors/yaml-web/assessment',{credentials:'same-origin'}).then(r=>r.ok?r.json():null).then(a=>a?.state==='healthy'&&a.revision!==${JSON.stringify(healthy.revision)})`);
  const stale=await evaluate(`yamlRequest('/api/v1/config/prepare',${JSON.stringify(oldSource)})`);assert.equal(stale.status,200);assert.equal(JSON.parse(stale.body).source,oldSource);
  const denied=await evaluate(`yamlRequest('/api/v1/config/apply',${JSON.stringify(oldSource)})`);assert.equal(denied.status,412);
  const csrf=await evaluate(`yamlRequest('/api/v1/config/prepare',${JSON.stringify(config.targetYaml)},false)`);assert.equal(csrf.status,403);
  stage='logout';await click(`document.querySelector('main > section')`,'Від’єднатися');await wait(`document.querySelector('input[autocomplete="username"]')`);
  assert.equal(await evaluate(`fetch('/api/v1/config/prepare',{method:'POST',headers:{'Content-Type':'application/yaml'},body:${JSON.stringify(config.targetYaml)}}).then(r=>r.status)`),401);
  assert.deepEqual(errors,[]);
  await writeFile(config.output,JSON.stringify({status:'passed',browserInteractionTested:true,strictNodeTLS:true,browserPinnedOwnedServerKey:true,globalBrowserTrustUnchanged:true,realPasswordLogin:true,rawYAMLValidation:true,prepareDoesNotCreateResources:true,typedFloatPreserved:true,largeIntegerPreserved:'9007199254740993',integerFloatCoercionRejected:true,duplicateKeyRejected:true,preparedRequestId:requestId,operationId,idempotentReplaySameOperation:true,explicitStaleVersionsPreserved:true,staleApplyStatus:denied.status,csrfStatus:csrf.status,unauthenticatedPrepareStatus:401,healthyFromRealProbe:healthy,consoleExceptions:errors.length,credentialsOmitted:true},null,2)+'\n',{flag:'wx',mode:0o600});
  console.log('PASS: browser YAML prepare/validate/plan/apply/replay, numeric fidelity and stale versions');
} catch(error) {console.error('YAML browser verification failed at stage: '+stage);console.error('Failure category: '+(['Browser condition timeout','Browser evaluation failed','CDP timeout','CDP command failed'].includes(error.message)?error.message:'assertion or transport failure'));try{console.error('DOM metadata: '+JSON.stringify(await debugEvaluate(`({buttons:[...document.querySelectorAll('button')].map(b=>({text:b.textContent,disabled:b.disabled})),inputs:[...document.querySelectorAll('input')].map(i=>({type:i.type,autocomplete:i.autocomplete,length:i.value.length,valid:i.validity.valid})),errors:[...document.querySelectorAll('[role=alert]')].map(e=>e.textContent)})`)));}catch{};console.error('HTTP error metadata: '+JSON.stringify(responses.filter(r=>r.status>=400)));process.exitCode=1;}
finally {
  if(socket)socket.close();if(chrome?.pid&&chrome.exitCode===null){chrome.kill('SIGTERM');await Promise.race([once(chrome,'exit'),pause(3000)]);if(chrome.exitCode===null){chrome.kill('SIGKILL');await once(chrome,'exit');}}
  await rm(browserHome,{recursive:true,force:true,maxRetries:5,retryDelay:200});
}
