(() => {
  'use strict';
  const el = id => document.getElementById(id), ui = window.ULaundryUI;
  let session, busy = false;
  async function api(path) {
    const response = await fetch(path,{credentials:'same-origin',cache:'no-store'});
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { window.location.href = '/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (!response.ok) throw new Error(data?.error?.message || 'Ringkasan belum dapat dimuat.');
    return data;
  }
  function count(value) {
    if (!Number.isSafeInteger(value) || value < 0) throw new Error('Angka ringkasan tidak valid. Hubungi pemilik.');
    return value.toLocaleString('id-ID');
  }
  async function load(notify = false) {
    if (busy) return;
    busy = true; el('refresh-dashboard').disabled = true; el('dashboard-content').hidden = true;
    ui.state(el('dashboard-state'),{kind:'loading',title:'Memuat ringkasan',description:'Mengambil data operasional terbaru…'});
    try {
      if (!session) { session = await api('/api/session'); ui.setSession(session); }
      if (!['owner','admin'].includes(session.role)) {
        ui.state(el('dashboard-state'),{title:'Dashboard untuk pemilik dan admin',description:'Gunakan menu Antrean, Pelanggan, atau Kas dan shift untuk pekerjaan kasir.'});
        return;
      }
      const data = await api('/api/laporan/dashboard');
      // Validate before displaying a partial set of financial numbers.
      const orders = count(data.orders.created), payments = ui.money(data.money.payments), receivable = ui.money(data.orders.receivable), shifts = count(data.shifts.active);
      const refunds = ui.money(data.money.refunds), net = ui.money(data.money.net);
      const statuses = [['received','Diterima'],['processing','Diproses'],['ready','Siap diambil'],['overdue','Terlambat']].map(([key,label]) => [label,count(data.orders[key])]);
      const at = new Intl.DateTimeFormat('id-ID',{dateStyle:'medium',timeStyle:'short',timeZone:'Asia/Makassar'}).format(new Date(data.as_of));
      el('metric-orders').textContent = orders; el('metric-payments').textContent = payments; el('metric-receivable').textContent = receivable; el('metric-shifts').textContent = shifts;
      el('updated').textContent = `Diperbarui ${at} WITA.`;
      el('queue-summary').replaceChildren();
      for (const [label,value] of statuses) { const item = document.createElement('li'); item.textContent = `${label}: ${value}`; el('queue-summary').appendChild(item); }
      el('receipts-note').textContent = `Refund hari ini ${refunds}. Penerimaan bersih setelah refund ${net}.`;
      el('dashboard-state').replaceChildren(); el('dashboard-state').setAttribute('aria-busy','false');
      if (data.orders.created === 0 && data.orders.received === 0 && data.orders.processing === 0 && data.orders.ready === 0 && data.money.payments === 0 && data.money.refunds === 0 && data.orders.receivable === 0 && data.shifts.active === 0) ui.state(el('dashboard-state'),{kind:'empty',title:'Ringkasan pesanan dan pembayaran masih kosong',description:'Data akan tampil setelah pesanan, pembayaran, atau shift dicatat.'});
      el('dashboard-content').hidden = false;
      if (notify) ui.notify('Ringkasan berhasil diperbarui.');
    } catch (error) {
      ui.state(el('dashboard-state'),{kind:navigator.onLine?'error':'offline',title:navigator.onLine?'Ringkasan belum dapat dimuat':'Perangkat sedang offline',description:navigator.onLine?(error.message || 'Silakan coba kembali.'):'Hubungkan koneksi lalu muat ulang data.',retry:() => load()});
    } finally { busy = false; el('refresh-dashboard').disabled = false; }
  }
  el('refresh-dashboard').addEventListener('click',() => load(true));
  load();
})();
