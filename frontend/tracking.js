(() => {
  'use strict';
  const el = id => document.getElementById(id);
  const labels = {received:'Diterima',processing:'Diproses',ready:'Siap diambil',collected:'Sudah diambil',cancelled:'Dibatalkan'};
  const message = (text, kind = 'info') => { if (window.ULaundryUI) window.ULaundryUI.message(el('message'), text, kind); else el('message').textContent = text; };
  // Share /tracking.html#TOKEN: the fragment never enters the static-page request.
  const initial = window.location.hash.slice(1); window.history.replaceState(null, '', '/tracking.html');
  async function track(token) {
    el('result').hidden = true; message('Memuat status…', 'loading'); el('track').disabled = true;
    try {
      if (!/^[A-Za-z0-9_-]{43}$/.test(token)) throw new Error('Status pesanan tidak ditemukan. Periksa tautan atau hubungi layanan.');
      const response = await fetch('/api/tracking/' + encodeURIComponent(token), {credentials:'omit',cache:'no-store',referrerPolicy:'no-referrer'});
      if (response.status === 404) throw new Error('Status pesanan tidak ditemukan. Periksa tautan atau hubungi layanan.');
      if (!response.ok) throw new Error('Layanan sementara tidak tersedia. Silakan coba kembali.');
      const data = await response.json();
      el('tracking-code').textContent = data.tracking_code; el('status').textContent = labels[data.status] || 'Hubungi layanan';
      el('estimate').textContent = 'Estimasi: ' + new Date(data.estimated_ready_at).toLocaleString('id-ID', {timeZone:'Asia/Makassar'} ) + ' WITA';
      el('service-message').textContent = data.message; el('result').hidden = false; message('Status terbaru dimuat.', 'success');
    } catch (error) { message(error.message || 'Tidak terhubung. Silakan coba kembali.', 'error'); }
    finally { el('track').disabled = false; el('token').value = ''; }
  }
  el('tracking-form').addEventListener('submit', event => { event.preventDefault(); track(el('token').value.trim()); });
  if (initial) track(initial);
})();
