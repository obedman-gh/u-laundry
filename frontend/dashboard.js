(() => {
  'use strict';
  const el = id => document.getElementById(id), ui = window.ULaundryUI, auth = window.ULaundrySession, model = window.ULaundryReports;
  const node = (tag,text) => {const e = document.createElement(tag);if (text !== undefined) e.textContent = text;return e;};
  const message = (id,text,kind = 'info') => ui.message(el(id),text,kind);
  const money = ui.money, count = n => model.integer(n).toLocaleString('id-ID');
  const limit = 20, tables = {orders:{offset:0},ledger:{offset:0}};
  let session, busy = false, applied, summary, cashierReady = false;
  const manager = () => ['owner','admin'].includes(session?.role);
  function controls() {
    document.querySelectorAll('main input,main select,main button').forEach(e => { e.disabled = busy; });
    for (const [name,state] of Object.entries(tables)) {
      el(name+'-prev').disabled = busy || !applied || state.offset === 0;
      el(name+'-next').disabled = busy || !state.data?.has_more;
      el(name+'-export').disabled = busy || !state.data || !manager() || !navigator.onLine;
    }
    el('print-dashboard').disabled = busy || !navigator.onLine || (manager()?!(summary && tables.orders.data && tables.ledger.data):!cashierReady);
  }
  const row = (body,values) => {const tr = node('tr');for (const value of values) tr.appendChild(node('td',value));body.appendChild(tr);};
  function card(target,title,value) {const article = node('article');article.className = 'card';article.appendChild(node('h3',title));const p = node('p',value);p.className = 'metric';article.appendChild(p);el(target).appendChild(article);}
  function metadata(data) {return applied.first+' sampai '+applied.last+' WITA · dibaca '+model.time(data.as_of)+' · ID '+data.id;}
  async function read(path) {
    const controller = new AbortController(), timer = setTimeout(() => controller.abort(),20000);
    try {
      const response = await auth.fetch(path,{signal:controller.signal});
      if (!response.ok) throw Object.assign(new Error(response.status === 403?'Akses laporan tidak diizinkan.':'Laporan belum dapat dimuat. Periksa koneksi lalu coba kembali.'),{status:response.status});
      return await response.json();
    } finally {clearTimeout(timer);}
  }
  function failure(target,retry,error) {
    if (error?.status === 403) auth.forbid('Akses data ini tidak diizinkan. Periksa peran akun Anda.');
    ui.state(el(target),{kind:navigator.onLine?'error':'offline',title:navigator.onLine?'Data belum dapat dimuat':'Perangkat sedang offline',description:'Data lama disembunyikan. Periksa koneksi dan coba kembali.',retry});
  }
  async function run(task) {
    if (busy) return;const focused = document.activeElement;busy = true;controls();
    try {await task();}
    catch (error) {failure('dashboard-state',() => run(load),error);}
    finally {busy = false;controls();if (document.activeElement === document.body && focused?.isConnected && !focused.disabled && focused !== document.body) focused.focus();}
  }
  function invalidate(name) {
    tables[name].data = undefined;el(name+'-content').hidden = true;el(name+'-rows').replaceChildren();el(name+'-state').replaceChildren();el(name+'-page').textContent = 'Belum dimuat';
  }
  function periodChanged() {
    if (busy || !manager()) return;
    applied = undefined;summary = undefined;el('dashboard-content').hidden = true;el('dashboard-state').replaceChildren();
    for (const name of Object.keys(tables)) {tables[name].offset = 0;invalidate(name);}
    message('period-message','Tanggal berubah. Pilih Tampilkan laporan untuk memuat periode ini.');message('export-message','');controls();
  }
  function renderSummary(data) {
    el('owner-cards').replaceChildren();
    for (const [label,value] of [['Pesanan dibuat dalam periode',count(data.orders.created)],['Penerimaan dalam periode',money(data.money.payments)],['Refund dalam periode',money(data.money.refunds)],['Penerimaan bersih periode',money(data.money.net)],['Piutang saat pembacaan',money(data.orders.receivable)],['Terlambat saat pembacaan',count(data.orders.overdue)],['Shift aktif saat pembacaan',count(data.shifts.active)],['Selisih penutupan periode',money(data.shifts.difference)]]) card('owner-cards',label,value);
    el('updated').textContent = metadata(data);
    el('queue-summary').replaceChildren();
    for (const [label,value] of [['Diterima',data.orders.received],['Diproses',data.orders.processing],['Siap diambil',data.orders.ready],['Pesanan berpiutang',data.orders.receivable_orders]]) el('queue-summary').appendChild(node('li',label+': '+count(value)));
    el('money-summary').replaceChildren();
    for (const [label,value] of [['Biaya selain refund/mutasi',money(data.money.expenses)],['Mutasi laci masuk',money(data.money.cash_in)],['Mutasi laci keluar',money(data.money.cash_out)],['Shift dibuka',count(data.shifts.opened)],['Shift ditutup',count(data.shifts.closed)],['Sistem tunai saat penutupan',money(data.shifts.system)],['Fisik saat penutupan',money(data.shifts.physical)]]) {el('money-summary').appendChild(node('dt',label));el('money-summary').appendChild(node('dd',value));}
    el('method-rows').replaceChildren();
    for (const method of data.methods) row(el('method-rows'),[model.methods[method.method],money(method.payments),money(method.refunds),money(model.sum([method.payments,-method.refunds]))]);
    el('unassigned-summary').textContent = 'Termasuk transaksi tanpa shift: penerimaan '+money(data.money.unassigned_payments)+', refund '+money(data.money.unassigned_refunds)+'. Bagian ini sudah termasuk total, jangan dijumlahkan kembali.';
    el('popular-rows').replaceChildren();
    if (!data.popular_services.length) row(el('popular-rows'),['Belum ada layanan pada periode ini.','—','—','—']);
    for (const service of data.popular_services) row(el('popular-rows'),[service.name+' / '+service.service_id,count(service.orders),service.unit === 'kg'?count(service.grams)+' gram':count(service.pieces)+' item',money(service.gross_before_discount)]);
    el('dashboard-state').replaceChildren();
    if (data.orders.created === 0 && data.money.payments === 0 && data.money.refunds === 0) ui.state(el('dashboard-state'),{kind:'empty',title:'Belum ada pesanan baru atau penerimaan/refund dalam periode',description:'Antrean dan piutang saat ini tetap ditampilkan bila tersedia.'});
    el('dashboard-content').hidden = false;
  }
  function query(name,state,format) {
    const p = new URLSearchParams({from:applied.from,to:applied.to,limit:String(limit),offset:String(state.offset)});
    if (name === 'orders') {p.set('kind',state.kind);if (state.status) p.set('status',state.status);}
    if (format) p.set('format',format);
    return '/api/laporan/'+name+'?'+p;
  }
  async function loadTable(name,offset = 0) {
    if (!manager() || !applied) return;
    const state = tables[name];state.offset = offset;
    if (name === 'orders') {state.kind = el('orders-kind').value;state.status = el('orders-status').value;}
    invalidate(name);ui.state(el(name+'-state'),{kind:'loading',title:'Memuat detail',description:'Mengambil halaman laporan…'});
    try {
      const data = model.page(await read(query(name,state)),applied,name === 'orders'?state.kind:'ledger',offset,limit,state.status);
      const body = el(name+'-rows');
      for (const x of data.items) {
        if (name === 'orders') row(body,[x.id+' / '+x.customer_name,ui.statusLabel(x.status),money(x.total),money(x.paid),money(x.due),model.time(x.created_at)+' / '+model.time(x.estimated_ready_at)]);
        else row(body,[model.time(x.occurred_at),(x.type === 'DEBIT'?'Masuk':'Keluar')+' / '+model.methods[x.method || 'unknown'],money(x.amount),[x.id,'Order: '+(x.order_id || '—'),'Pembayaran: '+(x.payment_id || '—'),'Refund: '+(x.refund_id || '—'),'Shift: '+(x.shift_id || '—')].join(' · '),x.description+' / '+x.recorded_by]);
      }
      if (!data.items.length) row(body,name === 'orders'?['Tidak ada pesanan sesuai filter.','—','—','—','—','—']:['Belum ada posting pada halaman ini.','—','—','—','—']);
      const label = name === 'orders'?({created:'Dibuat dalam periode',receivable:'Piutang saat pembacaan',overdue:'Terlambat saat pembacaan'})[state.kind]+' · '+(state.status?ui.statusLabel(state.status):'Semua status'):'Posting dalam periode';
      el(name+'-meta').textContent = metadata(data)+' · '+label+' · halaman '+(offset/limit+1)+(data.has_more?' · masih ada halaman berikutnya':' · halaman terakhir');
      el(name+'-page').textContent = 'Halaman '+(offset/limit+1)+' · '+data.items.length+' baris';
      el(name+'-state').replaceChildren();el(name+'-content').hidden = false;state.data = data;
    } catch (error) {invalidate(name);failure(name+'-state',() => run(() => loadTable(name,offset)),error);}
  }
  async function load() {
    if (!session) {session = await auth.get();el('dashboard-scope').textContent = manager()?'Laporan seluruh kasir · pemilik dan admin':'Operasional dan shift Anda sendiri';}
    if (!manager()) {
      el('owner-workspace').hidden = true;el('cashier-content').hidden = true;cashierReady = false;
      ui.state(el('dashboard-state'),{kind:'loading',title:'Memuat shift Anda',description:'Ringkasan hanya untuk akun kasir ini.'});
      const current = await read('/api/shifts/current');
      if (current !== null) {model.shift(current,session.user_id);if (current.status !== 'open') throw new Error('Snapshot shift aktif tidak sesuai.');}
      el('cashier-cards').replaceChildren();el('dashboard-state').replaceChildren();
      if (current) {
        card('cashier-cards','Sistem tunai shift Anda',money(current.expected));card('cashier-cards','Saldo awal',money(current.opening));card('cashier-cards','Penerimaan bersih shift',money(model.receipts(current.totals)));card('cashier-cards','Mutasi laci bersih',money(model.sum([current.totals.cash_in,-current.totals.cash_out])));
        el('cashier-updated').textContent = 'Shift '+current.id+' · dibuka '+model.time(current.opened_at)+' · dimuat '+model.time(new Date().toISOString());
      } else {el('cashier-updated').textContent = 'Belum ada shift aktif milik Anda.';ui.state(el('dashboard-state'),{kind:'empty',title:'Buka kas sebelum transaksi tunai',description:'Gunakan Kas dan shift untuk membuka shift Anda.'});}
      cashierReady = true;el('cashier-content').hidden = false;return;
    }
    el('cashier-content').hidden = true;el('owner-workspace').hidden = false;
    if (!el('report-from').value && !el('report-to').value) el('report-from').value = el('report-to').value = model.date(Date.now());
    let p;
    try {p = model.period(el('report-from').value,el('report-to').value);}
    catch (error) {message('period-message',error.message,'error');el('report-from').focus();return;}
    applied = p;summary = undefined;message('period-message','');message('export-message','');el('dashboard-content').hidden = true;
    ui.state(el('dashboard-state'),{kind:'loading',title:'Memuat ringkasan',description:'Mengambil laporan periode WITA…'});
    await Promise.allSettled([
      (async () => {try {const data = model.dashboard(await read('/api/laporan/dashboard?'+new URLSearchParams({from:p.from,to:p.to})),p);renderSummary(data);summary = data;}catch(error){failure('dashboard-state',() => run(load),error);}})(),
      loadTable('orders',0),loadTable('ledger',0)
    ]);
  }
  async function exportPage(name) {
    if (!manager() || !applied || !tables[name].data) return;
    message('export-message','Menyiapkan CSV halaman ini…','loading');
    const state = tables[name], controller = new AbortController(), timer = setTimeout(() => controller.abort(),20000);
    try {
      const response = await auth.fetch(query(name,state,'csv'),{signal:controller.signal});
      if (!response.ok) throw Object.assign(new Error('CSV belum dapat diunduh.'),{status:response.status});
      const h = response.headers;
      if (!/^text\/csv(?:;|$)/i.test(h.get('Content-Type') || '') || Number(h.get('X-Report-Limit')) !== limit || Number(h.get('X-Report-Offset')) !== state.offset || !['true','false'].includes(h.get('X-Report-Has-More'))) throw new Error('Metadata CSV tidak valid.');
      const meta = model.meta({id:h.get('X-Report-ID'),from:h.get('X-Report-From'),to:h.get('X-Report-To'),as_of:h.get('X-Report-As-Of'),timezone:model.zone},applied);
      // Download the server's CSV bytes. Formula protection and quoting belong
      // to its established encoding/csv implementation, never HTML rendering.
      const blob = await response.blob(), url = URL.createObjectURL(blob), link = node('a');
      link.href = url;link.download = 'ulaundry-'+name+'-'+applied.first+'-'+applied.last+'-offset-'+state.offset+'.csv';document.body.appendChild(link);link.click();link.remove();setTimeout(() => URL.revokeObjectURL(url),1000);
      message('export-message','Unduhan CSV halaman '+(state.offset/limit+1)+' dimulai. Dibaca '+model.time(meta.as_of)+' · ID '+meta.id+(h.get('X-Report-Has-More') === 'true'?'. Masih ada halaman berikutnya.':'. Halaman terakhir.')+' Ekspor memakai pembacaan baru.','success');
    } catch (error) {message('export-message',navigator.onLine?'CSV belum dapat diunduh. Periksa akses/koneksi lalu coba kembali.':'Perangkat sedang offline. Tidak ada ekspor yang diantrekan.','error');if (error.status === 403) auth.forbid('Ekspor laporan hanya untuk pemilik dan admin aktif.');}
    finally {clearTimeout(timer);}
  }
  el('period-form').addEventListener('submit',event => {event.preventDefault();if (manager()) run(load);});
  ['report-from','report-to'].forEach(id => {el(id).addEventListener('input',periodChanged);el(id).addEventListener('change',periodChanged);});
  el('today-report').addEventListener('click',() => {if (!manager() || busy) return;el('report-from').value = el('report-to').value = model.date(Date.now());periodChanged();run(load);});
  el('orders-filter').addEventListener('submit',event => {event.preventDefault();if (!manager() || busy) return;if (!applied) {message('period-message','Tampilkan periode laporan terlebih dahulu.');return;}run(() => loadTable('orders',0));});
  ['orders-kind','orders-status'].forEach(id => el(id).addEventListener('change',() => {if (busy || !manager()) return;invalidate('orders');tables.orders.offset = 0;ui.state(el('orders-state'),{title:'Filter pesanan berubah',description:'Pilih Terapkan filter pesanan untuk memuat hasil.'});controls();}));
  for (const name of Object.keys(tables)) {
    el(name+'-prev').addEventListener('click',() => {if (manager() && tables[name].offset > 0) run(() => loadTable(name,Math.max(0,tables[name].offset-limit)));});
    el(name+'-next').addEventListener('click',() => {if (manager() && tables[name].data?.has_more) run(() => loadTable(name,tables[name].offset+limit));});
    el(name+'-export').addEventListener('click',() => run(() => exportPage(name)));
  }
  el('refresh-dashboard').addEventListener('click',() => run(load));
  el('print-dashboard').addEventListener('click',() => {if (!el('print-dashboard').disabled) window.print();});
  window.addEventListener('offline',() => {message('export-message','Sedang offline. Muat ulang saat koneksi tersedia; tampilan cetak/ekspor dinonaktifkan.','offline');summary = undefined;cashierReady = false;for (const name of Object.keys(tables)) invalidate(name);el('dashboard-content').hidden = true;el('cashier-content').hidden = true;controls();});
  run(load);
})();
