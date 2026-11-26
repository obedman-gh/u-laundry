(() => {
  'use strict';
  const el = id => document.getElementById(id), ui = window.ULaundryUI, auth = window.ULaundrySession, model = window.ULaundryReports;
  const money = ui.money, node = (tag,text) => {const e = document.createElement(tag);if (text !== undefined) e.textContent = text;return e;};
  const message = (text,kind = 'info') => ui.message(el('message'),text,kind);
  const result = (text,kind = 'info') => ui.message(el('command-result'),text,kind);
  const limit = 20;
  let session, active, currentReady = false, reportReady = false, entriesReady = false, busy = false, intent, uncertain = false, selected = '', offset = 0, last = true;
  function controls() {
    const locked = busy || Boolean(intent);
    document.querySelectorAll('main input,main select,main button').forEach(e => {e.disabled = locked;});
    for (const name of ['open-form','cash-form','close-form']) el(name).querySelectorAll('input,select,button').forEach(e => {e.disabled = locked || !currentReady;});
    el('print').disabled = locked || !navigator.onLine || !reportReady || (Boolean(selected) && !entriesReady);
    el('previous').disabled = locked || !selected || offset === 0;
    el('next').disabled = locked || !selected || !entriesReady || last;
    el('retry-shift').hidden = !intent || busy;el('retry-shift').disabled = busy;
    el('workspace').setAttribute('aria-busy',String(busy));
  }
  function row(target,values) {const tr = node('tr');for (const value of values) tr.appendChild(node('td',value));target.appendChild(tr);return tr;}
  function card(target,title,value) {const article = node('article');article.className = 'card';article.appendChild(node('h3',title));const p = node('p',value);p.className = 'metric';article.appendChild(p);el(target).appendChild(article);}
  async function api(path,options = {}) {
    const controller = new AbortController(), timer = setTimeout(() => controller.abort(),20000);
    try {
      const response = await auth.fetch(path,{...options,signal:controller.signal});
      if (!response.ok) throw Object.assign(new Error(response.status === 409?'Kondisi shift berubah. Muat ulang dan tinjau kembali.':response.status === 403?'Akses shift tidak diizinkan.':'Permintaan belum dapat diproses.'),{status:response.status});
      try {return await response.json();} catch (_) {throw new Error('Respons belum dapat dibaca. Periksa hasil transaksi sebelum mencoba kembali.');}
    } finally {clearTimeout(timer);}
  }
  async function run(task) {
    if (busy || intent) return;const focused = document.activeElement;busy = true;controls();
    try {await task();}
    catch (error) {message(navigator.onLine?(error.message || 'Data belum dapat dimuat. Coba kembali.'):'Perangkat sedang offline. Hubungkan koneksi lalu muat ulang.','error');}
    finally {busy = false;controls();if (document.activeElement === document.body && focused?.isConnected && !focused.disabled && focused !== document.body) focused.focus();}
  }
  function clearDaily() {
    reportReady = false;entriesReady = false;el('shift-report-data').hidden = true;el('totals').replaceChildren();el('shift-list').replaceChildren();el('entries-panel').hidden = true;el('entries').replaceChildren();selected = '';offset = 0;
  }
  function renderCurrent(current) {
    el('current-cards').replaceChildren();
    el('open-form').hidden = Boolean(current);el('cash-form').hidden = !current;el('close-form').hidden = !current;
    el('current').textContent = current?'Shift '+current.id+' · dibuka '+model.time(current.opened_at)+'. Saldo sistem tunai '+money(current.expected)+'.':'Belum ada shift aktif. Buka kas sebelum menerima atau mengembalikan pembayaran tunai.';
    if (current) {card('current-cards','Saldo awal',money(current.opening));card('current-cards','Sistem tunai saat dimuat',money(current.expected));card('current-cards','Penerimaan bersih shift',money(model.receipts(current.totals)));card('current-cards','Mutasi laci bersih',money(model.sum([current.totals.cash_in,-current.totals.cash_out])));}
  }
  function renderDaily(report) {
    el('daily-cards').replaceChildren();
    card('daily-cards','Penerimaan bersih harian',money(model.receipts(report.totals)));card('daily-cards','Mutasi laci bersih harian',money(model.sum([report.totals.cash_in,-report.totals.cash_out])));card('daily-cards','Shift beririsan tanggal',report.shifts.length.toLocaleString('id-ID'));card('daily-cards','Bagian penerimaan bersih tanpa shift',money(model.receipts(report.unassigned)));
    for (const [label,inflow,outflow] of [['Pembayaran tunai',report.totals.cash_payments,report.totals.cash_refunds],['Transfer bank',report.totals.bank_payments,report.totals.bank_refunds],['QRIS',report.totals.qris_payments,report.totals.qris_refunds],['Mutasi laci kas',report.totals.cash_in,report.totals.cash_out],['Metode belum dicatat',report.totals.unknown_in,report.totals.unknown_out]]) row(el('totals'),[label,money(inflow),money(outflow),money(model.sum([inflow,-outflow]))]);
    const u = report.unassigned;
    el('unassigned').textContent = 'Bagian tanpa shift sudah termasuk total: tunai '+money(u.cash_payments)+' / refund '+money(u.cash_refunds)+'; transfer '+money(u.bank_payments)+' / refund '+money(u.bank_refunds)+'; QRIS '+money(u.qris_payments)+' / refund '+money(u.qris_refunds)+'; metode belum dicatat '+money(u.unknown_in)+' masuk / '+money(u.unknown_out)+' keluar. Jangan dijumlahkan kembali.';
    el('report-title').textContent = 'Tanggal '+report.date+' WITA · '+(session.role === 'cashier'?'kasir sendiri':'semua kasir')+' · dimuat '+model.time(new Date().toISOString());
    for (const shift of report.shifts) {
      const tr = row(el('shift-list'),[(shift.cashier_name || shift.cashier_id)+' / '+shift.id,model.time(shift.opened_at)+' / '+(shift.closed_at?model.time(shift.closed_at):'Masih aktif'),money(shift.opening),money(shift.expected),shift.physical === null?'Belum dihitung':money(shift.physical),shift.difference === null?'Belum ditutup':money(shift.difference),(shift.closed_by || 'Belum ditutup')+' / '+(shift.opening_notes || '—')+' / '+(shift.closing_notes || '—')]);
      const td = node('td'),button = node('button','Lihat');td.className = 'no-print';button.type = 'button';button.setAttribute('aria-label','Lihat transaksi shift '+shift.id);button.addEventListener('click',() => run(async () => {selected = shift.id;await entries(0);el('entries-panel').scrollIntoView({block:'start'});}));td.appendChild(button);tr.appendChild(td);
    }
    el('empty').hidden = report.shifts.length > 0;el('shift-report-data').hidden = false;el('shift-state').replaceChildren();
  }
  async function refresh() {
    const date = el('report-date').value;model.period(date,date);
    currentReady = false;active = undefined;clearDaily();el('current-cards').replaceChildren();el('current').textContent = 'Memuat kas sendiri…';
    for (const name of ['open-form','cash-form','close-form']) el(name).hidden = true;
    ui.state(el('shift-state'),{kind:'loading',title:'Memuat ringkasan harian',description:'Mengambil transaksi sesuai tanggal dan izin akun…'});
    // Independent reads: a failed daily report does not enable unsafe shift writes.
    const reads = await Promise.allSettled([api('/api/shifts/current'),api('/api/shifts/daily?date='+encodeURIComponent(date))]);
    try {
      if (reads[0].status === 'rejected') throw reads[0].reason;
      const current = reads[0].value;if (current !== null) {model.shift(current,session.user_id);if (current.status !== 'open') throw new Error('Snapshot shift aktif tidak sesuai.');}
      renderCurrent(current);active = current;currentReady = true;
    } catch (_) {el('current').textContent = 'Kas sendiri belum dapat dimuat. Tindakan dinonaktifkan sampai muat ulang berhasil.';}
    try {
      if (reads[1].status === 'rejected') throw reads[1].reason;
      const report = model.daily(reads[1].value,date,session);renderDaily(report);reportReady = true;
    } catch (_) {clearDaily();ui.state(el('shift-state'),{kind:navigator.onLine?'error':'offline',title:'Ringkasan harian belum dapat dimuat',description:'Data lama disembunyikan. Coba muat ulang setelah akses/koneksi tersedia.',retry:() => run(refresh)});}
    if (!currentReady) message('Kas sendiri belum dapat dimuat. Pilih Muat ulang untuk mencoba kembali.','error');
    else message(reportReady?'Ringkasan diperbarui.':'Kas sendiri dimuat; ringkasan harian perlu dicoba kembali.',reportReady?'success':'error');
  }
  async function entries(nextOffset) {
    if (!selected || !reportReady) return;offset = nextOffset;entriesReady = false;last = true;el('entries-panel').hidden = false;el('entries-data').hidden = true;el('entries').replaceChildren();
    ui.state(el('entries-state'),{kind:'loading',title:'Memuat transaksi shift',description:'Mengambil halaman '+(offset/limit+1)+'…'});
    try {
      const data = await api('/api/shifts/'+encodeURIComponent(selected)+'/entries?limit='+limit+'&offset='+offset);
      if (!Array.isArray(data) || data.length > limit) throw new Error('Respons transaksi tidak valid.');
      for (const e of data) {if (e.shift_id !== selected || !['DEBIT','KREDIT'].includes(e.type) || (e.method !== '' && !Object.hasOwn(model.methods,e.method))) throw new Error('Respons transaksi tidak valid.');model.integer(e.amount);model.time(e.occurred_at);}
      for (const e of data) row(el('entries'),[model.time(e.occurred_at),model.methods[e.method || 'unknown']+' / '+(e.type === 'DEBIT'?'Masuk':'Keluar'),money(e.amount),[e.id,e.payment_id || e.refund_id || '—',e.order_id || '—',e.notes].join(' / ')]);
      if (!data.length) row(el('entries'),['Belum ada transaksi pada halaman ini.','—','—','—']);
      el('entries-title').textContent = 'Shift '+selected+' · halaman '+(offset/limit+1)+' · dimuat '+model.time(new Date().toISOString())+'. Seluruh sesi; tidak dibatasi tanggal harian di atas.';
      el('page-info').textContent = 'Halaman '+(offset/limit+1)+' · '+data.length+' transaksi';last = data.length < limit;entriesReady = true;el('entries-data').hidden = false;el('entries-state').replaceChildren();
    } catch (_) {ui.state(el('entries-state'),{kind:navigator.onLine?'error':'offline',title:'Transaksi shift belum dapat dimuat',description:'Halaman lama disembunyikan.',retry:() => run(() => entries(nextOffset))});}
  }
  const amount = (id,positive = false) => {const raw = el(id).value;if (!/^[0-9]+$/.test(raw)) throw new Error('Isi rupiah bulat tanpa pemisah ribuan.');const n = model.integer(Number(raw));if (positive && n === 0) throw new Error('Nominal mutasi harus lebih dari nol.');return n;};
  const notes = (id,required = false) => {const raw = el(id).value.trim();if ((required && !raw) || [...raw].length > 250 || /[\p{Cc}\p{Cf}]/u.test(raw)) throw new Error('Catatan satu baris maksimal 250 karakter'+(required?' wajib diisi.':'.'));return raw;};
  async function command(name,path,body,form) {
    if (busy || intent || !currentReady) return;
    if (!navigator.onLine) {result('Perangkat sedang offline. Tidak ada transaksi yang dikirim.','offline');return;}
    if (name === 'close' && !window.confirm('Tutup shift dengan saldo fisik '+money(body.physical)+'? Penutupan tidak dapat diubah.')) return;
    const bytes = new Uint8Array(24);window.crypto.getRandomValues(bytes);
    intent = Object.freeze({name,path,body:JSON.stringify(body),key:Array.from(bytes,b => b.toString(16).padStart(2,'0')).join(''),form});uncertain = false;await sendCommand();
  }
  async function sendCommand() {
    if (busy || !intent) return;
    if (!navigator.onLine) {result('Masih offline. Simpan tab ini dan coba kembali dengan transaksi yang sama.','offline');return;}
    busy = true;controls();result('Menunggu konfirmasi transaksi…','loading');const attempt = intent;
    try {
      const data = await api(attempt.path,{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':attempt.key},body:attempt.body});
      if (typeof data.replayed !== 'boolean' || typeof data.id !== 'string' || !data.id) throw new Error();
      if (attempt.name === 'cash') {if (data.shift_id !== active?.id || data.amount !== JSON.parse(attempt.body).amount || data.type !== (JSON.parse(attempt.body).direction === 'in'?'DEBIT':'KREDIT')) throw new Error();}
      else model.shift(data,session.user_id);
      intent = undefined;uncertain = false;currentReady = false;attempt.form.reset();
      result('Transaksi '+data.id+(data.replayed?' sebelumnya sudah tercatat; hasil idempotent ditemukan tanpa posting ulang.':' berhasil dicatat.'),'success');
      try {await refresh();} catch (_) {currentReady = false;clearDaily();message('Transaksi sudah terkonfirmasi tersimpan. Muat ulang untuk mengambil ringkasan terbaru.','error');}
    } catch (error) {
      if (error.status >= 400 && error.status < 500 && error.status !== 408 && !uncertain) {
        intent = undefined;currentReady = false;clearDaily();result(error.message+' Periksa shift dan muat ulang sebelum mengirim transaksi baru.','error');
      } else {uncertain = true;result('Hasil belum dapat dipastikan. Jangan membuat transaksi pengganti. Pilih Periksa / kirim ulang untuk memakai kunci dan isi yang sama. Tetap buka tab ini.','error');}
    } finally {busy = false;controls();(intent?el('retry-shift'):el('reload')).focus();}
  }
  const submit = (id,fn) => el(id).addEventListener('submit',event => {event.preventDefault();if (busy || intent) return;try {fn();}catch(error){result(error.message,'error');}});
  submit('open-form',() => {if (active) return;command('open','/api/shifts',{opening:amount('opening'),notes:notes('opening-notes')},el('open-form'));});
  submit('cash-form',() => {if (!active) return;const value = amount('amount',true),direction = el('direction').value;if (!['in','out'].includes(direction)) throw new Error('Pilih arah mutasi yang sah.');if (direction === 'out' && value > active.expected) throw new Error('Kas keluar melebihi saldo sistem yang dimuat. Muat ulang dan periksa kembali.');command('cash','/api/shifts/'+encodeURIComponent(active.id)+'/cash',{amount:value,direction,notes:notes('cash-notes',true)},el('cash-form'));});
  submit('close-form',() => {if (!active) return;const physical = amount('physical');command('close','/api/shifts/'+encodeURIComponent(active.id)+'/close',{physical,notes:notes('closing-notes',physical !== active.expected)},el('close-form'));});
  submit('date-form',() => run(refresh));el('reload').addEventListener('click',() => run(refresh));
  el('report-date').addEventListener('change',() => {if (busy || intent) return;clearDaily();ui.state(el('shift-state'),{title:'Tanggal berubah',description:'Pilih Tampilkan untuk memuat ringkasan tanggal ini.'});controls();});
  el('previous').addEventListener('click',() => {if (offset > 0) run(() => entries(Math.max(0,offset-limit)));});el('next').addEventListener('click',() => {if (entriesReady && !last) run(() => entries(offset+limit));});
  el('print').addEventListener('click',() => {if (!el('print').disabled) window.print();});el('retry-shift').addEventListener('click',sendCommand);
  window.addEventListener('offline',() => {clearDaily();message('Sedang offline. Transaksi tidak dikirim otomatis; muat ulang saat koneksi tersedia.','offline');controls();});
  document.addEventListener('click',event => {if (!intent) return;const a = event.target.closest('a');if ((a && a.target !== '_blank') || event.target.closest('#logout')) {event.preventDefault();event.stopImmediatePropagation();result('Pastikan hasil transaksi dengan kunci yang sama sebelum meninggalkan halaman.','error');}},true);
  window.addEventListener('beforeunload',event => {if (intent && !el('private-content').hidden) {event.preventDefault();event.returnValue = '';}});
  run(async () => {session = await auth.get();el('scope').textContent = session.role === 'cashier'?'Kas dan laporan transaksi milik Anda sendiri.':'Kelola kas sendiri; laporan harian mencakup semua kasir.';el('report-date').value = model.date(Date.now());el('workspace').hidden = false;await refresh();});
})();
