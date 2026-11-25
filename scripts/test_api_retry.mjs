// Verify that transient read recovery never repeats an ambiguous mutation.
import assert from 'node:assert/strict';
import { validateConfigurationSource, prepareConfigurationSource, applyConfigurationSource } from '../web/src/api.ts';
const original=globalThis.fetch;
const session={csrfToken:'fixture-csrf'};
try {
 let calls=0;
 globalThis.fetch=async()=>{calls++;if(calls===1)throw new TypeError('fixture disconnect');return new Response(JSON.stringify({valid:true}),{headers:{'Content-Type':'application/json'}});};
 assert.equal((await validateConfigurationSource(session,'fixture')).valid,true);
 assert.equal(calls,2);
 calls=0;
 globalThis.fetch=async()=>{calls++;return calls===1?new Response('{}',{status:503,headers:{'Retry-After':'1'}}):new Response(JSON.stringify({resourceCount:1}));};
 assert.equal((await prepareConfigurationSource(session,'fixture')).resourceCount,1);
 assert.equal(calls,2);
 calls=0;
 globalThis.fetch=async()=>{calls++;throw new TypeError('fixture disconnect');};
 await assert.rejects(()=>applyConfigurationSource(session,'unchanged requestId'),/перервано/);
 assert.equal(calls,1,'An ambiguous apply must not be replayed');
 console.log('PASS: transport/503 recovery for read-only config requests; mutations sent once');
} finally {globalThis.fetch=original;}
