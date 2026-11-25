(() => {
  'use strict';
  const node = (tag, text, className) => {
    const element = document.createElement(tag);
    if (text !== undefined) element.textContent = text;
    if (className) element.className = className;
    return element;
  };
  const roles = {owner:'Pemilik', admin:'Admin', cashier:'Kasir'};
  const statuses = {received:'Diterima', processing:'Diproses', ready:'Siap diambil', collected:'Sudah diambil', cancelled:'Dibatalkan'};
  const routes = [
    ['dashboard','/dashboard.html','Dashboard',['owner','admin']],
    ['orders','/index.html','Antrean',['owner','admin','cashier']],
    ['pos','/pos.html','Pesanan baru',['owner','admin','cashier']],
    ['customers','/customers.html','Pelanggan',['owner','admin','cashier']],
    ['shifts','/shifts.html','Kas dan shift',['owner','admin','cashier']],
    ['services','/services.html','Layanan dan harga',['owner','admin']],
    ['staff','/staff.html','Pengelolaan staf',['owner']],
    ['audit','/audit.html','Riwayat audit',['owner']]
  ];
  const links = [];
  const toastRegion = node('aside', undefined, 'toast-region');
  toastRegion.setAttribute('aria-label','Pemberitahuan');
  document.body.appendChild(toastRegion);
  const main = document.querySelector('main');
  if (main) {
    main.id = 'main-content'; main.tabIndex = -1;
    const skip = node('a','Lewati ke konten','skip-link'); skip.href = '#main-content'; document.body.prepend(skip);
  }
  function message(target, text, kind = 'info') {
    target.textContent = text; target.classList.add('ui-alert'); target.dataset.state = kind;
    target.setAttribute('role',kind === 'error' || kind === 'offline' ? 'alert' : 'status');
    target.setAttribute('aria-live',kind === 'error' || kind === 'offline' ? 'assertive' : 'polite');
  }
  function notify(text, kind = 'success') {
    const toast = node('div',undefined,'ui-alert toast'); toast.dataset.state = kind;
    toast.setAttribute('role',kind === 'error' ? 'alert' : 'status');
    toast.appendChild(node('span',text));
    const close = node('button','Tutup','secondary'); close.type = 'button'; close.setAttribute('aria-label','Tutup pemberitahuan');
    close.addEventListener('click',() => toast.remove()); toast.appendChild(close); toastRegion.appendChild(toast);
    if (kind !== 'error') setTimeout(() => toast.remove(),7000);
  }
  function state(target, {kind = 'info', title, description, retry}) {
    const box = node('section',undefined,'ui-state'); box.dataset.state = kind;
    box.setAttribute('role',kind === 'error' ? 'alert' : 'status');
    box.appendChild(node('h2',title));
    if (description) box.appendChild(node('p',description));
    if (retry) { const button = node('button','Coba kembali'); button.type = 'button'; button.addEventListener('click',retry); box.appendChild(button); }
    target.replaceChildren(box); target.setAttribute('aria-busy',String(kind === 'loading'));
  }
  function money(amount) {
    if (!Number.isSafeInteger(amount)) throw new Error('Nominal melampaui batas tampilan. Hubungi pemilik untuk pemeriksaan.');
    return new Intl.NumberFormat('id-ID',{style:'currency',currency:'IDR',maximumFractionDigits:0}).format(amount);
  }
  let account, logout;
  if (document.body.hasAttribute('data-staff-app')) {
    const page = document.body.dataset.page;
    const header = node('header',undefined,'app-header');
    const brand = node('a',undefined,'app-brand'); brand.href = '/index.html'; brand.appendChild(node('span','ULaundry')); brand.appendChild(node('small','Perumahan Viola'));
    const actions = node('div',undefined,'app-header-actions');
    const toggle = node('button','Menu','secondary menu-toggle'); toggle.type = 'button'; toggle.setAttribute('aria-controls','app-nav'); toggle.setAttribute('aria-expanded','false'); toggle.setAttribute('aria-label','Buka menu navigasi');
    logout = node('button','Keluar','secondary'); logout.id = 'logout'; logout.type = 'button'; logout.disabled = true;
    actions.append(toggle,logout); header.append(brand,actions);
    const navigation = node('nav',undefined,'app-nav'); navigation.id = 'app-nav'; navigation.setAttribute('aria-label','Navigasi utama');
    const list = node('ul');
    for (const [key,href,label,allowed] of routes) {
      const item = node('li'), link = node('a',label); link.href = href; link.id = key + '-link';
      if (key === page) link.setAttribute('aria-current','page');
      item.hidden = true; item.appendChild(link); list.appendChild(item); links.push({item,link,allowed});
    }
    account = node('p','Memeriksa sesi…','app-account'); navigation.append(list,account);
    function closeMenu(restoreFocus) {
      navigation.dataset.open = 'false'; toggle.setAttribute('aria-expanded','false'); toggle.setAttribute('aria-label','Buka menu navigasi');
      if (restoreFocus) toggle.focus();
    }
    toggle.addEventListener('click',() => { const open = toggle.getAttribute('aria-expanded') !== 'true'; navigation.dataset.open = String(open); toggle.setAttribute('aria-expanded',String(open)); toggle.setAttribute('aria-label',open?'Tutup menu navigasi':'Buka menu navigasi'); });
    navigation.addEventListener('keydown',event => { if (event.key === 'Escape' && navigation.dataset.open === 'true') { event.preventDefault(); closeMenu(true); } });
    toggle.addEventListener('keydown',event => { if (event.key === 'Escape') closeMenu(true); });
    const connection = node('p','Anda sedang offline. Perubahan memerlukan koneksi; isian tidak dikirim otomatis.','ui-alert connection-banner');
    connection.dataset.state = 'offline'; connection.hidden = navigator.onLine; connection.setAttribute('role','alert');
    window.addEventListener('offline',() => { connection.hidden = false; });
    window.addEventListener('online',() => { connection.hidden = true; notify('Koneksi kembali tersedia. Muat ulang data bila diperlukan.','info'); });
    document.body.insertBefore(header,main); document.body.insertBefore(connection,main); document.body.insertBefore(navigation,main);
  }
  function setSession(value) {
    for (const {item,link,allowed} of links) { const visible = allowed.includes(value.role); item.hidden = !visible; link.hidden = !visible; }
    if (account) account.textContent = `${value.username || 'Staf'} · ${roles[value.role] || 'Staf'}`;
  }
  function clearSession() {
    for (const {item,link} of links) { item.hidden = true; link.hidden = true; }
    if (account) account.textContent = 'Sesi tidak aktif';
    if (logout) logout.disabled = true;
  }
  for (const region of document.querySelectorAll('.scroll')) {
    region.tabIndex = 0;
    if (!region.hasAttribute('role')) region.setAttribute('role','region');
    if (!region.hasAttribute('aria-label')) region.setAttribute('aria-label','Tabel data, geser untuk melihat seluruh kolom');
  }
  window.ULaundryUI = Object.freeze({setSession,clearSession,message,notify,state,money,statusLabel:value => statuses[value] || value});
})();
