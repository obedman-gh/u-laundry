const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const script = fs.readFileSync(path.join(__dirname, '../frontend/login.js'), 'utf8');

// Page tests use the shared session transport contract; browser verification
// exercises the real cookie/CSRF/lifecycle module rather than these stubs.
function sessionBridge(context) {
 const auth = {
  get: async () => (await context.fetch('/api/session', {credentials:'same-origin'})).json(),
  fetch: (url, options = {}) => context.fetch(url, {...options, credentials:'same-origin',headers:{...options.headers,'X-CSRF-Token':'csrf-test'}}),
  forbid: text => { context.document.getElementById('message').textContent = text; }
 };
 context.window.ULaundrySession = auth;
 return auth;
}

async function login(response, networkError = false) {
 const elements = new Map();
 const element = id => {
  if (!elements.has(id)) elements.set(id, { style: {}, dataset: {}, value: 'synthetic', textContent: '', focus() {}, removeAttribute(name){delete this[name];}, setAttribute(name,value) { this[name] = value; }, addEventListener(type, fn) { this[type] = fn; } });
  return elements.get(id);
 };
 const stored = new Map([["token", "legacy-token"]]);
 const window = { location: { href: '',search:'', replace(url){this.href=url;} },addEventListener(){},ULaundryUI:{message(target,text){target.textContent=text;}},ULaundrySession:{recheck:async()=>({username:'synthetic',csrf_token:'synthetic-csrf-token'}),announceLogin(){}} };
 const context = vm.createContext({ document: { getElementById: element,addEventListener(){} }, window,URLSearchParams,TextEncoder,AbortController,setTimeout,clearTimeout,
  localStorage: { setItem() { throw new Error("JWT must not enter localStorage"); }, removeItem(k) { stored.delete(k); } }, console: { error() {} },
  fetch: async () => { if (networkError) throw new Error('synthetic offline'); return { ok: response.ok,status:response.status??(response.ok?200:500), json: async () => response.body }; }
 });
 vm.runInContext(script, context);
 element('formLogin').submit({ preventDefault() {} });
 await new Promise(resolve => setImmediate(resolve));
 return { error: element('pesan-error'), button: element('btnLogin'), stored, window };
}
test('login hides backend authentication details, clears passwords and permits retry', async () => {
 const result = await login({ ok: false,status:401, body: { error: { code: 'unauthorized', message: '<b>Pesan aman</b>', request_id: 'synthetic-id' } } });
 assert.equal(result.error.textContent, 'Nama pengguna atau kata sandi tidak valid.');
 assert.equal(result.button.disabled, false);
 assert.equal(result.stored.size, 0);
});
test('login uses generic text for unexpected JSON objects', async () => {
 const result = await login({ ok: false, body: { unexpected: 'synthetic' } });
 assert.match(result.error.textContent, /Layanan masuk sedang tidak tersedia/);
 assert.doesNotMatch(result.error.textContent, /object Object/);
});
test('successful cookie login clears legacy storage and redirects', async () => {
 const result = await login({ ok: true, body: { csrf_token: 'synthetic-csrf-token',expires_at:'2099-01-01T00:00:00Z' } });
 assert.equal(result.stored.size, 0);
 assert.equal(result.window.location.href, '/index.html');
});
test('network failure restores form without storing credentials', async () => {
 const result = await login({}, true);
 assert.equal(result.button.disabled, false);
 assert.equal(result.stored.size, 0);
 assert.match(result.error.textContent, /Server tidak merespons/);
});


test('cashier queue uses shared session transport and safe DOM rendering', async () => {
 const elements = new Map();
 const make = () => ({style:{},disabled:true,textContent:'',children:[],setAttribute(name,value){this[name]=value;},appendChild(child){this.children.push(child);},replaceChildren(){this.children=[];},addEventListener(type,fn){this[type]=fn;}});
 const element = id => {if(!elements.has(id)) elements.set(id,make());return elements.get(id);};
 const calls=[];const stored=new Map([['token','legacy-token']]);const window={location:{href:''}};
 const source=fs.readFileSync(path.join(__dirname,'../frontend/queue.js'),'utf8');
 const context=vm.createContext({document:{getElementById:element,createElement:make},window,localStorage:{removeItem(k){stored.delete(k);}},fetch:async(url,options)=>{
  calls.push({url,options});assert.equal(options.credentials,'same-origin');assert.equal(options.headers?.Authorization,undefined);
  const body=url==='/api/session'?{csrf_token:'csrf-synthetic'}:url==='/api/pesanan'?[{nama_customer:'<script>synthetic</script>',id_pesanan:'order-1',layanan:'Cuci',total_biaya:1000,status:'received'}]:{pesan:'Sesi telah diakhiri.'};
  return {ok:true,status:200,json:async()=>body};
 }});
 sessionBridge(context);
 vm.runInContext(source,context);await new Promise(resolve=>setImmediate(resolve));
 assert.equal(element('tempat-pesanan').children[0].children[0].textContent,'<script>synthetic</script> (order-1)');
 assert.equal(calls.length,2);assert.equal(calls[0].url,'/api/session');assert.equal(calls[1].url,'/api/pesanan');
});


async function staffPage(role = 'owner', conflict = false) {
 const elements=new Map(),calls=[];
 const make=()=>({dataset:{},style:{},children:[],value:'',hidden:true,disabled:false,textContent:'',attrs:{},focus(){},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];},closest(){return {dataset:{}};},querySelectorAll(){return [];},reset(){for(const id of ['create-name','create-username','create-password','create-confirm'])element(id).value='';element('create-role').value='cashier';},appendChild(c){this.children.push(c);},replaceChildren(){this.children=[];},addEventListener(type,fn){this[type]=fn;}});
 const element=id=>{if(!elements.has(id)){const e=make();e.id=id;elements.set(id,e);}return elements.get(id);};
 const records=[{id:'owner-1',name:'Owner',username:'owner.test',role:'owner',active:true,version:1},{id:'cashier-1',name:'<script>synthetic</script>',username:'cashier.test',role:'cashier',active:true,version:2}];
 const context=vm.createContext({TextEncoder,document:{getElementById:element,createElement:make,querySelectorAll:()=>[],querySelector:()=>Array.from(elements.values()).find(e=>e.attrs['aria-invalid']==='true')},window:{location:{href:''},confirm:()=>true,ULaundryUI:{message(target,text){target.textContent=text;}}},fetch:async(url,options)=>{
  calls.push({url,options});let body=url==='/api/session'?{role,user_id:'owner-1',csrf_token:'csrf-test'}:records;
  if(options.method){body=conflict?{error:{message:'Data sudah berubah.'}}:records[1];assert.equal(options.headers['X-CSRF-Token'],'csrf-test');}
  return {ok:!options.method||!conflict,status:options.method&&conflict?409:200,json:async()=>body};
 }});
 sessionBridge(context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/staff.js'),'utf8'),context);
 await new Promise(resolve=>setImmediate(resolve));return {element,calls,context};
}
test('staff UI denies non-owner without requesting the staff list',async()=>{
 const page=await staffPage('cashier');assert.equal(page.calls.length,1);assert.equal(page.element('management').hidden,true);assert.match(page.element('message').textContent,/hanya dapat digunakan pemilik/);
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
 const request=page.calls.find(c=>c.url.endsWith('/reset-password'));assert.ok(request);assert.equal(JSON.parse(request.options.body).version,2);assert.equal(e('edit-password').value,'');assert.equal(e('password-button').disabled,true);assert.match(e('message').textContent,/Data telah berubah/);
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
 sessionBridge(context);
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
 sessionBridge(context);
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
 const request=page.calls.find(c=>c.options.method==='POST');assert.equal(request.url,'/api/services');assert.deepEqual(JSON.parse(request.options.body),{name:'Selimut',unit:'item',unit_price:20000,turnaround_minutes:2880,active:true,display_order:7});assert.match(e('message').textContent,/Harga pesanan lama tetap/);
});

test('catalog UI rejects fractional/unsafe money and clears stale edits',async()=>{
 const page=await catalogPage('admin',true),e=page.element;await e('services').children[0].children[0].click();
 for(const invalid of ['1.5','9007199254740992']){e('price').value=invalid;await e('service-form').submit({preventDefault(){}});assert.equal(page.calls.filter(c=>c.options.method).length,0);}
 e('price').value='21000';await e('service-form').submit({preventDefault(){}});
 const request=page.calls.find(c=>c.options.method==='PUT');assert.equal(JSON.parse(request.options.body).version,2);assert.equal(e('name').value,'');assert.equal(e('history').children.length,0);assert.equal(e('save').disabled,false);assert.match(e('message').textContent,/Versi layanan sudah berubah/);
});

function sessionModule(transport, protectedPage = true) {
 const elements = new Map(), redirects = [], calls = [], events = {};
 const make = () => ({hidden:false,children:[],value:'',disabled:true,setAttribute(){},appendChild(node){this.children.push(node);},append(...nodes){this.children.push(...nodes);},querySelectorAll(){return [];},addEventListener(type,fn){events[type]=fn;}});
 const element = id => { if (!elements.has(id)) elements.set(id,make()); return elements.get(id); };
 const main = make(), ui = {state(target,state){target.state=state;},setSession(){},clearSession(){},notify(){}};
 const window = {ULaundryUI:ui,location:{origin:'https://laundry.test',replace(url){redirects.push(url);},reload(){}},addEventListener(type,fn){events[type]=fn;}};
 const context = vm.createContext({window,document:{body:{hasAttribute:()=>protectedPage},visibilityState:'visible',querySelector:()=>main,querySelectorAll:()=>[],getElementById:element,createElement:make,addEventListener(type,fn){events[type]=fn;}},navigator:{onLine:true},URL,Headers,AbortController,setTimeout:()=>1,clearTimeout(){},fetch:async(url,options)=>{calls.push({url,options});return transport(url,options);}});
 vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/ui/session.js'),'utf8'),context);
 return {auth:window.ULaundrySession,redirects,calls,main,element,events};
}
const activeSession = {user_id:'owner-1',username:'owner.test',role:'owner',csrf_token:'memory-only-csrf',expires_at:'2099-01-01T01:00:00Z'};
function sessionResponse(status = 200, body = activeSession) {
 return {ok:status>=200&&status<300,status,headers:new Headers({Date:'Thu, 01 Jan 2099 00:00:00 GMT'}),json:async()=>body};
}

test('shared session deduplicates checks and keeps requests within cookie transport',async()=>{
 let release;const pending = new Promise(resolve=>{release=resolve;});
 const page=sessionModule(()=>pending);const first=page.auth.get(),second=page.auth.get();
 assert.equal(page.calls.length,1);release(sessionResponse());
 assert.equal((await first).user_id,'owner-1');assert.equal((await second).role,'owner');
 await page.auth.get();assert.equal(page.calls.length,1);
 assert.equal(page.calls[0].options.credentials,'same-origin');assert.equal(page.calls[0].options.cache,'no-store');assert.equal(page.calls[0].options.redirect,'error');
});

test('shared session stamps current CSRF and never retries an uncertain write',async()=>{
 const page=sessionModule((url)=>{if(url==='/api/session')return sessionResponse();throw Error('synthetic lost response');});
 await assert.rejects(page.auth.fetch('/api/staff',{method:'POST',headers:{'X-CSRF-Token':'stale'},body:'{}'}),/lost response/);
 assert.equal(page.calls.length,2);assert.equal(page.calls[1].options.headers.get('X-CSRF-Token'),'memory-only-csrf');assert.equal(page.element('logout').disabled,false);
});

test('focus and logout cannot race a pending account mutation',async()=>{
 let release;const page=sessionModule(url=>url==='/api/session'?sessionResponse():new Promise(resolve=>{release=resolve;}));
 const write=page.auth.fetch('/api/staff',{method:'POST',body:'{}'});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(page.element('logout').disabled,true);page.events.focus();await page.auth.logout();assert.equal(page.calls.length,2);assert.deepEqual(page.redirects,[]);
 release(sessionResponse());await write;assert.equal(page.element('logout').disabled,false);
});

test('shared session blocks cross-origin targets and bearer credentials',async()=>{
 const page=sessionModule(()=>sessionResponse());
 for(const url of ['https://elsewhere.test/api/staff','//elsewhere.test/api/staff','/api/../../other'])await assert.rejects(page.auth.fetch(url),/Tujuan/);
 await assert.rejects(page.auth.fetch('/api/staff',{headers:{Authorization:'Bearer prohibited'}}),/autentikasi/);
 assert.equal(page.calls.length,1);assert.equal(page.calls[0].url,'/api/session');
});

test('protected 401 redirects once while public login 401 stays for a manual retry',async()=>{
 const protectedUI=sessionModule(()=>sessionResponse(401));await assert.rejects(protectedUI.auth.get(),/Sesi/);await assert.rejects(protectedUI.auth.get(),/Sesi/);
 assert.deepEqual(protectedUI.redirects,['/login.html?reason=expired']);assert.equal(protectedUI.calls.length,1);
 const loginUI=sessionModule(()=>sessionResponse(401),false);await assert.rejects(loginUI.auth.recheck(),/Sesi/);assert.deepEqual(loginUI.redirects,[]);
});

test('business 401 conceals private content and suppresses subsequent requests',async()=>{
 const page=sessionModule(url=>sessionResponse(url==='/api/session'?200:401));await page.auth.get();
 await assert.rejects(page.auth.fetch('/api/staff'),/Sesi/);assert.equal(page.main.children[1].hidden,true);
 await assert.rejects(page.auth.fetch('/api/staff'),/Sesi/);assert.equal(page.calls.length,2);assert.deepEqual(page.redirects,['/login.html?reason=expired']);
});

test('session outage conceals data, recovers without logout and rejects changed identity',async()=>{
 let mode=200,role='owner';const page=sessionModule(()=>sessionResponse(mode,{...activeSession,role}));
 await page.auth.get();mode=503;await assert.rejects(page.auth.recheck(),/diperiksa/);assert.equal(page.main.children[1].hidden,true);assert.deepEqual(page.redirects,[]);
 mode=200;await page.auth.recheck();assert.equal(page.main.children[1].hidden,false);
 role='cashier';await assert.rejects(page.auth.recheck(),/Sesi/);assert.deepEqual(page.redirects,['/login.html?reason=changed']);
});

test('logout failure requires explicit retry and successful logout rejects late session responses',async()=>{
 let mode=503;const page=sessionModule(url=>sessionResponse(url==='/api/logout'?mode:200));
 await page.auth.get();await page.auth.logout();assert.deepEqual(page.redirects,[]);assert.equal(page.main.children[1].hidden,true);
 mode=200;await page.auth.logout();assert.deepEqual(page.redirects,['/login.html?reason=signed-out']);assert.equal(page.calls.filter(c=>c.url==='/api/logout').length,2);assert.equal(page.calls[1].options.headers['X-CSRF-Token'],'memory-only-csrf');
 let release;const late=sessionModule(()=>new Promise(resolve=>{release=resolve;}));const check=late.auth.get();late.auth.end('signed-out');release(sessionResponse());await assert.rejects(check,/Sesi/);await assert.rejects(late.auth.get(),/Sesi/);assert.equal(late.redirects.length,1);
});

const posContext=vm.createContext({window:{}});
vm.runInContext(fs.readFileSync(path.join(__dirname,'../frontend/pos-model.js'),'utf8'),posContext);
const pos=posContext.window.ULaundryPOSModel;
const posCatalog=new Map([['wash',{service_id:'wash',unit:'kg',unit_price:7005,turnaround_minutes:1440}],['blanket',{service_id:'blanket',unit:'item',unit_price:20000,turnaround_minutes:2880}]]);
const posDraft=()=>({customer:{customer_id:'customer-1'},lines:[{key:'1',serviceId:'wash',quantity:'0,100'},{key:'2',serviceId:'blanket',quantity:'2'}],discount:'0',discountReason:'',notes:'Pisahkan warna',readyAt:''});

test('POS converts decimal kg to integer grams without float rounding',()=>{
 for(const [value,expected] of [['0,001',1],['2.500',2500],['12,345',12345]])assert.equal(pos.quantity('kg',value,'weight').grams,expected);
 for(const value of ['0','0.0001','1e3','1,000.5','-1','9007199254740.992'])assert.throws(()=>pos.quantity('kg',value,'weight'));
 for(const value of ['0','1.5','1,5','9007199254740992'])assert.throws(()=>pos.quantity('item',value,'pieces'));
});

test('POS preview rounds half-up per line and rejects unsafe totals',()=>{
 const split=pos.calculate([{key:'1',serviceId:'wash',quantity:'0.1'},{key:'2',serviceId:'wash',quantity:'0.1'}],posCatalog);
 assert.equal(split.subtotal,1402);assert.equal(pos.calculate([{key:'1',serviceId:'wash',quantity:'0.2'}],posCatalog).subtotal,1401);
 assert.throws(()=>pos.calculate([{key:'1',serviceId:'blanket',quantity:'9007199254740991'}],posCatalog),/melampaui/);
});

test('POS request contains IDs and integer quantities without client prices or totals',()=>{
 const built=pos.build(posDraft(),posCatalog,'cashier');
 assert.equal(built.total,40701);assert.deepEqual(JSON.parse(JSON.stringify(built.payload)),{customer_id:'customer-1',items:[{service_id:'wash',grams:100},{service_id:'blanket',pieces:2}],discount:0,notes:'Pisahkan warna'});
});

test('POS discount requires current owner/admin role, reason and a bounded integer amount',()=>{
 const draft=posDraft();draft.discount='1';draft.discountReason='Penyesuaian';
 assert.throws(()=>pos.build(draft,posCatalog,'cashier'),/pemilik atau admin/);
 for(const role of ['owner','admin'])assert.equal(pos.build(draft,posCatalog,role).total,40700);
 draft.discount='40702';assert.throws(()=>pos.build(draft,posCatalog,'owner'),/melebihi/);
 draft.discount='1.5';assert.throws(()=>pos.build(draft,posCatalog,'owner'),/bilangan bulat/);
 draft.discount='1';draft.discountReason='';assert.throws(()=>pos.build(draft,posCatalog,'owner'));
 draft.discount='0';draft.discountReason='Tidak berlaku';assert.throws(()=>pos.build(draft,posCatalog,'owner'),/Kosongkan/);
});

test('POS estimate explicitly interprets WITA and validates calendar/future time',()=>{
 const now=Date.parse('2026-11-25T00:00:00Z');assert.equal(pos.estimate('2026-11-26T09:30',now),'2026-11-26T01:30:00.000Z');
 for(const value of ['2026-11-25T07:59','2026-02-31T12:00','2026-13-01T12:00','not-a-date'])assert.throws(()=>pos.estimate(value,now));
 assert.equal(pos.estimate('',now),undefined);
});

test('POS accepts server snapshot totals independently of preview and rejects inconsistent/unsafe data',()=>{
 const order={id_pesanan:'order-1',nama_customer:'<img src=x>',status:'received',payment_status:'unpaid',waktu_perkiraan_selesai:'2026-11-26T00:00:00Z',subtotal:800,discount:1,total_biaya:799,items:[{service_id:'wash',service_name:'Cuci',unit:'kg',unit_price:8000,grams:100,pieces:0,line_total:800}]};
 assert.equal(pos.result({order,replayed:false}).total_biaya,799);
 assert.throws(()=>pos.result({order:{...order,total_biaya:700},replayed:false}),/Total/);
 assert.throws(()=>pos.result({order:{...order,total_biaya:9007199254740992},replayed:false}));
 assert.equal(pos.result({order:{...order,discount:800,total_biaya:0,payment_status:'paid'},replayed:true}).total_biaya,0);
});

test('POS rejects missing customer, unavailable service and excess lines before sending',()=>{
 const draft=posDraft();draft.customer=undefined;assert.throws(()=>pos.build(draft,posCatalog,'cashier'),/pelanggan/);
 draft.customer={customer_id:'customer-1'};draft.lines[0].serviceId='missing';assert.throws(()=>pos.build(draft,posCatalog,'cashier'),/layanan aktif/);
 draft.lines=Array.from({length:101},()=>({key:'1',serviceId:'wash',quantity:'1'}));assert.throws(()=>pos.build(draft,posCatalog,'cashier'),/1–100/);
});
