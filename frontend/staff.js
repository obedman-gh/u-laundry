(() => {
  'use strict';
  const el = id => document.getElementById(id), ui = window.ULaundryUI, auth = window.ULaundrySession;
  const roles = {owner:'Pemilik',admin:'Admin',cashier:'Kasir'};
  let session, selected, offset = 0, busy = false, canManage = false, needsReload = false, last = true;
  const limit = 20;
  const passwordFields = ['create-password','create-confirm','edit-password','edit-confirm'];
  function message(text,kind = 'info') { ui.message(el('message'),text,kind); }
  function clearPasswords() { for (const id of passwordFields) el(id).value = ''; }
  function validationClear() {
    for (const field of document.querySelectorAll('[data-staff-field]')) {
      field.removeAttribute('aria-invalid'); const error = el(field.id+'-error'); error.textContent = ''; error.hidden = true;
    }
  }
  function invalid(id,text) {
    el(id).setAttribute('aria-invalid','true'); const error = el(id+'-error'); error.textContent = text; error.hidden = false;
  }
  function passwordError(value) {
    if (Array.from(value).length < 12 || new TextEncoder().encode(value).length > 72 || value.trim() !== value) return 'Gunakan minimal 12 karakter, maksimal 72 byte, tanpa spasi di awal/akhir.';
    if (/[\p{Cc}\p{Cf}\uD800-\uDFFF]/u.test(value)) return 'Kata sandi mengandung karakter yang tidak diizinkan.';
    if (new Set(Array.from(value)).size === 1 || ['123456789012','password1234','password12345','password123!','ulaundry12345','qwerty123456','administrator'].includes(value.toLowerCase())) return 'Gunakan frasa sandi yang tidak umum dan tidak berulang.';
    return '';
  }
  function validatePassword(first,confirm) {
    const error = passwordError(el(first).value);
    if (error) invalid(first,error);
    if (el(first).value !== el(confirm).value) invalid(confirm,'Konfirmasi kata sandi tidak cocok.');
  }
  function finishValidation() {
    const first = document.querySelector('[data-staff-field][aria-invalid="true"]');
    if (!first) return true;
    clearPasswords(); message('Periksa kolom yang ditandai. Kata sandi perlu diisi kembali.','error'); first.focus(); return false;
  }
  function select(record,focus = false) {
    selected = record; clearPasswords(); validationClear();
    el('selected').textContent = record ? record.name+' ('+record.username+'), versi '+record.version : 'Pilih akun dari daftar.';
    if (record) el('edit-role').value = record.role;
    for (const button of document.querySelectorAll('[data-select-staff]')) {
      const chosen = button.dataset.selectStaff === record?.id;
      button.setAttribute('aria-pressed',String(chosen)); button.closest('tr').dataset.selected = String(chosen);
    }
    controls();
    if (record && focus) el('edit-title').focus();
  }
  function controls() {
    const disabled = busy || !canManage;
    el('management').setAttribute('aria-busy',String(busy));
    for (const control of el('management').querySelectorAll('input,select,button')) control.disabled = disabled;
    const self = selected?.id === session?.user_id;
    el('create-button').disabled = disabled || needsReload;
    el('previous').disabled = disabled || offset === 0;
    el('next').disabled = disabled || last;
    el('role-button').disabled = disabled || needsReload || !selected || self || el('edit-role').value === selected.role;
    el('edit-role').disabled = disabled || needsReload || !selected || self;
    el('active-button').disabled = disabled || needsReload || !selected || self;
    el('password-button').disabled = disabled || needsReload || !selected;
    el('edit-password').disabled = el('edit-confirm').disabled = disabled || needsReload || !selected;
    el('active-button').textContent = selected?.active ? 'Nonaktifkan akun' : 'Aktifkan kembali akun';
  }
  function deny() {
    canManage = false; selected = undefined; clearPasswords(); el('staff-list').replaceChildren(); el('management').hidden = true;
    auth.forbid('Pengelolaan akun staf hanya dapat digunakan pemilik aktif.');
  }
  async function api(path,options = {}) {
    const response = await auth.fetch(path,{...options,headers:{...(options.body?{'Content-Type':'application/json'}:{}),...options.headers}});
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      if (response.status === 403 && body?.error?.code === 'forbidden') deny();
      const texts = {400:'Data akun belum memenuhi ketentuan. Periksa nama, peran, dan kata sandi baru.',403:'Permintaan belum dapat diizinkan. Muat ulang sesi sebelum mencoba kembali.',404:'Akun tidak ditemukan. Muat ulang daftar.',409:'Data telah berubah atau akun belum dapat disimpan. Muat ulang daftar sebelum mencoba kembali.',429:'Permintaan terlalu sering. Tunggu sebentar lalu muat ulang daftar.'};
      throw Object.assign(new Error(texts[response.status] || 'Hasil perubahan belum dapat dikonfirmasi. Muat ulang daftar sebelum mencoba kembali.'),{status:response.status});
    }
    return response.json();
  }
  function normalize(record) {
    if (!record || !/^[A-Za-z0-9_.-]{1,100}$/.test(record.id) || typeof record.name !== 'string' || typeof record.username !== 'string' || !Object.keys(roles).includes(record.role) || typeof record.active !== 'boolean' || !Number.isSafeInteger(record.version) || record.version < 1) throw new Error('Respons daftar staf belum dapat digunakan.');
    return {id:record.id,name:record.name,username:record.username,role:record.role,active:record.active,version:record.version};
  }
  async function load() {
    selected = undefined; clearPasswords(); el('staff-list').replaceChildren(); busy = true; controls(); message('Memuat daftar staf…','loading');
    try {
      const data = await api('/api/staff?limit='+limit+'&offset='+offset);
      if (!Array.isArray(data)) throw new Error('Respons daftar staf belum dapat digunakan.');
      const records = data.map(normalize); el('staff-list').replaceChildren();
      for (const record of records) {
        const row = document.createElement('tr');
        for (const text of [record.name+' / '+record.username,roles[record.role],(record.active?'Aktif':'Tidak aktif')+' / '+record.version]) { const cell = document.createElement('td'); cell.textContent = text; row.appendChild(cell); }
        const cell = document.createElement('td'),button = document.createElement('button'); button.type = 'button'; button.textContent = 'Pilih'; button.dataset.selectStaff = record.id; button.setAttribute('aria-label','Pilih akun '+record.username); button.setAttribute('aria-pressed','false');
        button.addEventListener('click',() => { if (canManage && !busy) select(record,true); }); cell.appendChild(button); row.appendChild(cell); el('staff-list').appendChild(row);
      }
      if (!records.length) { const row = document.createElement('tr'),cell = document.createElement('td'); cell.colSpan = 4; cell.textContent = 'Tidak ada akun pada halaman ini.'; row.appendChild(cell); el('staff-list').appendChild(row); }
      last = records.length < limit; needsReload = false;
      el('page-info').textContent = records.length ? 'Menampilkan '+(offset+1)+'–'+(offset+records.length) : 'Tidak ada akun pada halaman ini.';
      select(undefined); message('Daftar staf dimuat.');
    } finally { busy = false; controls(); }
  }
  async function refresh(success) {
    try { await load(); if (success) message(success,'success'); }
    catch (error) { selected = undefined; needsReload = true; controls(); message(success ? success+' Daftar belum dapat diperbarui. Pilih Muat ulang.' : 'Daftar belum dapat dimuat. Periksa koneksi lalu pilih Muat ulang.','error'); }
  }
  async function mutate(path,body,method,success,selfReset = false) {
    if (!canManage || busy || needsReload) return;
    busy = true; controls(); clearPasswords();
    try {
      await api(path,{method,body:JSON.stringify(body)});
      if (selfReset) { auth.end('password-reset',true); return; }
      if (path === '/api/staff') el('create-form').reset();
      busy = false; await refresh(success);
    } catch (error) {
      needsReload = true; selected = undefined;
      message(error.status ? error.message : 'Hasil perubahan belum dapat dikonfirmasi. Periksa koneksi lalu muat ulang daftar sebelum mencoba kembali.','error');
    } finally { busy = false; clearPasswords(); controls(); }
  }
  el('create-form').addEventListener('submit',async event => {
    event.preventDefault(); if (!canManage || busy || needsReload) return;
    validationClear();
    const name = el('create-name').value.trim(),username = el('create-username').value,role = el('create-role').value;
    if (!name || Array.from(name).length > 100 || /[\p{Cc}\p{Cf}]/u.test(name)) invalid('create-name','Isi nama 1–100 karakter tanpa karakter kontrol.');
    if (!/^[A-Za-z0-9_.-]{3,50}$/.test(username)) invalid('create-username','Gunakan 3–50 huruf, angka, titik, garis bawah atau tanda hubung.');
    if (!Object.keys(roles).includes(role)) invalid('create-role','Pilih peran yang tersedia.');
    validatePassword('create-password','create-confirm');
    if (!finishValidation()) return;
    await mutate('/api/staff',{name,username,role,password:el('create-password').value},'POST','Akun baru berhasil dibuat.');
  });
  el('role-form').addEventListener('submit',async event => {
    event.preventDefault(); if (!canManage || busy || needsReload || !selected || selected.id === session.user_id) return;
    validationClear(); const role = el('edit-role').value;
    if (!Object.keys(roles).includes(role)) { invalid('edit-role','Pilih peran yang tersedia.'); finishValidation(); return; }
    if (role === selected.role) return;
    if (!window.confirm('Ubah peran akun '+selected.username+' menjadi '+roles[role]+'? Sesi akun tersebut akan dicabut.')) return;
    await mutate('/api/staff/'+encodeURIComponent(selected.id)+'/role',{role,version:selected.version},'PUT','Peran akun berhasil diubah.');
  });
  el('active-button').addEventListener('click',async () => {
    if (!canManage || busy || needsReload || !selected || selected.id === session.user_id) return;
    if (!window.confirm((selected.active?'Nonaktifkan':'Aktifkan kembali')+' akun '+selected.username+'? Perubahan akses mencabut sesi akun terkait.')) return;
    await mutate('/api/staff/'+encodeURIComponent(selected.id)+'/active',{active:!selected.active,version:selected.version},'PUT',selected.active?'Akun berhasil dinonaktifkan.':'Akun berhasil diaktifkan kembali.');
  });
  el('password-form').addEventListener('submit',async event => {
    event.preventDefault(); if (!canManage || busy || needsReload || !selected) return;
    validationClear(); validatePassword('edit-password','edit-confirm'); if (!finishValidation()) return;
    const self = selected.id === session.user_id;
    if (!window.confirm('Reset kata sandi akun '+selected.username+'? '+(self?'Anda akan diminta masuk kembali.':'Seluruh sesi akun tersebut akan dicabut.'))) { clearPasswords(); return; }
    await mutate('/api/staff/'+encodeURIComponent(selected.id)+'/reset-password',{password:el('edit-password').value,version:selected.version},'POST','Kata sandi baru berhasil disimpan.',self);
  });
  el('edit-role').addEventListener('change',controls);
  el('reload').addEventListener('click',() => { if (canManage && !busy) refresh(); });
  el('previous').addEventListener('click',() => { if (canManage && !busy && offset>0) { offset = Math.max(0,offset-limit); refresh(); } });
  el('next').addEventListener('click',() => { if (canManage && !busy && !last) { offset += limit; refresh(); } });
  (async () => {
    try {
      session = await auth.get(); if (session.role !== 'owner') { deny(); return; }
      canManage = true; el('management').hidden = false; await refresh();
    } catch (_) { clearPasswords(); el('management').hidden = true; }
  })();
})();
