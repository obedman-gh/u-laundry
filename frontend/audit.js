(() => {
  'use strict';
  const element = id => document.getElementById(id);
  const actions = {
    login_success:'Login berhasil (sesi terdaftar)', login_failure:'Login ditolak', login_limited:'Batas percobaan login', logout:'Keluar', access_denied:'Akses ditolak', credentials_verified:'Kredensial terverifikasi', staff_logged_in:'Verifikasi kredensial (histori lama)',
    staff_created:'Staf dibuat', staff_bootstrapped:'Owner pertama dibuat', staff_activated:'Staf diaktifkan', staff_deactivated:'Staf dinonaktifkan', staff_role_changed:'Role staf diubah', staff_password_reset:'Password staf direset', staff_updated:'Staf diperbarui',
    service_configured:'Konfigurasi layanan/harga', service_updated:'Layanan diperbarui', order_created:'Pesanan dibuat', order_status_changed:'Status pesanan diubah', order_cancelled:'Pesanan dibatalkan', order_tracking_rotated:'Akses tracking diganti', discount_approved:'Diskon disetujui',
    payment_recorded:'Pembayaran dicatat', payment_refunded:'Refund dicatat', shift_opened:'Shift dibuka', shift_closed:'Shift ditutup', shift_cash_moved:'Mutasi kas', customer_created:'Pelanggan dibuat', customer_updated:'Pelanggan diperbarui', report_generated:'Laporan dibuat'
  };
  for (const [value, text] of Object.entries(actions)) {
    const option = document.createElement('option'); option.value = value; option.textContent = text; element('action').appendChild(option);
  }
  let busy = false, offset = 0, more = false, activeQuery = '';
  const message = (text, kind = 'info') => { const target = element('message'); if (window.ULaundryUI) window.ULaundryUI.message(target, text, kind); else target.textContent = text; };
  const dateFormat = new Intl.DateTimeFormat('id-ID', {timeZone:'Asia/Makassar', dateStyle:'medium', timeStyle:'medium'});
  const day = value => new Date(new Date(value).getTime() + 8*3600000).toISOString().slice(0,10);
  const displayTime = value => `${dateFormat.format(new Date(value))} WITA`;
  function setBusy(value) {
    busy = value;
    element('audit-panel').setAttribute('aria-busy', String(value));
    for (const control of element('filters').querySelectorAll('button, input, select')) control.disabled = value;
    element('previous').disabled = value || offset === 0; element('next').disabled = value || !more; element('reload').disabled = value;
  }
  async function api(path) {
    const response = await fetch(path, {credentials:'same-origin', cache:'no-store'});
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { element('audit-panel').hidden = true; window.location.href = '/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (response.status === 403) element('audit-panel').hidden = true;
    if (!response.ok) throw new Error(data?.error?.message || 'Riwayat audit tidak dapat dimuat.');
    return data;
  }
  function query() {
    const params = new URLSearchParams();
    const from = element('from').value, to = element('to').value;
    if (Boolean(from) !== Boolean(to)) throw new Error('Isi tanggal awal dan akhir berpasangan.');
    if (from && to) {
      if (to < from) throw new Error('Tanggal akhir tidak boleh mendahului tanggal awal.');
      const next = new Date(`${to}T00:00:00+08:00`); next.setUTCDate(next.getUTCDate() + 1);
      params.set('from', `${from}T00:00:00+08:00`); params.set('to', next.toISOString());
    }
    for (const [id,key] of [['action','action'],['source','source'],['actor','actor_id'],['entity','entity'],['entity-id','entity_id'],['request','request_id']]) {
      const value = element(id).value.trim(); if (value) params.set(key,value);
    }
    params.set('limit',element('limit').value);
    return params.toString();
  }
  async function load() {
    if (busy) return;
    setBusy(true); more = false; element('events').replaceChildren(); message('Memuat riwayat audit…');
    try {
      const data = await api(`/api/audit/events?${activeQuery}&offset=${offset}`);
      if (!Array.isArray(data.items)) throw new Error('Respons riwayat audit tidak valid.');
      for (const event of data.items) {
        const row = document.createElement('tr');
        const metadata = {...event.metadata}; if (event.permission) metadata.permission = event.permission;
        for (const text of [`${displayTime(event.occurred_at)} / ${event.source === 'security' ? 'Keamanan' : 'Operasional'}`,event.actor_id || 'Anonim',actions[event.action] || event.action,`${event.entity} / ${event.entity_id || '—'}`,event.request_id || 'Tidak tersedia (historis)',Object.keys(metadata).length ? JSON.stringify(metadata) : '—']) {
          const cell = document.createElement('td'); cell.textContent = text; row.appendChild(cell);
        }
        element('events').appendChild(row);
      }
      more = data.has_more === true;
      element('period').textContent = `Periode ${displayTime(data.from)} sampai sebelum ${displayTime(data.to)}.`;
      element('page-info').textContent = data.items.length ? `Baris ${offset+1}–${offset+data.items.length}` : 'Tidak ada catatan pada halaman ini.';
      if (!element('from').value && !element('to').value) { element('from').value = day(data.from); element('to').value = day(new Date(new Date(data.to).getTime()-86400000)); activeQuery = query(); }
      message(data.items.length ? 'Riwayat audit berhasil dimuat.' : 'Tidak ada catatan yang sesuai filter.');
    } catch (error) { element('page-info').textContent = 'Data belum dimuat.'; message(navigator.onLine ? error.message : 'Perangkat offline. Hubungkan internet lalu pilih Muat ulang.'); }
    finally { setBusy(false); }
  }
  element('filters').addEventListener('submit', event => { event.preventDefault(); if (busy) return; try { activeQuery = query(); offset = 0; load(); } catch (error) { message(error.message, 'error'); } });
  element('clear-filters').addEventListener('click', () => { if (busy) return; element('filters').reset(); offset = 0; activeQuery = query(); load(); });
  element('previous').addEventListener('click', () => { if (!busy && offset>0) { offset = Math.max(0,offset-Number(new URLSearchParams(activeQuery).get('limit'))); load(); } });
  element('next').addEventListener('click', () => { if (!busy && more) { offset += Number(new URLSearchParams(activeQuery).get('limit')); load(); } });
  element('reload').addEventListener('click', () => load());
  (async () => {
    try { const session = await api('/api/session'); window.ULaundryUI?.setSession(session); if (session.role !== 'owner') { message('Halaman ini hanya dapat digunakan owner.'); return; } element('audit-panel').hidden = false; activeQuery = query(); await load(); }
    catch (error) { message(error.message, 'error'); }
  })();
})();
