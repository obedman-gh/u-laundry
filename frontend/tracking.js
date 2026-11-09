(() => {
  'use strict';
  const el = id => document.getElementById(id);
  const labels = {received:'Diterima',processing:'Diproses',ready:'Siap diambil',collected:'Sudah diambil',cancelled:'Dibatalkan'};
  // Share /tracking.html#TOKEN: the fragment never enters the static-page request.
  const initial = window.location.hash.slice(1); window.history.replaceState(null, '', '/tracking.html');
  async function track(token) {
    el('result').hidden = true; el('message').textContent = 'Memuat status…'; el('track').disabled = true;
    try {
      if (!/^[A-Za-z0-9_-]{43}$/.test(token)) throw new Error('Status pesanan tidak ditemukan. Periksa tautan atau hubungi layanan.');
      const response = await fetch('/api/tracking/' + encodeURIComponent(token), {credentials:'omit',cache:'no-store',referrerPolicy:'no-referrer'});
      if (response.status === 404) throw new Error('Status pesanan tidak ditemukan. Periksa tautan atau hubungi layanan.');
      if (!response.ok) throw new Error('Layanan sementara tidak tersedia. Silakan coba kembali.');
      const data = await response.json();
      el('tracking-code').textContent = data.tracking_code; el('status').textContent = labels[data.status] || 'Hubungi layanan';
      el('estimate').textContent = 'Estimasi: ' + new Date(data.estimated_ready_at).toLocaleString('id-ID', {timeZone:'Asia/Makassar'} ) + ' WITA';
      el('service-message').textContent = data.message; el('result').hidden = false; el('message').textContent = 'Status terbaru dimuat.';
    } catch (error) { el('message').textContent = error.message || 'Tidak terhubung. Silakan coba kembali.'; }
    finally { el('track').disabled = false; el('token').value = ''; }
  }
  el('tracking-form').addEventListener('submit', event => { event.preventDefault(); track(el('token').value.trim()); });
  if (initial) track(initial);
})();
