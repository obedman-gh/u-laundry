(() => {
  'use strict';
  try { localStorage.removeItem('token'); } catch (_) {}
  const el = id => document.getElementById(id),form = el('formLogin'),button = el('btnLogin'),password = el('password');
  const ui = window.ULaundryUI,auth = window.ULaundrySession;
  let busy = false, cooldown = 0, cooldownTimer;
  const reasons = {expired:'Sesi telah berakhir. Silakan masuk kembali.', 'signed-out':'Anda telah keluar. Silakan masuk untuk melanjutkan.', 'password-reset':'Kata sandi telah diperbarui. Silakan masuk dengan kata sandi baru.', changed:'Sesi akun berubah. Silakan masuk kembali untuk melanjutkan.'};
  const reason = new URLSearchParams(window.location.search).get('reason');
  if (Object.keys(reasons).includes(reason)) { el('login-notice').hidden = false; ui.message(el('login-notice'),reasons[reason]); }
  function hidePassword() {
    password.type = 'password'; el('eyeOpen').classList.remove('eye-closed'); el('eyeClosed').classList.add('eye-closed');
    el('togglePassword').setAttribute('aria-pressed','false'); el('togglePassword').setAttribute('aria-label','Tampilkan kata sandi'); el('togglePassword').title = 'Tampilkan kata sandi';
  }
  function show(text) { ui.message(el('pesan-error'),text,'error'); password.focus(); }
  el('togglePassword').addEventListener('click',() => {
    const visible = password.type === 'password'; password.type = visible?'text':'password';
    el('eyeOpen').classList.toggle('eye-closed', visible); el('eyeClosed').classList.toggle('eye-closed', !visible);
    el('togglePassword').setAttribute('aria-pressed',String(visible)); el('togglePassword').setAttribute('aria-label',visible?'Sembunyikan kata sandi':'Tampilkan kata sandi'); el('togglePassword').title = visible?'Sembunyikan kata sandi':'Tampilkan kata sandi';
  });
  form.addEventListener('submit',async event => {
    event.preventDefault(); if (busy || Date.now()<cooldown) return;
    const username = el('username').value,secret = password.value;
    password.value = ''; hidePassword();
    el('username').removeAttribute('aria-invalid'); password.removeAttribute('aria-invalid');
    if (!username || Array.from(username).length>50 || !secret || new TextEncoder().encode(secret).length>72) {
      show('Isi nama pengguna dan kata sandi sesuai batas panjang. Kata sandi maksimal 72 byte.'); return;
    }
    busy = true; button.disabled = true; button.textContent = 'Memverifikasi…'; form.setAttribute('aria-busy','true'); el('pesan-error').textContent = '';
    const controller = new AbortController(),timeout = setTimeout(() => controller.abort(),12000);
    let navigating = false;
    try {
      const response = await fetch('/api/login',{method:'POST',credentials:'same-origin',cache:'no-store',mode:'same-origin',redirect:'error',signal:controller.signal,headers:{'Content-Type':'application/json'},body:JSON.stringify({username,password:secret})});
      const data = await response.json().catch(() => ({}));
      if (response.status===401) {
        el('username').setAttribute('aria-invalid','true'); password.setAttribute('aria-invalid','true');
        show('Nama pengguna atau kata sandi tidak valid.'); return;
      }
      if (response.status===429) {
        const raw = Number(response.headers.get('Retry-After')),seconds = Number.isFinite(raw)&&raw>0?Math.min(Math.ceil(raw),900):60;
        cooldown = Date.now()+seconds*1000; clearTimeout(cooldownTimer);
        cooldownTimer = setTimeout(() => { if (!busy) { button.disabled = false; button.textContent = 'Masuk'; } },seconds*1000);
        show('Percobaan masuk terlalu sering. Tunggu '+seconds+' detik sebelum mencoba kembali.'); return;
      }
      if (!response.ok) {
        show(response.status===400?'Periksa format nama pengguna dan kata sandi.':response.status===403?'Permintaan masuk belum dapat diizinkan. Muat ulang halaman lalu coba kembali.':'Layanan masuk sedang tidak tersedia. Silakan coba kembali.'); return;
      }
      if (typeof data.csrf_token!=='string' || !data.csrf_token || !Number.isFinite(Date.parse(data.expires_at))) { show('Sesi belum dapat dikonfirmasi. Silakan coba masuk kembali.'); return; }
      let active;
      try { active = await auth.recheck(); }
      catch (_) { show('Sesi belum dapat digunakan. Pastikan cookie diizinkan dan koneksi tersedia, lalu masuk kembali.'); return; }
      if (active.csrf_token!==data.csrf_token || active.username!==username) { show('Sesi berubah saat masuk. Silakan coba kembali.'); return; }
      auth.announceLogin(); navigating = true; window.location.replace('/index.html');
    } catch (_) { show('Server tidak merespons. Periksa koneksi lalu coba kembali.'); }
    finally {
      clearTimeout(timeout); busy = false; form.setAttribute('aria-busy','false'); password.value = '';
      if (!navigating) { button.disabled = Date.now()<cooldown; button.textContent = button.disabled?'Tunggu sebelum masuk':'Masuk'; }
    }
  });
  document.addEventListener('visibilitychange',() => { if (document.visibilityState==='hidden') { password.value = ''; hidePassword(); } });
  window.addEventListener('pageshow',event => { password.value = ''; hidePassword(); if (event.persisted) { busy = false; button.disabled = Date.now()<cooldown; form.setAttribute('aria-busy','false'); button.textContent = button.disabled?'Tunggu sebelum masuk':'Masuk'; } });
})();
