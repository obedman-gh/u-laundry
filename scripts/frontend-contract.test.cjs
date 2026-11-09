const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const html = fs.readFileSync(path.join(__dirname, '../frontend/login.html'), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];

async function login(response, networkError = false) {
 const elements = new Map();
 const element = id => {
  if (!elements.has(id)) elements.set(id, { style: {}, value: 'synthetic', textContent: '', addEventListener(type, fn) { this[type] = fn; } });
  return elements.get(id);
 };
 const stored = new Map([["token", "legacy-token"]]);
 const window = { location: { href: '' } };
 const context = vm.createContext({ document: { getElementById: element }, window,
  localStorage: { setItem() { throw new Error("JWT must not enter localStorage"); }, removeItem(k) { stored.delete(k); } }, console: { error() {} },
  fetch: async () => { if (networkError) throw new Error('synthetic offline'); return { ok: response.ok, json: async () => response.body }; }
 });
 vm.runInContext(script, context);
 element('formLogin').submit({ preventDefault() {} });
 await new Promise(resolve => setImmediate(resolve));
 return { error: element('pesan-error'), button: element('btnLogin'), stored, window };
}
test('login renders nested API error as safe text and permits retry', async () => {
 const result = await login({ ok: false, body: { error: { code: 'unauthorized', message: '<b>Pesan aman</b>', request_id: 'synthetic-id' } } });
 assert.equal(result.error.textContent, '<b>Pesan aman</b>');
 assert.equal(result.button.disabled, false);
 assert.equal(result.error.style.display, 'block');
 assert.equal(result.stored.size, 0);
});
test('login uses generic text for unexpected JSON objects', async () => {
 const result = await login({ ok: false, body: { unexpected: 'synthetic' } });
 assert.match(result.error.textContent, /^Gagal login\./);
 assert.doesNotMatch(result.error.textContent, /object Object/);
});
test('successful cookie login clears legacy storage and redirects', async () => {
 const result = await login({ ok: true, body: { csrf_token: 'synthetic-csrf-token' } });
 assert.equal(result.stored.size, 0);
 assert.equal(result.window.location.href, '/index.html');
});
test('network failure restores form without storing credentials', async () => {
 const result = await login({}, true);
 assert.equal(result.button.disabled, false);
 assert.equal(result.stored.size, 0);
 assert.match(result.error.textContent, /Server tidak merespon/);
});


test('cashier loads session and orders with cookies, renders safely, logs out with CSRF', async () => {
 const elements = new Map();
 const make = () => ({style:{},disabled:true,textContent:'',children:[],appendChild(child){this.children.push(child);},replaceChildren(){this.children=[];},addEventListener(type,fn){this[type]=fn;}});
 const element = id => {if(!elements.has(id)) elements.set(id,make());return elements.get(id);};
 const calls=[];const stored=new Map([['token','legacy-token']]);const window={location:{href:''}};
 const source=fs.readFileSync(path.join(__dirname,'../frontend/index.html'),'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
 const context=vm.createContext({document:{getElementById:element,createElement:make},window,localStorage:{removeItem(k){stored.delete(k);}},fetch:async(url,options)=>{
  calls.push({url,options});assert.equal(options.credentials,'same-origin');assert.equal(options.headers?.Authorization,undefined);
  const body=url==='/api/session'?{csrf_token:'csrf-synthetic'}:url==='/api/pesanan'?[{nama_customer:'<script>synthetic</script>',id_pesanan:'order-1',layanan:'Cuci',total_biaya:1000,status:'received'}]:{pesan:'Sesi telah diakhiri.'};
  return {ok:true,status:200,json:async()=>body};
 }});
 vm.runInContext(source,context);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(stored.size,0);assert.equal(element('logout').disabled,false);
 assert.equal(element('tempat-pesanan').children[0].children[0].textContent,'<script>synthetic</script> (order-1)');
 await element('logout').click();
 assert.equal(calls[2].url,'/api/logout');assert.equal(calls[2].options.headers['X-CSRF-Token'],'csrf-synthetic');
 assert.equal(window.location.href,'/login.html');
});


async function staffPage(role = 'owner', conflict = false) {
 const elements=new Map(),calls=[];
 const make=()=>({dataset:{},style:{},children:[],value:'',hidden:true,disabled:false,textContent:'',appendChild(c){this.children.push(c);},replaceChildren(){this.children=[];},addEventListener(type,fn){this[type]=fn;}});
 const element=id=>{if(!elements.has(id))elements.set(id,make());return elements.get(id);};
 const records=[{id:'owner-1',name:'Owner',username:'owner.test',role:'owner',active:true,version:1},{id:'cashier-1',name:'<script>synthetic</script>',username:'cashier.test',role:'cashier',active:true,version:2}];
 const context=vm.createContext({document:{getElementById:element,createElement:make},window:{location:{href:''},confirm:()=>true},fetch:async(url,options)=>{
  calls.push({url,options});let body=url==='/api/session'?{role,user_id:'owner-1',csrf_token:'csrf-test'}:records;
  if(options.method){body=conflict?{error:{message:'Data sudah berubah.'}}:records[1];assert.equal(options.headers['X-CSRF-Token'],'csrf-test');}
  return {ok:!options.method||!conflict,status:options.method&&conflict?409:200,json:async()=>body};
 }});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/staff.js'),'utf8'),context);
 await new Promise(resolve=>setImmediate(resolve));return {element,calls,context};
}
test('staff UI denies non-owner without requesting the staff list',async()=>{
 const page=await staffPage('cashier');assert.equal(page.calls.length,1);assert.equal(page.element('management').hidden,true);assert.match(page.element('message').textContent,/hanya dapat digunakan owner/);
});
test('owner staff UI creates an account without rendering or retaining its password',async()=>{
 const page=await staffPage();const e=page.element;
 assert.equal(e('management').hidden,false);assert.equal(e('staff-list').children[1].children[0].textContent,'<script>synthetic</script> / cashier.test');
 e('create-name').value='Staff';e('create-username').value='new.test';e('create-role').value='cashier';e('create-password').value=e('create-confirm').value='Frasa sandi sintetis';
 await e('create-form').submit({preventDefault(){}});
 const request=page.calls.find(c=>c.url==='/api/staff'&&c.options.method==='POST');assert.ok(request);assert.equal(JSON.parse(request.options.body).password,'Frasa sandi sintetis');assert.equal(e('create-password').value,'');assert.equal(e('create-confirm').value,'');assert.doesNotMatch(e('message').textContent,/Frasa/);
});
test('staff changes send version and CSRF; stale responses clear the selection',async()=>{
 const page=await staffPage('owner',true);const e=page.element;e('staff-list').children[1].children[3].children[0].click();
 e('edit-password').value=e('edit-confirm').value='Frasa pengganti sintetis';e('password-form').submit({preventDefault(){}});await new Promise(resolve=>setImmediate(resolve));
 const request=page.calls.find(c=>c.url.endsWith('/reset-password'));assert.ok(request);assert.equal(JSON.parse(request.options.body).version,2);assert.equal(e('edit-password').value,'');assert.equal(e('password-button').disabled,true);assert.match(e('message').textContent,/Data sudah berubah/);
});

async function customersPage(conflict = false) {
 const elements=new Map(),calls=[];
 const make=()=>({children:[],value:'',hidden:false,disabled:false,textContent:'',appendChild(c){this.children.push(c);},replaceChildren(){this.children=[];this.textContent='';},addEventListener(type,fn){this[type]=fn;}});
 const element=id=>{if(!elements.has(id))elements.set(id,make());return elements.get(id);};
 const customer={customer_id:'customer-1',name:'<script>Synthetic</script>',contact:'',address:'Viola',notes:'Synthetic note',version:2};
 const context=vm.createContext({document:{getElementById:element,createElement:make},window:{location:{href:''}},fetch:async(url,options)=>{
  calls.push({url,options});assert.equal(options.credentials,'same-origin');
  let body=url==='/api/session'?{csrf_token:'csrf-test'}:url.includes('/orders?')?[{id_pesanan:'order-1',status:'received',total_biaya:1000,payment_status:'unpaid',version:3}]:url.includes('?')?[customer]:customer;
  if(options.method){assert.equal(options.headers['X-CSRF-Token'],'csrf-test');body=conflict?{error:{message:'Data sudah berubah.'}}:url.endsWith('/tracking')?{tracking_token:'new-secret-token',version:4}:customer;}
  return {ok:!options.method||!conflict,status:options.method&&conflict?409:200,json:async()=>body};
 }});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/customers.js'),'utf8'),context);
 await new Promise(resolve=>setImmediate(resolve));return {element,calls};
}

test('customer UI uses minimal create and safe DOM, scoped history and token rotation',async()=>{
 const page=await customersPage(),e=page.element;
 assert.equal(e('customers').children[0].children[0].textContent,'<script>Synthetic</script>');
 e('name').value='Name only';await e('customer-form').submit({preventDefault(){}});
 const created=page.calls.find(c=>c.options.method==='POST');assert.equal(created.url,'/api/customers');assert.deepEqual(JSON.parse(created.options.body),{name:'Name only',contact:'',address:'',notes:''});
 assert.ok(page.calls.some(c=>c.url==='/api/customers/customer-1/orders?limit=20&offset=0'));
 const row=e('orders').children[0];assert.equal(row.children[0].href,'/api/pesanan/order-1/struk');
 await row.children[1].click();
 const rotation=page.calls.find(c=>c.url==='/api/pesanan/order-1/tracking');assert.deepEqual(JSON.parse(rotation.options.body),{version:3});
 assert.equal(row.children[2].children[0].href,'/tracking.html#new-secret-token');assert.match(e('message').textContent,/Tautan lama dicabut/);
});

test('stale customer edit clears optional personal data and requires reselection',async()=>{
 const page=await customersPage(true),e=page.element;
 await e('customers').children[0].children[0].click();
 assert.equal(e('address').value,'Viola');
 await e('customer-form').submit({preventDefault(){}});
 const update=page.calls.find(c=>c.options.method==='PUT');assert.equal(update.url,'/api/customers/customer-1');assert.equal(JSON.parse(update.options.body).version,2);
 assert.equal(e('address').value,'');assert.equal(e('notes').value,'');assert.equal(e('orders').children.length,0);assert.match(e('message').textContent,/Data sudah berubah/);
});

test('public tracking consumes fragment without persistence and renders only public fields',async()=>{
 const elements=new Map(),calls=[];let cleared='';
 const element=id=>{if(!elements.has(id))elements.set(id,{value:'',textContent:'',hidden:false,disabled:false,addEventListener(type,fn){this[type]=fn;}});return elements.get(id);};
 const token='B'.repeat(42)+'A';
 const context=vm.createContext({document:{getElementById:element},window:{location:{hash:'#'+token},history:{replaceState(_,__,path){cleared=path;}}},fetch:async(url,options)=>{
  calls.push({url,options});return{ok:true,status:200,json:async()=>({tracking_code:'abc123def456',status:'processing',estimated_ready_at:'2026-11-10T00:00:00Z',message:'<b>Diproses</b>'})};
 }});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/tracking.js'),'utf8'),context);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(cleared,'/tracking.html');assert.equal(calls[0].url,'/api/tracking/'+token);assert.equal(calls[0].options.credentials,'omit');assert.equal(calls[0].options.referrerPolicy,'no-referrer');assert.equal(element('service-message').textContent,'<b>Diproses</b>');assert.equal(element('tracking-code').textContent,'abc123def456');assert.equal(element('token').value,'');
 element('token').value='ORDER-1';await element('tracking-form').submit({preventDefault(){}});await new Promise(resolve=>setImmediate(resolve));assert.equal(calls.length,1);assert.equal(element('result').hidden,true);assert.match(element('message').textContent,/tidak ditemukan/);
});

async function catalogPage(role = 'owner', conflict = false) {
 const elements=new Map(),calls=[];
 const make=()=>({children:[],value:'',hidden:true,disabled:false,textContent:'',appendChild(c){this.children.push(c);},replaceChildren(){this.children=[];this.textContent='';},addEventListener(type,fn){this[type]=fn;}});
 const element=id=>{if(!elements.has(id))elements.set(id,make());return elements.get(id);};
 const record={service_id:'service-1',name:'<script>Synthetic</script>',unit:'item',unit_price:20000,turnaround_minutes:2880,active:true,display_order:5,version:2,source:'baseline',changed_at:'2026-11-09T00:00:00Z'};
 const context=vm.createContext({document:{getElementById:element,createElement:make},window:{location:{href:''}},fetch:async(url,options)=>{
  calls.push({url,options});assert.equal(options.credentials,'same-origin');
  let body=url==='/api/session'?{role,csrf_token:'csrf-test'}:url.includes('?')?[record]:record;
  if(options.method){assert.equal(options.headers['X-CSRF-Token'],'csrf-test');body=conflict?{error:{message:'Versi layanan sudah berubah.'}}:record;}
  return {ok:!options.method||!conflict,status:options.method&&conflict?409:200,json:async()=>body};
 }});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/services.js'),'utf8'),context);await new Promise(resolve=>setImmediate(resolve));return {element,calls};
}

test('catalog UI limits management to owner/admin and renders history as safe text',async()=>{
 for(const role of ['owner','admin']){
  const page=await catalogPage(role),e=page.element;assert.equal(e('management').hidden,false);assert.match(e('services').children[0].children[0].textContent,/<script>Synthetic/);
  await e('services').children[0].children[0].click();assert.equal(e('unit').value,'item');assert.equal(e('price').value,'20000');assert.match(e('history').children[0].textContent,/Kondisi awal tercatat/);
 }
 const denied=await catalogPage('cashier');assert.equal(denied.calls.length,1);assert.equal(denied.element('management').hidden,true);
});

test('catalog UI creates a service with integer prices and ordered active catalog fields',async()=>{
 const page=await catalogPage(),e=page.element;e('name').value='Selimut';e('unit').value='item';e('price').value='20000';e('minutes').value='2880';e('display-order').value='7';e('active').value='true';
 await e('service-form').submit({preventDefault(){}});
 const request=page.calls.find(c=>c.options.method==='POST');assert.equal(request.url,'/api/services');assert.deepEqual(JSON.parse(request.options.body),{name:'Selimut',unit:'item',unit_price:20000,turnaround_minutes:2880,active:true,display_order:7});assert.match(e('message').textContent,/Pesanan lama tetap memakai snapshot/);
});

test('catalog UI rejects fractional/unsafe money and clears stale edits',async()=>{
 const page=await catalogPage('admin',true),e=page.element;await e('services').children[0].children[0].click();
 for(const invalid of ['1.5','9007199254740992']){e('price').value=invalid;await e('service-form').submit({preventDefault(){}});assert.equal(page.calls.filter(c=>c.options.method).length,0);}
 e('price').value='21000';await e('service-form').submit({preventDefault(){}});
 const request=page.calls.find(c=>c.options.method==='PUT');assert.equal(JSON.parse(request.options.body).version,2);assert.equal(e('name').value,'');assert.equal(e('history').children.length,0);assert.equal(e('save').disabled,false);assert.match(e('message').textContent,/Versi layanan sudah berubah/);
});
