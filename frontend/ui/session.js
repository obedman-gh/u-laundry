(() => {
  'use strict';
  const ui = window.ULaundryUI;
  const protectedPage = document.body.hasAttribute('data-staff-app');
  if (protectedPage) { try { localStorage.removeItem('token'); } catch (_) {} }
  const main = document.querySelector('main');
  const content = document.createElement('div');
  const screen = document.createElement('section');
  let current, identity, inFlight, timer, deadline = 0, ended = false, epoch = 0, writes = 0, signingOut = false, suspended = false, forbidden = '', bootstrapFailed = false;
  let channel;
  if (protectedPage) {
    content.id = 'private-content'; content.hidden = true;
    while (main.firstChild) content.appendChild(main.firstChild);
    screen.id = 'session-status'; screen.setAttribute('aria-live','polite');
    main.append(screen,content);
    ui.state(screen,{kind:'loading',title:'Memeriksa sesi',description:'Menyiapkan akses staf…'});
    if ('BroadcastChannel' in window) {
      channel = new BroadcastChannel('ulaundry-session');
      channel.onmessage = event => {
        if (event.data === 'signed-out') end('signed-out',false);
        if (event.data === 'signed-in' && !ended) check().catch(() => {});
      };
    }
  }
  const sessionError = () => Object.assign(new Error('Sesi tidak aktif. Silakan masuk kembali.'),{status:401});
  function scrub() {
    document.querySelectorAll('input[type="password"]').forEach(field => { field.value = ''; });
  }
  function header() {
    const button = document.getElementById('logout');
    if (button) button.disabled = ended || !current || signingOut || writes > 0;
  }
  function publish(kind) {
    if (channel) channel.postMessage(kind);
    else if ('BroadcastChannel' in window) { const once = new BroadcastChannel('ulaundry-session'); once.postMessage(kind); once.close(); }
  }
  function end(reason = 'expired', broadcast = false) {
    if (ended) return;
    ended = true; epoch++; current = undefined; clearTimeout(timer); scrub();
    if (protectedPage) {
      content.hidden = true;
      content.querySelectorAll('input,textarea').forEach(field => { field.value = ''; });
      content.querySelectorAll('tbody,ul.data-list,.order-list').forEach(region => region.replaceChildren());
      ui.clearSession();
    }
    header();
    if (broadcast) publish('signed-out');
    const allowed = ['expired','signed-out','password-reset','changed'];
    window.location.replace('/login.html?reason='+(allowed.includes(reason)?reason:'expired'));
  }
  function unavailable() {
    if (!protectedPage || ended) return;
    content.hidden = true; screen.hidden = false;
    ui.state(screen,{kind:navigator.onLine?'error':'offline',title:'Sesi belum dapat diperiksa',description:'Periksa koneksi lalu coba kembali. Perubahan tidak dikirim ulang otomatis.',retry:async () => {
      try { const hadSession = Boolean(identity); await check(); if (!hadSession && !ended) window.location.reload(); } catch (_) {}
    }});
  }
  function restore() {
    if (!protectedPage || ended || suspended || document.visibilityState === 'hidden') return;
    if (forbidden) { forbid(forbidden); return; }
    if (bootstrapFailed) {
      content.hidden = true; screen.hidden = false;
      ui.state(screen,{title:'Sesi dapat digunakan',description:'Muat ulang halaman untuk mengambil data terbaru.',retry:() => window.location.reload()});
      return;
    }
    screen.hidden = true; content.hidden = false;
  }
  // Only session and logout requests have a UI timeout. Business writes retain
  // their existing uncertainty/idempotency handling and are never retried here.
  async function authRequest(path,options = {},readBody = false) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(),12000);
    try {
      const response = await fetch(path,{...options,credentials:'same-origin',cache:'no-store',mode:'same-origin',redirect:'error',signal:controller.signal});
      const data = readBody && response.ok ? await response.json() : undefined;
      return {response,data};
    }
    finally { clearTimeout(timeout); }
  }
  async function check() {
    if (ended) throw sessionError();
    if (inFlight) return inFlight;
    const version = epoch;
    if (protectedPage) { content.hidden = true; screen.hidden = false; ui.state(screen,{kind:'loading',title:'Memeriksa sesi',description:'Memastikan akses staf masih aktif…'}); }
    inFlight = (async () => {
      try {
        const {response,data} = await authRequest('/api/session',{},true);
        if (ended || version !== epoch) throw sessionError();
        if (response.status === 401) { if (protectedPage) end('expired'); throw sessionError(); }
        if (!response.ok) throw new Error('Sesi belum dapat diperiksa.');
        if (ended || version !== epoch) throw sessionError();
        const expires = Date.parse(data.expires_at);
        if (typeof data.user_id !== 'string' || !data.user_id || typeof data.username !== 'string' || !['owner','admin','cashier'].includes(data.role) || typeof data.csrf_token !== 'string' || !data.csrf_token || !Number.isFinite(expires)) throw new Error('Respons sesi belum dapat digunakan.');
        const next = Object.freeze({user_id:data.user_id,username:data.username,role:data.role,csrf_token:data.csrf_token,expires_at:data.expires_at});
        if (protectedPage && identity && (identity.user_id !== next.user_id || identity.role !== next.role || identity.csrf_token !== next.csrf_token)) { end('changed'); throw sessionError(); }
        const serverDate = Date.parse(response.headers.get('Date'));
        const remaining = Math.max(0,expires-(Number.isFinite(serverDate)?serverDate:Date.now()));
        if (!remaining) { if (protectedPage) end('expired'); throw sessionError(); }
        identity = next; current = next;
        deadline = Date.now()+remaining;
        clearTimeout(timer);
        if (protectedPage) timer = setTimeout(() => check().catch(() => {}),Math.min(remaining+100,2147483000));
        ui.setSession(next); header(); restore(); return next;
      } catch (error) {
        if (!ended) { if (!identity) bootstrapFailed = true; current = undefined; header(); unavailable(); }
        throw error;
      } finally { inFlight = undefined; }
    })();
    return inFlight;
  }
  async function get() {
    if (ended) throw sessionError();
    if (inFlight) return inFlight;
    if (current && Date.now() < deadline) return current;
    return check();
  }
  async function request(path,options = {}) {
    const url = new URL(path,window.location.origin);
    if (!path.startsWith('/api/') || url.origin !== window.location.origin || !url.pathname.startsWith('/api/')) throw new Error('Tujuan permintaan tidak valid.');
    const active = await get(), version = epoch;
    const method = (options.method || 'GET').toUpperCase();
    const mutation = !['GET','HEAD'].includes(method), headers = new Headers(options.headers);
    if (headers.has('Authorization')) throw new Error('Metode autentikasi tidak didukung.');
    if (mutation) headers.set('X-CSRF-Token',active.csrf_token);
    if (mutation) { writes++; header(); }
    try {
      const response = await fetch(path,{...options,method,headers,credentials:'same-origin',cache:'no-store',mode:'same-origin',redirect:'error'});
      if (ended || version !== epoch) throw sessionError();
      if (response.status === 401) { end('expired'); throw sessionError(); }
      return response;
    } finally { if (mutation) { writes--; header(); } }
  }
  function forbid(text = 'Halaman ini hanya dapat digunakan pemilik.') {
    forbidden = text; scrub();
    if (!protectedPage || ended) return;
    content.hidden = true; screen.hidden = false;
    ui.state(screen,{title:'Akses tidak diizinkan',description:text});
    const link = document.createElement('a'); link.href = '/index.html'; link.textContent = 'Kembali ke antrean'; link.className = 'button secondary'; screen.appendChild(link);
  }
  async function logout() {
    if (ended || signingOut) return;
    if (writes) { ui.notify('Tunggu permintaan yang sedang diproses sebelum keluar.','info'); return; }
    signingOut = true; header(); scrub(); content.hidden = true; screen.hidden = false;
    ui.state(screen,{kind:'loading',title:'Mengakhiri sesi',description:'Menunggu konfirmasi keluar…'});
    try {
      const active = await get();
      const {response} = await authRequest('/api/logout',{method:'POST',headers:{'X-CSRF-Token':active.csrf_token}});
      if (!response.ok && response.status !== 401) throw new Error('Keluar belum dapat dikonfirmasi.');
      end('signed-out',true);
    } catch (_) {
      if (!ended) {
        content.hidden = true; screen.hidden = false;
        ui.state(screen,{kind:'error',title:'Keluar belum dapat dikonfirmasi',description:'Periksa koneksi lalu pilih Coba kembali. Sesi tidak dianggap berakhir sebelum server mengonfirmasinya.',retry:() => logout()});
      }
    } finally { signingOut = false; header(); }
  }
  if (protectedPage) {
    document.getElementById('logout').addEventListener('click',logout);
    document.addEventListener('visibilitychange',() => {
      if (document.visibilityState === 'hidden') { content.hidden = true; scrub(); }
      else if (!ended) check().catch(() => {});
    });
    // A native confirmation dialog can return focus while its write is being
    // sent. That request already verifies the session on the server.
    window.addEventListener('focus',() => { if (!ended && !suspended && !writes && !signingOut) check().catch(() => {}); });
    window.addEventListener('pagehide',() => { suspended = true; content.hidden = true; scrub(); });
    window.addEventListener('pageshow',event => { suspended = false; if (event.persisted && !ended) check().catch(() => {}); });
    window.addEventListener('online',() => { if (!ended && !current) check().catch(() => {}); });
  }
  window.ULaundrySession = Object.freeze({get,recheck:check,fetch:request,logout,end,forbid,announceLogin:() => publish('signed-in')});
})();
