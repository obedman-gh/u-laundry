(() => {
  const element = id => document.getElementById(id);
  const message = text => { element('message').textContent = text; };
  let session, selected, offset = 0, busy = false;
  const limit = 20;
  async function api(path, options = {}) {
    const response = await fetch(path, { credentials: 'same-origin', ...options,
      headers: { ...(options.body ? {'Content-Type':'application/json'} : {}), ...(options.method ? {'X-CSRF-Token':session.csrf_token} : {}), ...options.headers } });
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { selected = null; element('management').hidden = true; window.location.href = '/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (!response.ok) throw new Error(data?.error?.message || 'Permintaan tidak dapat diproses.');
    return data;
  }
  function select(record) {
    selected = record;
    element('selected').textContent = record ? `${record.name} (${record.username}), versi ${record.version}` : 'Pilih akun dari daftar.';
    if (record) element('edit-role').value = record.role;
    const self = record && record.id === session.user_id;
    element('role-button').disabled = !record || busy || self;
    element('active-button').disabled = !record || busy || self;
    element('password-button').disabled = !record || busy;
    element('active-button').textContent = record?.active ? 'Nonaktifkan akun' : 'Aktifkan akun';
  }
  function setBusy(value) { busy = value; element('reload').disabled = value; element('create-button').disabled = value; element('previous').disabled = value || offset === 0; element('next').disabled = value || element('next').dataset.last === 'true'; select(selected); }
  async function load() {
    setBusy(true); select(null);
    try {
    const records = await api(`/api/staff?limit=${limit}&offset=${offset}`);
    if (!Array.isArray(records)) throw new Error('Respons daftar staf tidak valid.');
    element('staff-list').replaceChildren(); select(null);
    for (const record of records) {
      const row = document.createElement('tr');
      for (const text of [`${record.name} / ${record.username}`, record.role, `${record.active ? 'Aktif' : 'Tidak aktif'} / ${record.version}`]) {
        const cell = document.createElement('td'); cell.textContent = text; row.appendChild(cell);
      }
      const cell = document.createElement('td'), button = document.createElement('button');
      button.type = 'button'; button.textContent = 'Pilih'; button.addEventListener('click', () => { if (!busy) select(record); }); cell.appendChild(button); row.appendChild(cell); element('staff-list').appendChild(row);
    }
    element('page-info').textContent = records.length ? `Menampilkan ${offset + 1}–${offset + records.length}` : 'Tidak ada staf pada halaman ini.';
    element('next').dataset.last = String(records.length < limit);
    } finally { setBusy(false); }
  }
  async function change(operation, payload) {
    if (!selected || busy) return;
    const record = selected;
    if (!window.confirm('Simpan perubahan? Perubahan akses atau password mencabut sesi akun terkait.')) return;
    setBusy(true);
    try {
      await api(`/api/staff/${encodeURIComponent(record.id)}/${operation}`, {method:operation === 'reset-password' ? 'POST' : 'PUT',body:JSON.stringify({...payload,version:record.version})});
      if (operation === 'reset-password' && record.id === session.user_id) { element('management').hidden = true; window.location.href = '/login.html'; return; }
      await load(); message('Perubahan akun berhasil disimpan.');
    } catch (error) { select(null); message(`${error.message} Muat ulang daftar sebelum mencoba kembali.`); }
    finally { setBusy(false); }
  }
  element('create-form').addEventListener('submit', async event => {
    event.preventDefault(); if (busy) return;
    const password = element('create-password').value;
    if (password !== element('create-confirm').value) { message('Konfirmasi password tidak cocok.'); return; }
    const payload = {name:element('create-name').value,username:element('create-username').value,role:element('create-role').value,password};
    element('create-password').value = ''; element('create-confirm').value = ''; setBusy(true);
    try { await api('/api/staff',{method:'POST',body:JSON.stringify(payload)}); offset = 0; await load(); message('Akun baru berhasil dibuat.'); }
    catch (error) { message(error.message); } finally { setBusy(false); }
  });
  element('role-form').addEventListener('submit', event => { event.preventDefault(); change('role',{role:element('edit-role').value}); });
  element('active-button').addEventListener('click', () => { if (selected) change('active',{active:!selected.active}); });
  element('password-form').addEventListener('submit', event => {
    event.preventDefault(); const password = element('edit-password').value;
    if (password !== element('edit-confirm').value) { message('Konfirmasi password tidak cocok.'); return; }
    element('edit-password').value = ''; element('edit-confirm').value = ''; change('reset-password',{password});
  });
  element('reload').addEventListener('click', async () => { if (busy) return; try { await load(); message('Daftar staf diperbarui.'); } catch(error) { message(error.message); } });
  element('previous').addEventListener('click', async () => { if (busy || offset === 0) return; offset = Math.max(0,offset-limit); try {await load();} catch(error){message(error.message);} });
  element('next').addEventListener('click', async () => { if (busy || element('next').dataset.last === 'true') return; offset += limit; try {await load();} catch(error){message(error.message);} });
  (async () => {try {session = await api('/api/session'); if (session.role !== 'owner') {message('Halaman ini hanya dapat digunakan owner.');return;} element('management').hidden = false; await load(); message('');} catch(error){message(error.message);}})();
})();
