(() => {
  'use strict';
  const el = id => document.getElementById(id),ui = window.ULaundryUI,auth = window.ULaundrySession,model = window.ULaundryPOSModel;
  let session, customer, catalog = new Map(), lines = [], serial = 0, customerOffset = 0, customerLast = true, searchTerm = '', catalogOffset = 0, catalogLast = true;
  // Pending commands keep their original serialized body and key until a
  // confirmed result. An uncertain write can only be explicitly replayed.
  let busy = false, phase = 'editing', pending, customerUncertain = false, dirty = false;
  const message = (id,text,kind = 'info') => ui.message(el(id),text,kind);
  const node = (tag,text) => { const element = document.createElement(tag); if (text !== undefined) element.textContent = text; return element; };
  const wita = value => new Intl.DateTimeFormat('id-ID',{dateStyle:'medium',timeStyle:'short',timeZone:'Asia/Makassar'}).format(new Date(value))+' WITA';
  function clearErrors() {
    el('pos-workspace').querySelectorAll('[aria-invalid]').forEach(field => field.removeAttribute('aria-invalid'));
    el('pos-workspace').querySelectorAll('.field-error').forEach(error => {error.hidden = true;error.textContent = '';});
  }
  function invalid(error,region = 'pos-message') {
    const field = el(error.field),feedback = el(error.field+'-error');
    if (field) field.setAttribute('aria-invalid','true');
    if (feedback) { feedback.hidden = false; feedback.textContent = error.message; }
    message(region,error.message || 'Periksa isian sebelum mencoba kembali.','error');
    if (field) field.focus();
  }
  function controls() {
    const locked = busy || phase !== 'editing';
    for (const field of el('pos-workspace').querySelectorAll('input,select,textarea,button')) field.disabled = locked;
    el('discount-fields').disabled = locked || !['owner','admin'].includes(session?.role);
    el('customer-previous').disabled = locked || customerOffset === 0;
    el('customer-next').disabled = locked || customerLast;
    el('create-customer').disabled = locked || customerUncertain;
    el('catalog-more').hidden = catalogLast;
    el('catalog-more').disabled = locked;
    el('add-item').disabled = locked || lines.length >= 100 || catalog.size === 0;
    el('save-order').hidden = phase !== 'editing';
    el('retry-order').hidden = phase !== 'uncertain'; el('retry-order').disabled = busy;
    el('edit-order').hidden = phase !== 'rejected'; el('edit-order').disabled = busy;
    el('pending-help').hidden = !['sending','uncertain'].includes(phase);
    el('inspect-orders').hidden = !['uncertain','rejected'].includes(phase);
    el('pos-form').setAttribute('aria-busy',String(busy));
    el('pos-customer').setAttribute('aria-busy',String(busy));
    for (const row of lines) el('remove-'+row.key).disabled = locked || lines.length === 1;
  }
  async function api(path,options = {}) {
    const controller = options.signal?undefined:new AbortController(),timeout = controller?setTimeout(()=>controller.abort(),15000):undefined;
    try {
      const response = await auth.fetch(path,{...options,signal:options.signal||controller.signal,headers:{...(options.body?{'Content-Type':'application/json'}:{}),...options.headers}});
      if (!response.ok) {
        const texts = {400:'Isian belum memenuhi ketentuan server. Periksa pelanggan, satuan, jumlah, diskon dan estimasi.',403:'Permintaan tidak diizinkan. Periksa hak akses dan sesi Anda.',404:'Pelanggan atau layanan tidak ditemukan. Muat ulang data.',409:'Data atau layanan telah berubah. Periksa antrean dan muat ulang harga sebelum memperbaiki pesanan.',429:'Permintaan terlalu sering. Tunggu sebentar sebelum mencoba kembali.'};
        throw Object.assign(new Error(texts[response.status] || 'Hasil permintaan belum dapat dipastikan.'),{status:response.status});
      }
      return await response.json();
    } finally { if (timeout) clearTimeout(timeout); }
  }
  function choose(record) {
    customer = record; dirty = true;
    el('selected-customer').textContent = 'Pelanggan: '+record.name+(record.contact?' / '+record.contact:'');
    el('customer-results').replaceChildren(); el('customer-page').textContent = '';customerLast = true;
    message('customer-message','Pelanggan dipilih. Lanjutkan ke layanan.','success'); controls();
    if (lines.length) el('service-'+lines[0].key).focus();
  }
  async function searchCustomers() {
    el('customer-results').replaceChildren(); message('customer-message','Mencari pelanggan…','loading');
    const query = new URLSearchParams({q:searchTerm,limit:'20',offset:String(customerOffset)});
    const records = await api('/api/customers?'+query);
    if (!Array.isArray(records) || records.some(record => !model.id(record.customer_id) || typeof record.name !== 'string' || record.contact !== undefined && typeof record.contact !== 'string')) throw new Error('Daftar pelanggan belum dapat digunakan.');
    for (const record of records) {
      const row = node('li'),button = node('button',record.name+(record.contact?' / '+record.contact:''));button.type = 'button';button.className = 'secondary';
      button.addEventListener('click',() => { if (!busy && phase === 'editing') choose(record); }); row.appendChild(button);el('customer-results').appendChild(row);
    }
    if (!records.length) el('customer-results').appendChild(node('li','Pelanggan tidak ditemukan. Periksa nama/kontak atau tambahkan pelanggan baru.'));
    customerLast = records.length < 20;
    el('customer-page').textContent = records.length?'Hasil '+(customerOffset+1)+'–'+(customerOffset+records.length):'Tidak ada hasil.';
    if (customerUncertain) { customerUncertain = false;message('customer-message','Hasil pencarian dimuat. Pilih pelanggan yang telah tersimpan; buat baru hanya setelah memastikan tidak ada akun pelanggan yang sama.'); }
    else message('customer-message','Pilih pelanggan dari hasil pencarian.');
  }
  async function loadCatalog(append = false) {
    message('catalog-message','Memuat layanan dan harga…','loading');
    const offset = append?catalogOffset+100:0,records = await api('/api/services?limit=100&offset='+offset);
    if (!Array.isArray(records) || records.some(record => !model.id(record.service_id) || typeof record.name !== 'string' || !['kg','item'].includes(record.unit) || record.active !== true || !Number.isSafeInteger(record.unit_price) || record.unit_price<0 || !Number.isSafeInteger(record.turnaround_minutes) || record.turnaround_minutes<1)) throw new Error('Katalog belum dapat digunakan pada tampilan ini.');
    const next = append?new Map(catalog):new Map(); for (const service of records) next.set(service.service_id,service);
    catalog = next; catalogOffset = offset; catalogLast = records.length < 100;
    for (const line of lines) fillServices(line);
    message('catalog-message',catalog.size?'Harga katalog dimuat. Total final diperiksa server saat disimpan.':'Belum ada layanan aktif. Minta pemilik/admin menyiapkan layanan.');
    preview();
  }
  function fillServices(line) {
    const select = el('service-'+line.key);select.replaceChildren();
    const placeholder = node('option','Pilih layanan');placeholder.value = '';select.appendChild(placeholder);
    for (const service of catalog.values()) { const option = node('option',service.name+' — '+ui.money(service.unit_price)+'/'+(service.unit==='kg'?'kg':'item'));option.value = service.service_id;select.appendChild(option); }
    if (line.serviceId && !catalog.has(line.serviceId)) { const missing = node('option','Layanan belum tersedia di katalog yang dimuat — pilih ulang');missing.value = line.serviceId;select.appendChild(missing); }
    select.value = line.serviceId;
    describeQuantity(line);
  }
  function describeQuantity(line) {
    const service = catalog.get(line.serviceId),weight = service?.unit === 'kg';
    el('quantity-label-'+line.key).textContent = weight?'Berat (kg)':'Jumlah (item)';
    el('quantity-'+line.key).inputMode = weight?'decimal':'numeric';
    el('quantity-help-'+line.key).textContent = weight?'Minimal 0,001 kg; maksimal tiga angka desimal.':'Bilangan bulat, minimal 1 item.';
    el('price-'+line.key).textContent = service?'Harga: '+ui.money(service.unit_price)+'/'+(weight?'kg':'item'):'Pilih layanan untuk melihat harga.';
  }
  function numberLines() {
    el('pos-items').querySelectorAll('fieldset').forEach((row,index) => {
      row.querySelector('legend').textContent = 'Layanan '+(index+1);
      row.querySelector('button').setAttribute('aria-label','Hapus baris layanan '+(index+1));
    });
  }
  function addLine(focus = false) {
    if (lines.length >= 100) return;
    const line = {key:String(++serial),serviceId:'',quantity:'1'};lines.push(line);
    const row = node('fieldset');row.dataset.line = line.key;row.appendChild(node('legend','Layanan '+lines.length));
    const fields = node('div');fields.className = 'pos-line-fields';
    const serviceBox = node('div'),serviceLabel = node('label','Layanan'),select = node('select');select.id = 'service-'+line.key;serviceLabel.htmlFor = select.id;select.setAttribute('aria-describedby',select.id+'-error');select.required = true;serviceBox.append(serviceLabel,select);
    const serviceError = node('small');serviceError.id = select.id+'-error';serviceError.className = 'field-error';serviceError.hidden = true;serviceBox.appendChild(serviceError);
    const quantityBox = node('div'),label = node('label'),quantity = node('input'),help = node('small'),error = node('small');
    label.id = 'quantity-label-'+line.key;quantity.id = 'quantity-'+line.key;label.htmlFor = quantity.id;quantity.value = line.quantity;quantity.required = true;quantity.maxLength = 17;quantity.autocomplete = 'off';help.id = 'quantity-help-'+line.key;error.id = quantity.id+'-error';error.className = 'field-error';error.hidden = true;quantity.setAttribute('aria-describedby',help.id+' '+error.id);quantityBox.append(label,quantity,help,error);fields.append(serviceBox,quantityBox);row.appendChild(fields);
    const prices = node('div'),price = node('span'),total = node('output');prices.className = 'pos-line-price';price.id = 'price-'+line.key;total.id = 'line-total-'+line.key;total.setAttribute('for',select.id+' '+quantity.id);total.textContent = 'Subtotal: —';prices.append(price,total);row.appendChild(prices);
    const remove = node('button','Hapus baris');remove.id = 'remove-'+line.key;remove.type = 'button';remove.className = 'secondary';remove.setAttribute('aria-label','Hapus baris layanan '+lines.length);row.appendChild(remove);el('pos-items').appendChild(row);fillServices(line);
    select.addEventListener('change',() => { if (busy || phase !== 'editing') return;line.serviceId = select.value;line.quantity = '1';quantity.value = '1';dirty = true;clearErrors();describeQuantity(line);preview(); });
    quantity.addEventListener('input',() => { if (busy || phase !== 'editing') return;line.quantity = quantity.value;dirty = true;clearErrors();message('pos-message','');preview(); });
    remove.addEventListener('click',() => { if (busy || phase !== 'editing' || lines.length === 1) return;lines = lines.filter(item => item !== line);row.remove();numberLines();dirty = true;preview();controls();el('add-item').focus(); });
    controls();if (focus) select.focus();
  }
  function draft() { return {customer,lines,discount:el('pos-discount').value,discountReason:el('discount-reason').value,readyAt:el('ready-at').value,notes:el('pos-notes').value}; }
  function preview() {
    // This arithmetic is a catalog estimate only. No price/total is submitted,
    // and the success view exclusively uses the server's persisted snapshot.
    for (const line of lines) {
      try { const value = model.calculate([line],catalog);el('line-total-'+line.key).textContent = 'Subtotal: '+ui.money(value.subtotal); }
      catch (_) { el('line-total-'+line.key).textContent = 'Subtotal: —'; }
    }
    try {
      const value = model.calculate(lines,catalog),discount = model.integer(el('pos-discount').value,'pos-discount');
      el('preview-subtotal').textContent = ui.money(value.subtotal);el('preview-discount').textContent = ui.money(discount);
      el('preview-total').textContent = discount<=value.subtotal?ui.money(value.subtotal-discount):'Periksa diskon';
      const ready = model.estimate(el('ready-at').value,Date.now()) || Date.now()+value.minutes*60000;
      el('preview-ready').textContent = Number.isFinite(new Date(ready).getTime())?wita(ready):'Durasi di luar batas tampilan';
    } catch (_) { el('preview-subtotal').textContent = el('preview-total').textContent = el('preview-ready').textContent = '—'; }
  }
  async function run(task,region) {
    if (busy || phase !== 'editing') return;
    const active = document.activeElement;let completed = false;
    busy = true;controls();
    try { await task();completed = true; }
    catch (error) { message(region,error.status?error.message:(navigator.onLine?'Data belum dapat dimuat. Pilih coba kembali dengan koneksi yang tersedia.':'Perangkat sedang offline. Sambungkan koneksi lalu coba kembali.'),'error'); }
    finally {
      busy = false;controls();
      if (completed && region === 'customer-message') (el('customer-results').querySelector('button') || el('customer-query')).focus();
      else if (active?.isConnected && !active.disabled) active.focus();
    }
  }
  function key() {
    const bytes = new Uint8Array(24);window.crypto.getRandomValues(bytes);
    return 'order-'+Array.from(bytes,value=>value.toString(16).padStart(2,'0')).join('');
  }
  function showResult(data) {
    const order = model.result(data);
    el('result-customer').textContent = 'Pelanggan: '+order.nama_customer;el('result-id').textContent = 'Nomor pesanan: '+order.id_pesanan;
    el('result-items').replaceChildren();
    for (const item of order.items) {
      const quantity = item.unit==='kg'?(BigInt(item.grams)/1000n)+','+(BigInt(item.grams)%1000n).toString().padStart(3,'0')+' kg':item.pieces+' item';
      el('result-items').appendChild(node('li',item.service_name+' / '+quantity+' × '+ui.money(item.unit_price)+'/'+(item.unit==='kg'?'kg':'item')+' = '+ui.money(item.line_total)));
    }
    el('result-subtotal').textContent = ui.money(order.subtotal);el('result-discount').textContent = ui.money(order.discount);el('result-total').textContent = ui.money(order.total_biaya);el('result-ready').textContent = wita(order.waktu_perkiraan_selesai);
    el('result-status').textContent = ui.statusLabel(order.status)+' / '+({unpaid:'Belum dibayar',partially_paid:'Dibayar sebagian',paid:'Lunas'})[order.payment_status];
    const changed = order.total_biaya !== pending.total || order.subtotal !== pending.subtotal;
    message('result-message',(data.replayed?'Pesanan sebelumnya ditemukan; tidak membuat pesanan baru.':'Pesanan berhasil disimpan.')+(changed?' Harga final server berbeda dari perkiraan. Gunakan total final di bawah.':''),changed?'info':'success');
    el('receipt-link').href = '/api/pesanan/'+encodeURIComponent(order.id_pesanan)+'/struk';
    const tracking = !data.replayed && typeof data.tracking_token === 'string' && /^[A-Za-z0-9_-]{43}$/.test(data.tracking_token);
    el('tracking-link').hidden = !tracking;
    if (tracking) el('tracking-link').href = '/tracking.html#'+data.tracking_token;else el('tracking-link').removeAttribute('href');
    el('tracking-message').textContent = tracking?'Tautan tracking tersedia untuk pelanggan; struk berisi data internal dan memerlukan sesi staf.':'Token tracking tidak tersedia pada respons ini. Jika diperlukan, buat tautan baru melalui halaman pelanggan setelah memeriksa pesanan.';
    phase = 'success';pending = undefined;dirty = false;customer = undefined;lines = [];el('pos-items').replaceChildren();el('customer-results').replaceChildren();el('customer-create').reset();el('customer-query').value = '';el('pos-form').reset();el('selected-customer').textContent = '';el('pos-workspace').hidden = true;el('pos-result').hidden = false;el('result-title').focus();
  }
  async function sendOrder() {
    if (busy || !pending || !['editing','uncertain'].includes(phase)) return;
    if (!navigator.onLine) { message('pos-message','Perangkat sedang offline. Pesanan tidak dikirim. Sambungkan koneksi lalu pilih coba kembali.','offline');return; }
    const wasUncertain = pending.uncertain;busy = true;phase = 'sending';controls();message('pos-message','Menyimpan dan memeriksa total di server…','loading');
    const controller = new AbortController(),timeout = setTimeout(() => controller.abort(),20000);
    try {
      const data = await api('/api/pesanan',{method:'POST',headers:{'Idempotency-Key':pending.key},body:pending.body,signal:controller.signal});
      showResult(data);
    } catch (error) {
      if (!wasUncertain && [400,403,404,409,413,415,429].includes(error.status)) {
        phase = 'rejected';message('pos-message',error.message || 'Permintaan ditolak. Periksa data sebelum mencoba kembali.','error');
      } else {
        pending.uncertain = true;phase = 'uncertain';message('pos-message','Hasil pesanan belum dapat dipastikan. Jangan membuat pesanan baru. Pilih Periksa hasil / coba kembali agar server mencari pesanan yang sama.','error');
      }
    } finally {
      clearTimeout(timeout);busy = false;
      if (phase !== 'success') {
        controls();
        if (phase === 'uncertain') el('retry-order').focus();
        else if (phase === 'rejected') el('edit-order').focus();
      }
    }
  }
  el('customer-search').addEventListener('submit',event => {event.preventDefault();if (busy || phase !== 'editing') return;searchTerm = el('customer-query').value.trim();customerOffset = 0;run(searchCustomers,'customer-message');});
  for (const [id,delta] of [['customer-previous',-20],['customer-next',20]]) el(id).addEventListener('click',() => {if (busy || phase !== 'editing') return;customerOffset = Math.max(0,customerOffset+delta);run(searchCustomers,'customer-message');});
  el('customer-create').addEventListener('submit',async event => {
    event.preventDefault();if (busy || phase !== 'editing' || customerUncertain) return;clearErrors();
    if (!navigator.onLine) {message('customer-message','Perangkat sedang offline. Pelanggan tidak dikirim.','offline');return;}
    let body;
    try {
      const name = model.text(el('customer-name').value,100,'customer-name',true),contact = el('customer-contact').value.trim();
      let normalized = contact.replace(/[ ()-]/g,'');if (normalized.startsWith('0')) normalized = '+62'+normalized.slice(1);else if (normalized.startsWith('62')) normalized = '+'+normalized;
      if (contact && !/^\+[1-9][0-9]{7,14}$/.test(normalized)) throw Object.assign(new Error('Isi nomor kontak yang valid atau kosongkan.'),{field:'customer-contact'});
      body = {name,contact};
    } catch (error) {invalid(error,'customer-message');return;}
    customer = undefined;dirty = true;el('selected-customer').textContent = 'Belum ada pelanggan terpilih.';
    busy = true;controls();message('customer-message','Menyimpan pelanggan…','loading');
    try {
      const saved = await api('/api/customers',{method:'POST',body:JSON.stringify(body)});
      if (!model.id(saved.customer_id) || typeof saved.name !== 'string') throw new Error('Respons pelanggan belum dapat dipastikan.');
      el('customer-create').reset();el('new-customer').open = false;busy = false;choose(saved);
    } catch (error) {
      // Customer creation has no idempotency API: search, never auto-retry.
      if (![400,403,404,413,415,429].includes(error.status)) customerUncertain = true;
      message('customer-message',customerUncertain?'Hasil penyimpanan pelanggan belum dapat dipastikan. Cari nama/kontaknya dan pilih hasil yang tersimpan sebelum membuat pelanggan baru.':error.message,'error');
      if (customerUncertain) {el('customer-query').value = body.name;searchTerm = body.name;}
    } finally {busy = false;controls();if (customerUncertain) el('customer-query').focus();}
  });
  el('catalog-reload').addEventListener('click',() => run(()=>loadCatalog(),'catalog-message'));
  el('catalog-more').addEventListener('click',() => run(()=>loadCatalog(true),'catalog-message'));
  el('add-item').addEventListener('click',() => {if (!busy && phase === 'editing') {addLine(true);dirty = true;preview();}});
  for (const id of ['pos-discount','discount-reason','ready-at','pos-notes']) el(id).addEventListener('input',() => {if (phase === 'editing') {dirty = true;clearErrors();message('pos-message','');preview();}});
  el('pos-form').addEventListener('submit',async event => {
    event.preventDefault();if (busy || phase !== 'editing' || pending) return;clearErrors();
    if (!navigator.onLine) {message('pos-message','Perangkat sedang offline. Pesanan tidak dikirim; isian tetap tersedia.','offline');return;}
    try { const built = model.build(draft(),catalog,session.role);pending = {key:key(),body:JSON.stringify(built.payload),total:built.total,subtotal:built.subtotal,uncertain:false}; }
    catch (error) {invalid(error);return;}
    await sendOrder();
  });
  el('retry-order').addEventListener('click',sendOrder);
  el('edit-order').addEventListener('click',() => {if (busy || phase !== 'rejected') return;pending = undefined;phase = 'editing';controls();message('pos-message','Permintaan ditolak server. Periksa isian dan muat ulang harga sebelum menyimpan kembali.');el('catalog-reload').focus();});
  el('new-order').addEventListener('click',() => {
    if (phase !== 'success') return;
    phase = 'editing';customer = undefined;customerOffset = 0;customerLast = true;pending = undefined;
    el('pos-result').hidden = true;el('result-items').replaceChildren();
    for (const id of ['result-customer','result-id','result-message','result-subtotal','result-discount','result-total','result-ready','result-status','tracking-message']) el(id).textContent = '';
    el('receipt-link').removeAttribute('href');el('tracking-link').removeAttribute('href');el('tracking-link').hidden = true;
    el('pos-workspace').hidden = false;el('pos-form').reset();el('selected-customer').textContent = 'Belum ada pelanggan terpilih.';
    message('pos-message','');message('customer-message','Cari pelanggan untuk pesanan berikutnya.');
    addLine();preview();controls();el('customer-query').focus();
  });
  window.addEventListener('beforeunload',event => {if ((pending || dirty) && !el('private-content').hidden) {event.preventDefault();event.returnValue = '';}});
  document.addEventListener('click',event => {
    const trigger = event.target.closest?.('a, #logout');
    if (!pending || !trigger || trigger.id !== 'logout' && (trigger.target === '_blank' || trigger.hash)) return;
    event.preventDefault();event.stopPropagation();
    message('pos-message','Pastikan hasil pesanan terlebih dahulu. Gunakan Periksa hasil / coba kembali atau periksa antrean di tab lain.','error');
    if (phase === 'uncertain') el('retry-order').focus();
    else if (phase === 'rejected') el('edit-order').focus();
  },true);
  async function bootstrap() {
    ui.state(el('pos-state'),{kind:'loading',title:'Menyiapkan pesanan',description:'Memuat sesi dan katalog layanan…'});
    try {
      session = await auth.get();el('discount-fields').hidden = !['owner','admin'].includes(session.role);await loadCatalog();
      if (!lines.length) addLine();controls();preview();el('pos-workspace').hidden = false;el('pos-state').replaceChildren();el('pos-state').setAttribute('aria-busy','false');
    } catch (_) {ui.state(el('pos-state'),{kind:navigator.onLine?'error':'offline',title:'POS belum dapat dimuat',description:'Periksa koneksi lalu coba kembali.',retry:bootstrap});}
  }
  bootstrap();
})();
