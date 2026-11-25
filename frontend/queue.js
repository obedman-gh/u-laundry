(() => {
  'use strict';
  const el = id => document.getElementById(id), ui = window.ULaundryUI, auth = window.ULaundrySession, model = window.ULaundryQueue;
  const node = (tag,text) => { const e = document.createElement(tag); if (text !== undefined) e.textContent = text; return e; };
  const message = (id,text,kind = 'info') => ui.message(el(id),text,kind);
  const wita = at => new Date(at).toLocaleString('id-ID',{timeZone:'Asia/Makassar'})+' WITA';
  const methodLabel = method => ({cash:'Tunai',bank_transfer:'Transfer bank',qris:'QRIS'})[method] || 'Metode tidak dikenal';
  const paymentLabel = status => ({paid:'Lunas',partially_paid:'Dibayar sebagian',unpaid:'Belum dibayar'})[status] || 'Periksa pembayaran';
  let role, offset = 0, filter = {search:'',status:''}, pageLength = 0, listBusy = false, detailBusy = false, listFailed = false;
  let order, payments = [], paymentOffset = 0, paymentMore = false, selectedRefund, intent, sending = false, needsReview = false;
  function controls() {
    const locked = detailBusy || sending || Boolean(intent), edit = locked || needsReview || !order;
    ['queue-search','queue-status','apply-filter','refresh-orders'].forEach(id => { el(id).disabled = listBusy || locked; });
    el('queue-prev').disabled = listBusy || locked || offset === 0;
    el('queue-next').disabled = listBusy || locked || listFailed || pageLength < 20;
    el('refresh-detail').disabled = locked; el('close-detail').disabled = locked;
    el('tempat-pesanan').querySelectorAll('button').forEach(e => { e.disabled = locked; });
    ['payment-form','cancel-form','refund-form','status-actions','payment-list'].forEach(id => el(id).querySelectorAll('input,select,button').forEach(e => { e.disabled = edit; }));
    el('more-payments').disabled = edit;
    el('retry-command').hidden = !intent || sending; el('retry-command').disabled = detailBusy || sending;
    if (order) {
      const allowed = model.actions(order,role);
      el('payment-form').hidden = !allowed.payment;
      el('cancel-form').hidden = !allowed.cancel;
      el('refund-form').hidden = !allowed.refund || !selectedRefund;
    }
  }
  async function read(path) {
    const response = await auth.fetch(path);
    if (!response.ok) throw new Error(response.status === 403?'Akses tidak diizinkan.':'Data belum dapat dimuat. Coba kembali.');
    return response.json();
  }
  async function loadOrders(nextOffset = offset) {
    if (listBusy || detailBusy || intent || sending) return;
    listBusy = true; listFailed = false; offset = nextOffset; controls();
    el('queue-page').textContent = 'Halaman '+(offset/20+1)+' · memuat…';
    el('tempat-pesanan').setAttribute('aria-busy','true');
    ui.state(el('tempat-pesanan'),{kind:'loading',title:'Memuat antrean',description:'Mengambil pesanan sesuai filter…'});
    try {
      await auth.get();
      const params = new URLSearchParams({limit:'20',offset:String(offset)});
      for (const [name,value] of Object.entries(filter)) if (value) params.set(name,value);
      const rows = await read('/api/pesanan?'+params);
      if (!Array.isArray(rows)) throw new Error();
      rows.forEach(model.validate); pageLength = rows.length;
      el('tempat-pesanan').replaceChildren();
      if (!rows.length) ui.state(el('tempat-pesanan'),{kind:'empty',title:'Tidak ada pesanan pada halaman ini',description:'Ubah filter atau kembali ke halaman sebelumnya.'});
      for (const row of rows) {
        const card = node('article'), title = node('h2',row.nama_customer+' ('+row.id_pesanan+')');card.className = 'card';card.appendChild(title);
        [row.layanan,'Total: '+ui.money(row.total_biaya),'Status: '+ui.statusLabel(row.status)+' · '+paymentLabel(row.payment_status),'Estimasi: '+wita(row.waktu_perkiraan_selesai)].forEach(text => card.appendChild(node('p',text)));
        if (model.overdue(row)) { const badge = node('p','Terlambat dari estimasi');badge.className = 'overdue';card.appendChild(badge); }
        const button = node('button','Detail pesanan');button.type = 'button';button.setAttribute('aria-label','Detail pesanan '+row.id_pesanan);button.addEventListener('click',() => loadDetail(row.id_pesanan));card.appendChild(button);el('tempat-pesanan').appendChild(card);
      }
      el('queue-page').textContent = 'Halaman '+(offset/20+1)+' · '+rows.length+' pesanan';message('queue-message','');
    } catch (_) {
      listFailed = true; pageLength = 0;
      el('queue-page').textContent = 'Halaman '+(offset/20+1)+' · belum dimuat';
      ui.state(el('tempat-pesanan'),{kind:navigator.onLine?'error':'offline',title:navigator.onLine?'Antrean belum dapat dimuat':'Perangkat sedang offline',description:'Periksa koneksi lalu muat ulang.',retry:() => loadOrders(offset)});
    } finally { listBusy = false;el('tempat-pesanan').setAttribute('aria-busy','false');controls(); }
  }
  function renderPayments() {
    el('payment-list').replaceChildren();
    if (!payments.length) el('payment-list').appendChild(node('li','Belum ada pembayaran.'));
    for (const payment of payments) {
      const item = node('li',payment.payment_id+' · '+ui.money(payment.amount)+' · '+methodLabel(payment.method)+' · '+(payment.status === 'refunded'?'Sudah direfund':'Tercatat')+' · '+wita(payment.received_at));
      if (model.actions(order,role).refund && payment.status === 'posted') {
        const button = node('button','Refund penuh');button.type = 'button';button.className = 'secondary';button.addEventListener('click',() => {
          if (intent || sending || detailBusy || needsReview || !model.actions(order,role).refund) return;
          selectedRefund = payment;el('refund-reason').value = '';el('refund-selection').textContent = payment.payment_id+' · '+ui.money(payment.amount)+' · '+methodLabel(payment.method);controls();el('refund-reason').focus();
        });item.appendChild(button);
      }
      el('payment-list').appendChild(item);
    }
    el('more-payments').hidden = !paymentMore;
  }
  function renderDetail() {
    const summary = el('detail-summary');summary.replaceChildren();
    el('detail-title').textContent = 'Pesanan '+order.id_pesanan;
    ['Pelanggan: '+order.nama_customer,'Status: '+ui.statusLabel(order.status)+' · '+paymentLabel(order.payment_status),'Versi: '+order.version,'Diterima: '+wita(order.waktu_pesan),'Estimasi: '+wita(order.waktu_perkiraan_selesai)].forEach(text => summary.appendChild(node('p',text)));
    if (model.overdue(order)) {const p = node('p','Terlambat dari estimasi');p.className = 'overdue';summary.appendChild(p);}
    const items = node('ul');
    for (const item of order.items || []) items.appendChild(node('li',item.service_name+' · '+(item.unit === 'kg'?item.grams+' gram':item.pieces+' item')+' × '+ui.money(item.unit_price)+'/'+item.unit+' = '+ui.money(item.line_total)));
    summary.appendChild(items);
    ['Subtotal: '+ui.money(order.subtotal),'Diskon: '+ui.money(order.discount),'Total: '+ui.money(order.total_biaya),'Dibayar bersih: '+ui.money(order.paid),'Sisa tagihan: '+ui.money(order.balance)].forEach(text => summary.appendChild(node('p',text)));
    for (const [field,label] of [['processing_at','Mulai proses'],['waktu_selesai_real','Siap'],['collected_at','Diambil'],['cancelled_at','Dibatalkan']]) if (order[field]) summary.appendChild(node('p',label+': '+wita(order[field])));
    if (order.notes) { const p = node('p','Catatan: '+order.notes);p.className = 'notes';summary.appendChild(p); }
    if (order.cancellation_reason) summary.appendChild(node('p','Alasan batal: '+order.cancellation_reason));
    for (const paper of ['58','80']) el('receipt-'+paper).href = '/api/pesanan/'+encodeURIComponent(order.id_pesanan)+'/struk?paper='+paper;
    el('status-actions').replaceChildren();
    const allowed = model.actions(order,role);
    if (allowed.next) {const button = node('button',ui.statusLabel(allowed.next));button.type = 'button';button.addEventListener('click',() => begin('status',{status:allowed.next,version:order.version}));el('status-actions').appendChild(button);}
    el('status-help').textContent = order.status === 'ready' && order.balance > 0?'Lunasi tagihan sebelum pesanan diambil.':allowed.next?'Tindakan mengikuti status dan versi yang sedang ditampilkan.':'Tidak ada tindakan status berikutnya.';
    el('payment-amount').value = String(order.balance);el('payment-method').value = 'cash';el('cancel-reason').value = '';el('refund-reason').value = '';selectedRefund = undefined;renderPayments();
  }
  async function loadDetail(id = order?.id_pesanan) {
    if (!id || detailBusy || intent || sending) return;
    detailBusy = true;needsReview = true;order = undefined;el('order-detail').hidden = false;
    el('detail-summary').replaceChildren();el('status-actions').replaceChildren();el('payment-list').replaceChildren();
    ['payment-form','cancel-form','refund-form','more-payments','receipt-58','receipt-80'].forEach(id => {el(id).hidden = true;});controls();
    el('refresh-detail').dataset.order = id;
    message('detail-message','Memuat snapshot pesanan dan pembayaran…','loading');
    try {
      const base = '/api/pesanan/'+encodeURIComponent(id);
      const [snapshot,rows] = await Promise.all([read(base),read(base+'/pembayaran?limit=20&offset=0')]);
      model.detail(snapshot);
      if (snapshot.id_pesanan !== id || !Array.isArray(rows) || !snapshot.items?.length) throw new Error();
      rows.forEach(row => model.payment(row,id));
      order = snapshot;payments = rows;paymentOffset = 0;paymentMore = rows.length === 20;needsReview = false;
      renderDetail();['receipt-58','receipt-80'].forEach(id => {el(id).hidden = false;});
      message('detail-message','Data terbaru dimuat. Tinjau status dan saldo sebelum mengirim tindakan.','success');
      message('command-result','');
    } catch (_) { order = undefined;needsReview = true;message('detail-message',navigator.onLine?'Detail belum dapat dimuat. Pilih Muat ulang detail; tindakan dinonaktifkan.':'Perangkat sedang offline. Sambungkan koneksi lalu muat ulang detail.','error'); }
    finally {detailBusy = false;controls();el('detail-title').focus();}
  }
  function validReason(value) { return value.trim().length > 0 && [...value.trim()].length <= 250 && !/[\p{Cc}\p{Cf}]/u.test(value); }
  function key() {const bytes = new Uint8Array(24);window.crypto.getRandomValues(bytes);return Array.from(bytes,b => b.toString(16).padStart(2,'0')).join('');}
  async function begin(kind,body) {
    if (!order || intent || sending || detailBusy || needsReview) return;
    const allowed = model.actions(order,role);
    if ((kind === 'status' && body.status !== allowed.next) || (kind === 'cancel' && !allowed.cancel) || (kind === 'payments' && !allowed.payment) || (kind === 'refunds' && (!allowed.refund || selectedRefund?.status !== 'posted'))) return;
    if (!navigator.onLine) {message('command-result','Perangkat sedang offline. Tidak ada permintaan yang dikirim.','offline');return;}
    intent = Object.freeze({kind,body:JSON.stringify(body),key:key(),id:order.id_pesanan});
    await sendCommand();
  }
  async function sendCommand() {
    if (!intent || sending || detailBusy) return;
    if (!navigator.onLine) {message('command-result','Perangkat sedang offline. Permintaan tetap tersimpan di tab ini; coba kembali setelah terhubung.','offline');return;}
    sending = true;controls();message('command-result','Menunggu konfirmasi server…','loading');
    const command = intent, controller = new AbortController(), timer = setTimeout(() => controller.abort(),20000);
    try {
      const suffix = command.kind === 'status'?'':command.kind === 'payments'?'/pembayaran':'/'+command.kind;
      const response = await auth.fetch('/api/pesanan/'+encodeURIComponent(command.id)+suffix,{method:command.kind === 'status'?'PUT':'POST',headers:{'Content-Type':'application/json','Idempotency-Key':command.key},body:command.body,signal:controller.signal});
      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        if (response.status >= 500 || response.status === 408 || needsReview) throw new Error();
        const messages = {shift_required:'Buka shift Anda sebelum transaksi tunai.',insufficient_cash:'Saldo tunai shift tidak cukup untuk refund.',forbidden:'Anda tidak diizinkan menjalankan tindakan ini.'};
        intent = undefined;needsReview = true;
        message('detail-message','Snapshot sebelumnya. Muat ulang detail sebelum tindakan berikutnya.');
        message('queue-message','Kondisi pesanan mungkin berubah. Muat ulang antrean untuk memperbarui kartu.');
        message('command-result',(response.status === 409?'Data atau kondisi pesanan telah berubah. ':messages[data?.error?.code] || 'Permintaan ditolak. ')+'Muat ulang detail dan tinjau kembali sebelum membuat permintaan baru.','error');
        return;
      }
      const data = await response.json();
      if (command.kind === 'status' || command.kind === 'cancel') {model.validate(data);if (data.id_pesanan !== command.id) throw new Error();}
      else {
        const body = JSON.parse(command.body), id = command.kind === 'payments'?data.payment_id:data.refund_id;
        if (typeof id !== 'string' || !id || data.order_id !== command.id || data.idempotency_key !== command.key || !Number.isSafeInteger(data.amount) || data.amount < 1 || typeof data.replayed !== 'boolean' || (command.kind === 'payments' && (data.amount !== body.amount || data.method !== body.method)) || (command.kind === 'refunds' && (data.payment_id !== body.payment_id || data.reason !== body.reason))) throw new Error();
      }
      intent = undefined;needsReview = true;
      message('detail-message','Snapshot sebelum tindakan. Muat ulang detail untuk mengambil status dan saldo terbaru.');
      message('queue-message','Ada perubahan pesanan. Muat ulang antrean untuk memperbarui kartu.');
      const transaction = command.kind === 'payments'?'Pembayaran '+data.payment_id+' · '+ui.money(data.amount):command.kind === 'refunds'?'Refund '+data.refund_id+' · '+ui.money(data.amount):'Perubahan status';
      message('command-result',transaction+(data.replayed?' sebelumnya sudah tercatat; hasil idempotent ditemukan tanpa pencatatan ulang.':' berhasil dicatat.')+' Muat ulang detail untuk meninjau saldo dan status terbaru.','success');
    } catch (_) {
      // A later rejection cannot disprove an earlier commit with a lost response.
      needsReview = true;
      message('detail-message','Snapshot sebelum permintaan. Tindakan lain dikunci sampai hasil terkonfirmasi.');
      message('command-result','Hasil belum dapat dipastikan. Jangan membuat transaksi pengganti. Periksa / kirim ulang dengan kunci dan isi yang sama; tidak ada retry otomatis. Tetap buka tab ini.','error');
    } finally {clearTimeout(timer);sending = false;controls();(intent?el('retry-command'):el('refresh-detail')).focus();}
  }
  el('queue-filter').addEventListener('submit',event => {event.preventDefault();if (listBusy || detailBusy || intent || sending) return;filter = {search:el('queue-search').value.trim(),status:el('queue-status').value};loadOrders(0);});
  el('queue-prev').addEventListener('click',() => loadOrders(Math.max(0,offset-20)));el('queue-next').addEventListener('click',() => loadOrders(offset+20));el('refresh-orders').addEventListener('click',() => loadOrders());
  el('refresh-detail').addEventListener('click',() => loadDetail(el('refresh-detail').dataset.order));
  el('close-detail').addEventListener('click',() => {if (detailBusy || intent || sending) return;el('order-detail').hidden = true;order = undefined;el('refresh-orders').focus();});
  el('payment-form').addEventListener('submit',event => {event.preventDefault();if (!order || intent || sending || detailBusy || needsReview) return;
    try {const amount = model.amount(el('payment-amount').value,order.balance), method = el('payment-method').value;if (!model.methods.includes(method)) throw new Error('Pilih metode pembayaran yang sah.');begin('payments',{amount,method});}
    catch (error) {message('command-result',error.message,'error');el('payment-amount').focus();}
  });
  el('cancel-form').addEventListener('submit',event => {event.preventDefault();if (!order || !validReason(el('cancel-reason').value)) {message('command-result','Isi alasan pembatalan, maksimal 250 karakter tanpa karakter kontrol.','error');return;}begin('cancel',{version:order.version,reason:el('cancel-reason').value.trim()});});
  el('refund-form').addEventListener('submit',event => {event.preventDefault();if (!selectedRefund || !validReason(el('refund-reason').value)) {message('command-result','Isi alasan refund, maksimal 250 karakter tanpa karakter kontrol.','error');return;}begin('refunds',{payment_id:selectedRefund.payment_id,reason:el('refund-reason').value.trim()});});
  el('cancel-refund').addEventListener('click',() => {if (intent || sending || detailBusy) return;selectedRefund = undefined;controls();});
  el('retry-command').addEventListener('click',sendCommand);
  el('more-payments').addEventListener('click',async () => {
    if (!order || detailBusy || intent || sending || needsReview) return;detailBusy = true;controls();
    try {const rows = await read('/api/pesanan/'+encodeURIComponent(order.id_pesanan)+'/pembayaran?limit=20&offset='+(paymentOffset+20));if (!Array.isArray(rows)) throw new Error();rows.forEach(row => model.payment(row,order.id_pesanan));paymentOffset += 20;payments.push(...rows);paymentMore = rows.length === 20;renderPayments();}
    catch (_) {message('detail-message','Riwayat berikutnya belum dapat dimuat. Coba kembali.','error');}
    finally {detailBusy = false;controls();}
  });
  document.addEventListener('click',event => {if (!intent && !sending) return;const link = event.target.closest('a');if ((link && link.target !== '_blank') || event.target.closest('#logout')) {event.preventDefault();event.stopImmediatePropagation();message('command-result','Pastikan hasil permintaan dengan kunci yang sama sebelum meninggalkan halaman.','error');}},true);
  window.addEventListener('beforeunload',event => {if ((!intent && !sending) || el('private-content').hidden) return;event.preventDefault();event.returnValue = '';});
  (async () => {try {role = (await auth.get()).role;await loadOrders(0);} catch (_) {message('queue-message','Sesi belum dapat diperiksa. Gunakan pemeriksaan sesi untuk mencoba kembali.','error');}})();
})();
