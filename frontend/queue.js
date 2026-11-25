(() => {
  'use strict';
  const container = document.getElementById('tempat-pesanan');
  const refresh = document.getElementById('refresh-orders');
  const ui = window.ULaundryUI;
  let busy = false;
  async function read(response) {
    const data = await response.json().catch(() => ({}));
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
      await window.ULaundrySession.get();
      const orders = await read(await window.ULaundrySession.fetch('/api/pesanan',{credentials:'same-origin',cache:'no-store'}));
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
  refresh.addEventListener('click',loadOrders);
  loadOrders();
})();
