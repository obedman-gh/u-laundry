(() => {
  'use strict';
  const el = id => document.getElementById(id);
  let csrf = '', selected = null, offset = 0, historyOffset = 0, sequence = 0, listSequence = 0, busy = false;
  const message = text => { el('message').textContent = text; };
  async function api(path, options = {}) {
    const response = await fetch(path, {credentials:'same-origin', ...options});
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { window.location.href = '/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (!response.ok) {
      if (response.status === 409) reset();
      throw new Error(data?.error?.message || 'Permintaan gagal. Muat ulang sebelum mencoba kembali.');
    }
    return data;
  }
  function reset() {
    sequence++; selected = null; historyOffset = 0;
    for (const id of ['name','price','minutes']) el(id).value = '';
    el('unit').value = 'kg'; el('active').value = 'true'; el('display-order').value = '0';
    el('form-title').textContent = 'Tambah layanan'; el('selected').textContent = 'Pilih layanan untuk melihat histori.';
    el('history').replaceChildren(); el('history-previous').disabled = el('history-next').disabled = true;
  }
  async function history(ticket) {
    if (!selected) return;
    const records = await api(`/api/services/${encodeURIComponent(selected.service_id)}/history?limit=20&offset=${historyOffset}`);
    if (ticket !== sequence) return;
    el('history').replaceChildren();
    for (const record of records) {
      const row = document.createElement('li');
      const source = record.source === 'baseline' ? 'Kondisi awal tercatat' : record.source === 'demo' ? 'Data contoh' : 'Perubahan staf';
      row.textContent = `Versi ${record.version}: ${record.name}, Rp ${record.unit_price}/${record.unit}, ${record.turnaround_minutes} menit, ${record.active ? 'aktif' : 'nonaktif'}, urutan ${record.display_order}. ${source} — ${new Date(record.changed_at).toLocaleString('id-ID', {timeZone:'Asia/Makassar'})} WITA`;
      el('history').appendChild(row);
    }
    if (!records.length) el('history').textContent = 'Belum ada histori pada halaman ini.';
    el('history-previous').disabled = historyOffset === 0; el('history-next').disabled = records.length < 20;
  }
  async function choose(id) {
    if (busy) return; reset(); const ticket = sequence;
    try {
      message('Memuat layanan…'); const record = await api('/api/services/' + encodeURIComponent(id));
      if (ticket !== sequence) return; selected = record;
      for (const [id,key] of [['name','name'],['unit','unit'],['price','unit_price'],['minutes','turnaround_minutes'],['display-order','display_order']]) el(id).value = String(record[key]);
      el('active').value = String(record.active); el('form-title').textContent = 'Ubah layanan'; el('selected').textContent = record.name;
      await history(ticket); if (ticket === sequence) message('Data layanan dimuat.');
    } catch (error) { if (ticket === sequence) message(error.message || 'Tidak terhubung. Silakan coba kembali.'); }
  }
  async function list() {
    const ticket = ++listSequence; message('Memuat daftar layanan…');
    const records = await api(`/api/services?include_inactive=true&limit=20&offset=${offset}`);
    if (ticket !== listSequence) return;
    el('services').replaceChildren();
    for (const record of records) {
      const row = document.createElement('li'), button = document.createElement('button'); button.type = 'button';
      button.textContent = `${record.display_order}. ${record.name} — Rp ${record.unit_price}/${record.unit} — ${record.active ? 'Aktif' : 'Nonaktif'}`;
      button.addEventListener('click', () => choose(record.service_id)); row.appendChild(button); el('services').appendChild(row);
    }
    if (!records.length) el('services').textContent = 'Belum ada layanan pada halaman ini.';
    el('previous').disabled = offset === 0; el('next').disabled = records.length < 20; el('page-info').textContent = `Halaman ${offset/20+1}`;
    message('Daftar layanan dimuat.');
  }
  function integer(id, min, max) {
    const raw = el(id).value, value = Number(raw);
    if (!/^\d+$/.test(raw) || !Number.isSafeInteger(value) || value < min || value > max) throw new Error('Harga, durasi, dan urutan wajib bilangan bulat sesuai batas kolom.');
    return value;
  }
  async function run(fn) { try { await fn(); } catch (error) { message(error.message || 'Tidak terhubung. Silakan coba kembali.'); } }
  el('new').addEventListener('click', () => { if (!busy) { reset(); message('Form layanan baru.'); } });
  el('reload').addEventListener('click', () => { if (!busy) { reset(); run(list); } });
  el('previous').addEventListener('click', () => { offset = Math.max(0,offset-20); run(list); });
  el('next').addEventListener('click', () => { offset += 20; run(list); });
  for (const [id,delta] of [['history-previous',-20],['history-next',20]]) el(id).addEventListener('click', () => { historyOffset = Math.max(0,historyOffset+delta); run(() => history(++sequence)); });
  el('service-form').addEventListener('submit', async event => {
    event.preventDefault(); if (busy || !csrf) return;
    busy = true; el('save').disabled = true;
    try {
      const current = selected;
      const body = {name:el('name').value,unit:el('unit').value,unit_price:integer('price',0,Number.MAX_SAFE_INTEGER),turnaround_minutes:integer('minutes',1,153722867),active:el('active').value === 'true',display_order:integer('display-order',0,1000000)};
      if (current) body.version = current.version;
      const saved = await api(current ? '/api/services/' + encodeURIComponent(current.service_id) : '/api/services', {method:current ? 'PUT' : 'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify(body)});
      reset(); offset = 0; await list(); busy = false; await choose(saved.service_id); message('Layanan tersimpan. Pesanan lama tetap memakai snapshot semula.');
    } catch (error) { message(error.message || 'Tidak terhubung. Periksa daftar sebelum mengirim ulang.'); }
    finally { busy = false; el('save').disabled = false; }
  });
  run(async () => {
    const session = await api('/api/session');
    if (session.role !== 'owner' && session.role !== 'admin') { message('Halaman ini hanya untuk owner atau admin.'); return; }
    csrf = session.csrf_token; el('management').hidden = false; await list();
  });
})();
