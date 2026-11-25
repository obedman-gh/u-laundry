(() => {
  'use strict';
  try { localStorage.removeItem('token'); } catch (_) {}
  const container = document.getElementById('tempat-pesanan');
  const logout = document.getElementById('logout');
  const refresh = document.getElementById('refresh-orders');
  const ui = window.ULaundryUI;
  let csrfToken = '', busy = false;
  function loginPage() { csrfToken = ''; window.location.href = '/login.html'; }
  async function read(response) {
    const data = await response.json().catch(() => ({}));
    if (response.status === 401) { loginPage(); throw new Error('Sesi telah berakhir. Silakan masuk kembali.'); }
    if (!response.ok) throw new Error(data?.error?.message || 'Permintaan tidak dapat diproses.');
    return data;
  }
  function state(kind,title,description,retry) {
    if (ui) ui.state(container,{kind,title,description,retry});
    else container.textContent = title;
  }
  async function loadOrders() {
    if (busy) return;
    busy = true; refresh.disabled = true;
    state('loading','Memuat antrean','Mengambil daftar pesanan terbaru…');
    try {
      const session = await read(await fetch('/api/session',{credentials:'same-origin'}));
      csrfToken = session.csrf_token; ui?.setSession(session); logout.disabled = false;
      const orders = await read(await fetch('/api/pesanan',{credentials:'same-origin',cache:'no-store'}));
      if (!Array.isArray(orders)) throw new Error('Respons daftar pesanan tidak valid.');
      container.replaceChildren(); container.setAttribute('aria-busy','false');
      if (orders.length === 0) state('empty','Belum ada pesanan dalam antrean.','Pesanan yang sudah dicatat akan ditampilkan di sini.');
      for (const order of orders) {
        const card = document.createElement('article'); card.className = 'card';
        const title = document.createElement('h2'); title.textContent = order.nama_customer + ' (' + order.id_pesanan + ')'; card.appendChild(title);
        const details = ['Layanan: '+order.layanan,'Total: '+(ui ? ui.money(order.total_biaya) : 'Rp '+Number(order.total_biaya).toLocaleString('id-ID')),'Status: '+(ui ? ui.statusLabel(order.status) : order.status)];
        details.forEach((text,index) => { const line = document.createElement('div'); line.textContent = text; if (index === 2) line.className = 'status'; card.appendChild(line); });
        container.appendChild(card);
      }
    } catch (error) {
      state(navigator.onLine?'error':'offline',navigator.onLine?(error.message || 'Layanan sementara tidak tersedia.'):'Perangkat sedang offline','Hubungkan koneksi atau coba muat ulang antrean.',loadOrders);
    } finally { busy = false; refresh.disabled = false; }
  }
  logout.addEventListener('click',async () => {
    if (!csrfToken) { loginPage(); return; }
    logout.disabled = true;
    try { await read(await fetch('/api/logout',{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':csrfToken}})); loginPage(); }
    catch (error) { state('error',error.message || 'Keluar gagal. Silakan coba kembali.'); logout.disabled = !csrfToken; }
  });
  refresh.addEventListener('click',loadOrders);
  loadOrders();
})();
