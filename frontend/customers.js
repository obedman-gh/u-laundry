(() => {
  'use strict';
  const el = id => document.getElementById(id);
  let csrf = '', selected = null, offset = 0, orderOffset = 0, sequence = 0, busy = false;
  const message = text => { el('message').textContent = text; };
  async function api(path, options = {}) {
    const response = await fetch(path, { credentials: 'same-origin', ...options });
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { window.location.href = '/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (!response.ok) {
      if (response.status === 409) reset();
      throw new Error(data?.error?.message || 'Permintaan gagal. Silakan muat ulang sebelum mencoba kembali.');
    }
    return data;
  }
  function reset() {
    sequence++; selected = null; orderOffset = 0;
    for (const key of ['name','contact','address','notes']) el(key).value = '';
    el('form-title').textContent = 'Tambah pelanggan'; el('selected').textContent = 'Pilih pelanggan untuk melihat riwayat.';
    el('orders').replaceChildren(); el('orders-previous').disabled = el('orders-next').disabled = true;
  }
  async function history(ticket) {
    const id = selected?.customer_id; if (!id) return;
    const orders = await api(`/api/customers/${encodeURIComponent(id)}/orders?limit=20&offset=${orderOffset}`);
    if (ticket !== sequence) return;
    el('orders').replaceChildren();
    for (const order of orders) {
      const row = document.createElement('li');
      row.textContent = `${order.id_pesanan} — ${order.status} — Rp ${order.total_biaya} — ${order.payment_status} `;
      const receipt = document.createElement('a'); receipt.textContent = 'Struk internal'; receipt.href = `/api/pesanan/${encodeURIComponent(order.id_pesanan)}/struk`;
      row.appendChild(receipt); el('orders').appendChild(row);
      const rotate = document.createElement('button'); rotate.type = 'button'; rotate.textContent = 'Buat tautan tracking baru';
      const share = document.createElement('span');
      rotate.addEventListener('click', async () => {
        if (busy || ticket !== sequence) return;
        busy = true; rotate.disabled = true; share.replaceChildren();
        try {
          const data = await api(`/api/pesanan/${encodeURIComponent(order.id_pesanan)}/tracking`, {method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify({version:order.version})});
          if (ticket !== sequence) return;
          order.version = data.version;
          const link = document.createElement('a'); link.textContent = 'Buka / salin tautan tracking'; link.href = '/tracking.html#' + data.tracking_token; link.rel = 'noreferrer';
          share.appendChild(link); message('Tautan lama dicabut. Tautan baru berlaku 90 hari; salin sebelum meninggalkan halaman.');
        } catch (error) { message(error.message || 'Rotasi belum dapat dikonfirmasi. Muat ulang riwayat sebelum mencoba kembali.'); }
        finally { busy = false; rotate.disabled = false; }
      });
      row.appendChild(rotate); row.appendChild(share);
    }
    if (!orders.length) el('orders').textContent = 'Belum ada pesanan pada halaman ini.';
    el('orders-previous').disabled = orderOffset === 0; el('orders-next').disabled = orders.length < 20;
  }
  async function choose(id) {
    if (busy) return;
    reset(); const ticket = sequence;
    try {
      message('Memuat pelanggan…');
      const data = await api(`/api/customers/${encodeURIComponent(id)}`); if (ticket !== sequence) return;
      selected = data;
      for (const key of ['name','contact','address','notes']) el(key).value = data[key] || '';
      el('form-title').textContent = 'Ubah pelanggan'; el('selected').textContent = data.name;
      await history(ticket); if (ticket === sequence) message('Data pelanggan dimuat.');
    } catch (error) { if (ticket === sequence) message(error.message || 'Tidak terhubung. Silakan coba kembali.'); }
  }
  async function search() {
    message('Memuat daftar pelanggan…');
    const q = el('search').value.trim();
    const data = await api(`/api/customers?limit=20&offset=${offset}${q ? '&q=' + encodeURIComponent(q) : ''}`);
    el('customers').replaceChildren();
    for (const customer of data) {
      const row = document.createElement('li'), button = document.createElement('button'); button.type = 'button';
      button.textContent = customer.name + (customer.contact ? ` — ${customer.contact}` : '');
      button.addEventListener('click', () => choose(customer.customer_id)); row.appendChild(button); el('customers').appendChild(row);
    }
    if (!data.length) el('customers').textContent = 'Tidak ada pelanggan yang cocok.';
    el('previous').disabled = offset === 0; el('next').disabled = data.length < 20; el('page-info').textContent = `Halaman ${offset / 20 + 1}`;
    message('Daftar pelanggan dimuat.');
  }
  async function run(fn) { try { await fn(); } catch (error) { message(error.message || 'Tidak terhubung. Silakan coba kembali.'); } }
  el('search-form').addEventListener('submit', event => { event.preventDefault(); offset = 0; run(search); });
  el('previous').addEventListener('click', () => { offset = Math.max(0,offset-20); run(search); });
  el('next').addEventListener('click', () => { offset += 20; run(search); });
  el('new').addEventListener('click', () => { if (!busy) { reset(); message('Form pelanggan baru.'); } });
  for (const [id, delta] of [['orders-previous',-20],['orders-next',20]]) el(id).addEventListener('click', () => { orderOffset = Math.max(0,orderOffset+delta); run(() => history(++sequence)); });
  el('customer-form').addEventListener('submit', async event => {
    event.preventDefault(); if (busy || !csrf) return;
    busy = true; el('save').disabled = true;
    const current = selected, body = {};
    for (const key of ['name','contact','address','notes']) body[key] = el(key).value;
    if (current) body.version = current.version;
    try {
      const saved = await api(current ? `/api/customers/${encodeURIComponent(current.customer_id)}` : '/api/customers', { method: current ? 'PUT' : 'POST', headers: {'Content-Type':'application/json','X-CSRF-Token':csrf}, body:JSON.stringify(body) });
      reset(); offset = 0; await search(); busy = false; await choose(saved.customer_id); message('Pelanggan tersimpan.');
    } catch (error) { message(error.message || 'Tidak terhubung. Periksa daftar sebelum mengirim ulang.'); }
    finally { busy = false; el('save').disabled = false; }
  });
  run(async () => { const session = await api('/api/session'); csrf = session.csrf_token; el('management').hidden = false; await search(); });
})();
