// Targeted PWA browser verification: Node 22+ (native WebSocket), installed Edge/Chromium.
// Loopback fixture only; no database, credentials, npm packages, or business data.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const net = require('node:net');
const os = require('node:os');
const {spawn} = require('node:child_process');
const assert = require('node:assert/strict');
const base = path.resolve(__dirname, '../frontend');
// Exercise the exact CSP emitted by the Go middleware on the loopback fixture.
const csp = fs.readFileSync(path.join(__dirname,'../internal/http/middleware/security.go'),'utf8')
  .match(/h.Set\("Content-Security-Policy", "([^"]+)"\)/)[1];
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const profile = fs.mkdtempSync(path.join(os.tmpdir(), 'ulaundry-pwa-'));
const browser = process.env.PWA_BROWSER || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
const mime = {'.html':'text/html; charset=utf-8', '.js':'application/javascript', '.css':'text/css',
  '.webmanifest':'application/manifest+json', '.png':'image/png', '.svg':'image/svg+xml'};
let release = 'A', assetRelease = 'A', outage = false, brokenInstall = false, stalledNavigation = false, navigationStatus = 200, navigationMarker = '';
const trace = [], exceptions = [], clients = [];
let edge;
const server = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost');
  trace.push({path:url.pathname, method:req.method, release});
  if (outage) { req.socket.destroy(); return; }
  if (stalledNavigation && url.pathname === '/login.html') return;
  res.setHeader('Cache-Control', 'no-store');
  res.setHeader('Content-Security-Policy', csp);
  res.setHeader('X-Content-Type-Options', 'nosniff');
  if (url.pathname.startsWith('/api/') || url.pathname === '/private-receipt') {
    res.setHeader('Content-Type', url.pathname.includes('struk') ? 'text/html' : 'application/json');
    if (url.pathname === '/api/session') { res.statusCode = 401; res.end('{"error":{"code":"unauthenticated","message":"Sesi tidak aktif."}}'); return; }
    res.end(JSON.stringify({fixture:'SYNTHETIC_PRIVATE_DATA', method:req.method})); return;
  }
  const relative = url.pathname === '/' ? 'index.html' : url.pathname.slice(1);
  const file = path.resolve(base, relative);
  if (!file.startsWith(base + path.sep) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) { res.statusCode = 404; res.end('Not found'); return; }
  if (brokenInstall && url.pathname === '/icons/icon-512.png') { res.statusCode = 503; res.end('unavailable'); return; }
  res.setHeader('Content-Type', mime[path.extname(file)] || 'application/octet-stream');
  let data = fs.readFileSync(file);
  if (url.pathname === '/sw.js') data = Buffer.from(data.toString().replace(/const VERSION = '[^']+';/, "const VERSION = 'fixture-" + release + "';"));
  if (url.pathname === '/login.js') {
    res.setHeader('Cache-Control', 'public, max-age=31536000');
    data = Buffer.from(data.toString() + '\nwindow.__pwaFixtureRelease=' + JSON.stringify(assetRelease) + ';');
  }
  if (url.pathname.endsWith('.html')) {
    res.statusCode = navigationStatus;
    data = Buffer.from(data.toString().replace('</body>', navigationMarker + '</body>'));
  }
  res.end(data);
});
async function wait(fn, label) {
  for (let count = 0; count < 160; count++) {
    try { if (await fn()) return; } catch (error) { if (!/context|target|execution/i.test(error.message)) throw error; }
    await sleep(75);
  }
  throw new Error('Timeout: ' + label);
}
async function connect(url) {
  const ws = new WebSocket(url);
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
  let id = 0; const pending = new Map();
  const send = (method, params = {}) => new Promise((resolve, reject) => {
    const key = ++id; pending.set(key, {resolve, reject}); ws.send(JSON.stringify({id:key, method, params}));
    setTimeout(() => { if (pending.delete(key)) reject(new Error('CDP timeout: ' + method)); }, 12000).unref();
  });
  ws.onmessage = event => {
    const message = JSON.parse(event.data);
    if (message.id) {
      const result = pending.get(message.id); if (!result) return;
      pending.delete(message.id);
      if (message.error) result.reject(new Error(message.error.message)); else result.resolve(message.result);
    }
    if (message.method === 'Runtime.exceptionThrown') exceptions.push(message.params.exceptionDetails.text);
    if (message.method === 'Runtime.consoleAPICalled' && message.params.type === 'error') exceptions.push('console.error');
  };
  const ev = async expression => {
    const result = await send('Runtime.evaluate', {expression:`(async () => (${expression}))()`, awaitPromise:true, returnByValue:true});
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  };
  const client = {ws, send, ev}; clients.push(client);
  await send('Runtime.enable'); await send('Page.enable'); await send('Network.enable');
  return client;
}
async function main() {
  assert(fs.existsSync(browser), 'Set PWA_BROWSER to an installed Edge/Chromium executable');
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = 'http://127.0.0.1:' + server.address().port;
  const probe = net.createServer(); await new Promise(resolve => probe.listen(0, '127.0.0.1', resolve));
  const port = probe.address().port; await new Promise(resolve => probe.close(resolve));
  edge = spawn(browser, ['--headless=new', '--no-first-run', '--disable-gpu', '--no-sandbox', '--disable-background-networking',
    '--remote-debugging-port=' + port, '--user-data-dir=' + profile, 'about:blank'], {windowsHide:true, stdio:'ignore'});
  let target;
  await wait(async () => {
    try { target = (await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()).find(item => item.type === 'page'); return !!target; }
    catch (_) { return false; }
  }, 'browser startup');
  const a = await connect(target.webSocketDebuggerUrl);
  const nav = async (client, route) => {
    await client.send('Page.navigate', {url:origin + route});
    await wait(() => client.ev('document.readyState === "complete" && !!document.querySelector(".pwa-panel")'), route);
  };
  const buttons = label => `[...document.querySelectorAll('.pwa-panel button')].find(b => b.textContent === ${JSON.stringify(label)})`;
  const click = (client, label) => client.ev(`${buttons(label)}.click()`);
  const screenshot = async (client, name) => {
    if (!process.env.PWA_ARTIFACT_DIR) return;
    await client.send('Page.bringToFront');
    fs.mkdirSync(process.env.PWA_ARTIFACT_DIR, {recursive:true});
    const result = await client.send('Page.captureScreenshot', {captureBeyondViewport:false});
    fs.writeFileSync(path.join(process.env.PWA_ARTIFACT_DIR, name + '.png'), Buffer.from(result.data, 'base64'));
  };
  const cacheKeys = client => client.ev(`(async () => { const all = {}; for (const name of await caches.keys()) all[name] = (await (await caches.open(name)).keys()).map(r => r.url); return all; })()`);
  await a.send('Emulation.setDeviceMetricsOverride', {width:1280, height:960, deviceScaleFactor:1, mobile:false});
  await nav(a, '/login.html');
  const policy = await a.ev(`(async () => {
    const violations = [];
    document.addEventListener('securitypolicyviolation', event => violations.push(event.effectiveDirective));
    const script = document.createElement('script'); script.textContent = 'window.__injected=true'; document.body.appendChild(script);
    const base = document.createElement('base'); base.href = 'https://blocked.invalid/'; document.head.appendChild(base);
    const blocked = await fetch('https://blocked.invalid/exfiltrate').then(() => false, () => true);
    await new Promise(resolve => setTimeout(resolve,100));
    script.remove(); base.remove();
    document.getElementById('togglePassword').click();
    const visible = getComputedStyle(document.getElementById('eyeClosed')).display !== 'none' && document.getElementById('password').type === 'text';
    document.getElementById('togglePassword').click();
    return {injected:!!window.__injected,blocked,violations,visible};
  })()`);
  assert.equal(policy.injected, false); assert.equal(policy.blocked, true); assert.equal(policy.visible, true);
  for (const directive of ['script-src-elem','base-uri','connect-src']) assert(policy.violations.includes(directive));
  console.log('PASS exact server CSP blocks inline script, base injection and external connection; password toggle works');
  await wait(() => a.ev('!!navigator.serviceWorker.controller'), 'first service worker control');
  assert.equal(await a.ev('(await navigator.serviceWorker.getRegistration()).updateViaCache'), 'none');
  assert.equal(await a.ev('(await navigator.serviceWorker.getRegistration()).scope'), origin + '/');
  const manifestResult = await a.send('Page.getAppManifest');
  assert.deepEqual(manifestResult.errors, []);
  const manifest = JSON.parse(manifestResult.data);
  assert.equal(manifest.start_url, '/login.html'); assert.equal(manifest.display, 'standalone');
  for (const icon of manifest.icons) {
    const buffer = Buffer.from(await (await fetch(origin + icon.src)).arrayBuffer());
    assert.equal(buffer.subarray(1, 4).toString(), 'PNG');
    assert.equal(buffer.readUInt32BE(16), Number(icon.sizes.split('x')[0]));
    assert.equal(buffer.readUInt32BE(20), Number(icon.sizes.split('x')[1]));
  }
  const installability = await a.send('Page.getInstallabilityErrors');
  assert.deepEqual(installability.installabilityErrors, []);
  console.log('PASS manifest, PNG sizes, Chromium installability, scope and updateViaCache');

  const expected = ['/offline.html','/ui/tokens.css','/ui/ui.css','/pwa/pwa.css','/pwa/pwa.js',
    '/manifest.webmanifest','/ui/favicon.svg','/icons/icon-192.png','/icons/icon-512.png'].sort();
  function assertSafeCache(all, version) {
    const names = Object.keys(all); assert(names.includes('ulaundry-shell-fixture-' + version), JSON.stringify(all));
    for (const urls of Object.values(all)) {
      assert.deepEqual(urls.map(url => new URL(url).pathname).sort(), expected);
      assert(urls.every(url => !new URL(url).search));
    }
  }
  const sensitive = ['/api/login', '/api/logout', '/api/session', '/api/customers?search=synthetic',
    '/api/pesanan', '/api/pesanan/fixture/struk', '/api/laporan/dashboard', '/api/laporan/orders?format=csv',
    '/api/audit', '/api/tracking/synthetic-token', '/private-receipt'];
  await a.ev(`Promise.all(${JSON.stringify(sensitive)}.map(url => fetch(url, {cache:'no-store'}).then(r => r.text())))`);
  await a.ev(`fetch('/api/pesanan', {method:'POST', body:'{"fixture":true}', cache:'no-store'}).then(r => r.text())`);
  assertSafeCache(await cacheKeys(a), 'A');
  assert.equal(await a.ev(`(await fetch('/unknown-page')).status`), 404);
  console.log('PASS exact nine public cache keys; API/auth/customer/receipt/report/tracking and writes excluded');

  await a.send('Network.emulateNetworkConditions', {offline:true, latency:0, downloadThroughput:0, uploadThroughput:0});
  outage = true;
  const writesBefore = trace.filter(item => item.method === 'POST').length;
  for (const route of ['/index.html','/pos.html','/customers.html','/dashboard.html','/tracking.html?token=synthetic-secret','/login.html']) {
    await nav(a, route);
    assert.equal(await a.ev('document.body.hasAttribute("data-offline-shell")'), true);
    assert.equal(await a.ev('document.body.textContent.includes("SYNTHETIC_PRIVATE_DATA")'), false);
    assert.equal(await a.ev('navigator.onLine'), false);
  }
  for (const url of sensitive) assert.equal(await a.ev(`fetch(${JSON.stringify(url)}).then(() => false, () => true)`), true);
  assert.equal(await a.ev(`fetch('/api/pesanan',{method:'POST',body:'{}'}).then(() => false, () => true)`), true);
  assert.equal(await a.ev(`(await fetch('/offline.html')).headers.get('Cache-Control')`), 'no-store');
  assert.equal(await a.ev(`fetch('/pwa/pwa.js',{headers:{Authorization:'Bearer synthetic'}}).then(()=>false,()=>true)`), true);
  assert.equal(await a.ev(`fetch('/ui/ui.css?private=synthetic').then(()=>false,()=>true)`), true);
  await screenshot(a, 'offline-desktop');
  assertSafeCache(await cacheKeys(a), 'A');
  console.log('PASS offline navigation shell and sensitive GET/POST network failures without cache fallback');
  outage = false;
  await a.send('Network.emulateNetworkConditions', {offline:false, latency:0, downloadThroughput:-1, uploadThroughput:-1});
  await wait(() => a.ev('navigator.onLine && document.getElementById("offline-status").textContent.includes("Pilih Coba kembali")'), 'online recovery notice');
  assert.equal(await a.ev('document.body.hasAttribute("data-offline-shell")'), true);
  assert.equal(trace.filter(item => item.method === 'POST').length, writesBefore);
  await a.ev('document.getElementById("pwa-retry").click()');
  await wait(() => a.ev('!!document.getElementById("formLogin")'), 'manual recovery');
  navigationMarker = '<p id="new-deploy">Fresh deployment</p>';
  assetRelease = 'A-assets-only';
  // Already-controlled page must bypass a previously fresh one-year JS HTTP cache.
  await nav(a, '/login.html'); assert.equal(await a.ev('!!document.getElementById("new-deploy")'), true);
  assert.equal(await a.ev('window.__pwaFixtureRelease'), 'A-assets-only');
  assetRelease = 'A';
  navigationMarker = '';
  navigationStatus = 503; await nav(a, '/login.html'); assert.equal(await a.ev('document.body.hasAttribute("data-offline-shell")'), true);
  navigationStatus = 403; await nav(a, '/login.html'); assert.equal(await a.ev('document.body.hasAttribute("data-offline-shell")'), false);
  navigationStatus = 200; await nav(a, '/login.html');
  stalledNavigation = true; await nav(a, '/login.html');
  assert.equal(await a.ev('document.body.hasAttribute("data-offline-shell")'), true);
  stalledNavigation = false; await nav(a, '/login.html');
  console.log('PASS manual online recovery, no write replay, fresh HTML/JS despite HTTP cache, 503/timeout shell and preserved 403');

  // Installation UI wiring without performing a native OS installation.
  await a.ev(`(()=>{window.installPromptCount=0; const event = new Event('beforeinstallprompt',{cancelable:true}); event.prompt=async()=>{window.installPromptCount++}; event.userChoice=Promise.resolve({outcome:'dismissed'}); dispatchEvent(event);})()`);
  assert.equal(await a.ev(`${buttons('Pasang aplikasi')}.hidden`), false);
  await click(a, 'Pasang aplikasi'); assert.equal(await a.ev('window.installPromptCount'), 1);
  assert.equal(await a.ev(`${buttons('Pasang aplikasi')}.hidden`), true);

  const {targetId} = await a.send('Target.createTarget', {url:'about:blank'});
  const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const b = await connect(targets.find(item => item.id === targetId).webSocketDebuggerUrl);
  await nav(b, '/login.html');
  await a.ev('document.getElementById("username").value="draft-A"');
  await b.ev('document.getElementById("username").value="draft-B"');
  release = 'B';
  await a.ev('(await navigator.serviceWorker.getRegistration()).update()');
  await wait(() => a.ev('!!(await navigator.serviceWorker.getRegistration()).waiting'), 'waiting update B');
  await wait(() => a.ev(`${buttons('Aktifkan versi baru')}.hidden === false`), 'update notification A');
  await wait(() => b.ev(`${buttons('Aktifkan versi baru')}.hidden === false`), 'update notification B');
  assert.equal(await a.ev('document.getElementById("username").value'), 'draft-A');
  assert.equal(await a.ev('(await navigator.serviceWorker.getRegistration()).active.state'), 'activated');
  assert(Object.keys(await cacheKeys(a)).includes('ulaundry-shell-fixture-A'));
  await screenshot(a, 'update-waiting');
  await click(a, 'Aktifkan versi baru');
  await wait(() => a.ev(`${buttons('Muat ulang halaman')}.hidden === false`), 'activation notification');
  await wait(() => b.ev(`${buttons('Muat ulang halaman')}.hidden === false`), 'other tab activation notification');
  await wait(async () => Object.keys(await cacheKeys(a)).length === 1, 'old cache cleanup');
  assertSafeCache(await cacheKeys(a), 'B');
  assert.equal(await a.ev('document.getElementById("username").value'), 'draft-A');
  assert.equal(await b.ev('document.getElementById("username").value'), 'draft-B');
  await screenshot(a, 'update-active');
  await click(a, 'Muat ulang halaman');
  await wait(() => a.ev('document.getElementById("username")?.value === ""'), 'manual updated reload');
  console.log('PASS waiting update notification, explicit activation, two preserved tabs/drafts, old cache cleanup and manual reload');

  release = 'C'; brokenInstall = true;
  await a.ev('(await navigator.serviceWorker.getRegistration()).update()');
  await wait(() => a.ev('document.querySelector(".pwa-panel").textContent.includes("belum dapat disiapkan")'), 'failed install notification');
  await wait(async () => !Object.keys(await cacheKeys(a)).includes('ulaundry-shell-fixture-C'), 'failed cache cleanup');
  assertSafeCache(await cacheKeys(a), 'B');
  brokenInstall = false;
  await click(a, 'Periksa pembaruan');
  await wait(() => a.ev('!!(await navigator.serviceWorker.getRegistration()).waiting'), 'retry install C');
  await click(a, 'Aktifkan versi baru');
  await wait(async () => { const all = await cacheKeys(a); return Object.keys(all).length === 1 && !!all['ulaundry-shell-fixture-C']; }, 'recovered installation');
  assertSafeCache(await cacheKeys(a), 'C');
  // A rollback also carries a unique worker version and uses the same lifecycle.
  release = 'rollback-D'; await a.ev('(await navigator.serviceWorker.getRegistration()).update()');
  await wait(() => a.ev('!!(await navigator.serviceWorker.getRegistration()).waiting'), 'rollback update');
  await click(a, 'Aktifkan versi baru');
  await wait(async () => { const all = await cacheKeys(a); return Object.keys(all).length === 1 && !!all['ulaundry-shell-fixture-rollback-D']; }, 'rollback activation');
  assertSafeCache(await cacheKeys(a), 'rollback-D');
  console.log('PASS failed install keeps prior worker/cache; retry and unique-version rollback recover');

  await nav(a, '/login.html');
  for (const width of [1280, 360, 390, 412]) {
    await a.send('Emulation.setDeviceMetricsOverride', {width, height:900, deviceScaleFactor:1, mobile:width < 768});
    await a.ev('document.querySelector(".pwa-panel").open=true');
    assert.equal(await a.ev('document.documentElement.scrollWidth <= innerWidth'), true, 'overflow ' + width);
    await screenshot(a, 'panel-' + width);
  }
  await a.send('Emulation.setEmulatedMedia', {media:'print'});
  assert.equal(await a.ev('getComputedStyle(document.querySelector(".pwa-panel")).display'), 'none');
  await a.send('Emulation.setEmulatedMedia', {media:''});
  // Manual escape hatch: scoped unregister/cache deletion, not all origin storage.
  await a.ev(`(async()=>{ for(const r of await navigator.serviceWorker.getRegistrations()) if(r.active?.scriptURL === location.origin+'/sw.js') await r.unregister(); for(const k of await caches.keys()) if(k.startsWith('ulaundry-shell-')) await caches.delete(k); })()`);
  await nav(a, '/login.html');
  await wait(() => a.ev('!!(await navigator.serviceWorker.getRegistration())?.active'), 're-register after recovery');
  assert.equal(exceptions.length, 0, JSON.stringify(exceptions));
  console.log('PASS desktop/mobile overflow, print, scoped recovery and no page JavaScript exceptions');
  console.log('PWA browser verification passed. API fixture; no OS install, Android hardware, HTTPS proxy or database acceptance.');
}
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(async () => {
  for (const client of clients) client.ws.close();
  if (edge) {
    edge.kill();
    await Promise.race([new Promise(resolve => edge.once('exit', resolve)), sleep(2000)]);
  }
  server.closeAllConnections(); server.close();
  // Only remove the absolute, task-owned temporary profile, never user profiles.
  if (path.dirname(profile) === path.resolve(os.tmpdir()) && path.basename(profile).startsWith('ulaundry-pwa-')) {
    try { fs.rmSync(profile, {recursive:true, force:true, maxRetries:4, retryDelay:250}); } catch (_) { /* Browser profile may remain locked briefly. */ }
  }
});
