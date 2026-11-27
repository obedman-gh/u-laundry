(() => {
  'use strict';
  const make = (tag, text) => { const element = document.createElement(tag); element.textContent = text; return element; };
  const panel = make('details', ''); panel.className = 'pwa-panel';
  panel.appendChild(make('summary', 'Aplikasi ULaundry'));
  const connection = make('p', ''); connection.setAttribute('role', 'status');
  const status = make('p', ''); status.setAttribute('role', 'status');
  const button = (label, action) => { const element = make('button', label); element.type = 'button'; element.addEventListener('click', action); panel.appendChild(element); return element; };
  panel.append(connection, status);
  (document.querySelector('main') || document.body).prepend(panel);
  let registration, prompt, checking = false, activating = false;
  const install = button('Pasang aplikasi', async () => {
    if (!prompt) return;
    const pending = prompt; prompt = undefined; install.hidden = true;
    try { await pending.prompt(); await pending.userChoice; }
    catch (_) { status.textContent = 'Pemasangan belum berhasil. Gunakan menu browser untuk mencoba kembali.'; }
  });
  install.hidden = true;
  const update = button('Aktifkan versi baru', () => {
    const worker = registration?.waiting;
    if (!worker) { check(); return; }
    activating = true; update.disabled = true;
    worker.postMessage({type: 'ACTIVATE_UPDATE'});
    status.textContent = 'Mengaktifkan versi baru. Halaman dan isian tetap terbuka.';
  });
  update.hidden = true;
  const reload = button('Muat ulang halaman', () => window.location.reload()); reload.hidden = true;
  const checkButton = button('Periksa pembaruan', () => check(true));
  function connectivity() {
    connection.textContent = navigator.onLine ? 'Koneksi perangkat tersedia. Transaksi tetap memerlukan respons server.' :
      'Anda offline. Perubahan tidak diantrekan atau dikirim ulang otomatis. Coba kembali setelah koneksi pulih.';
    if (!navigator.onLine) panel.open = true;
    const offlineStatus = document.getElementById('offline-status');
    if (offlineStatus) offlineStatus.textContent = navigator.onLine ? 'Koneksi perangkat tersedia. Pilih Coba kembali untuk memuat halaman.' : 'Perangkat sedang offline.';
  }
  function waiting() {
    if (!registration?.waiting) return;
    update.hidden = false; update.disabled = false; panel.open = true;
    status.textContent = 'Versi baru tersedia. Selesaikan transaksi dan pastikan hasilnya sebelum memuat ulang. Draft dan key retry hanya tersimpan di tab.';
  }
  async function check(manual = false) {
    if (!registration || checking || !navigator.onLine) return;
    checking = true; checkButton.disabled = true;
    try {
      await registration.update();
      if (registration.waiting) waiting();
      else if (manual) status.textContent = registration.installing ? 'Versi baru sedang disiapkan.' : 'Pemeriksaan selesai. Belum ada versi baru yang siap.';
    } catch (_) {
      status.textContent = 'Pembaruan belum dapat diperiksa. Coba kembali saat koneksi dan server tersedia.';
      if (manual) panel.open = true;
    } finally { checking = false; checkButton.disabled = false; }
  }
  connectivity();
  window.addEventListener('offline', connectivity);
  window.addEventListener('online', () => { connectivity(); check(); }); // No business requests/replay/reload.
  document.getElementById('pwa-retry')?.addEventListener('click', () => window.location.reload());
  window.addEventListener('beforeinstallprompt', event => { event.preventDefault(); prompt = event; install.hidden = false; });
  window.addEventListener('appinstalled', () => { prompt = undefined; install.hidden = true; status.textContent = 'Aplikasi terpasang.'; });
  if (!window.isSecureContext || !('serviceWorker' in navigator)) {
    checkButton.hidden = true;
    status.textContent = 'PWA memerlukan HTTPS dan browser yang mendukung. Untuk memasang, gunakan menu browser bila tersedia.';
    return;
  }
  status.textContent = 'Pemasangan tersedia melalui menu browser jika didukung. Data bisnis tetap memerlukan koneksi.';
  let controller = navigator.serviceWorker.controller;
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    const previous = controller; controller = navigator.serviceWorker.controller;
    if (!previous && !activating) return; // First install needs no reload.
    activating = false; update.hidden = true; reload.hidden = false; panel.open = true;
    status.textContent = 'Versi baru aktif. Selesaikan transaksi dan periksa hasilnya, lalu muat ulang. Memuat ulang menghapus draft dan key retry di tab ini.';
    // Never reload automatically, including other tabs with pending writes.
  });
  navigator.serviceWorker.register('/sw.js', {scope: '/', updateViaCache: 'none'}).then(value => {
    registration = value; waiting();
    registration.addEventListener('updatefound', () => {
      const worker = registration.installing;
      worker?.addEventListener('statechange', () => {
        if (worker.state === 'installed') waiting();
        if (worker.state === 'redundant') status.textContent = 'Versi baru belum dapat disiapkan. Versi aktif tetap digunakan; coba periksa pembaruan kembali.';
      });
    });
    check();
    setInterval(() => { if (document.visibilityState === 'visible') check(); }, 5 * 60 * 1000);
    document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible') check(); });
  }).catch(() => {
    status.textContent = 'Mode offline belum siap. Muat ulang saat koneksi dan server tersedia untuk mencoba kembali.';
  });
})();
