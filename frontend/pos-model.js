(() => {
  'use strict';
  const max = BigInt(Number.MAX_SAFE_INTEGER), id = value => typeof value === 'string' && /^[A-Za-z0-9_.-]{1,100}$/.test(value);
  function fail(field,message) { throw Object.assign(new Error(message),{field}); }
  function integer(value,field,zero = true) {
    if (!/^\d{1,16}$/.test(String(value))) fail(field,'Gunakan bilangan bulat tanpa pemisah ribuan.');
    const number = BigInt(value);
    if (number > max || number < (zero?0n:1n)) fail(field,'Nilai berada di luar batas yang dapat ditampilkan.');
    return Number(number);
  }
  function quantity(unit,value,field) {
    if (unit === 'item') return {pieces:integer(value,field,false)};
    const match = /^(\d{1,13})(?:[.,](\d{1,3}))?$/.exec(value);
    if (!match) fail(field,'Isi berat kg dengan maksimal tiga angka desimal, misalnya 2,500.');
    const grams = BigInt(match[1])*1000n+BigInt((match[2]||'').padEnd(3,'0'));
    if (grams < 1n || grams > max) fail(field,'Berat minimal 0,001 kg dan harus dalam batas tampilan.');
    return {grams:Number(grams)};
  }
  function text(value,maxLength,field,required = false,multiline = false) {
    const clean = value.replace(/\r\n/g,'\n').trim();
    const disallowed = multiline ? /[\p{Cc}\p{Cf}\uD800-\uDFFF]/u.test(clean.replace(/[\n\t]/g,'')) : /[\p{Cc}\p{Cf}\uD800-\uDFFF]/u.test(clean);
    if (required && !clean || Array.from(clean).length > maxLength || disallowed) fail(field,'Isi teks yang sesuai, maksimal '+maxLength+' karakter tanpa karakter kontrol.');
    return clean;
  }
  function calculate(lines,catalog) {
    if (!lines.length || lines.length > 100) fail('add-item','Pesanan harus berisi 1–100 baris layanan.');
    let subtotal = 0n, minutes = 0;
    const items = lines.map(line => {
      const service = catalog.get(line.serviceId);
      if (!service) fail('service-'+line.key,'Pilih layanan aktif dari katalog yang sudah dimuat.');
      const amount = quantity(service.unit,line.quantity,'quantity-'+line.key),price = BigInt(service.unit_price);
      const total = service.unit === 'kg' ? (price*BigInt(amount.grams)+500n)/1000n : price*BigInt(amount.pieces);
      subtotal += total;
      if (total > max || subtotal > max) fail('quantity-'+line.key,'Subtotal melampaui batas tampilan. Kurangi berat/jumlah.');
      minutes = Math.max(minutes,service.turnaround_minutes);
      return {request:{service_id:service.service_id,...amount},total:Number(total)};
    });
    return {subtotal:Number(subtotal),minutes,items};
  }
  function estimate(value,now) {
    if (!value) return undefined;
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) fail('ready-at','Isi tanggal dan jam WITA yang valid.');
    const parsed = Date.parse(value+':00+08:00');
    if (!Number.isFinite(parsed) || new Date(parsed+8*3600000).toISOString().slice(0,16) !== value || parsed <= now) fail('ready-at','Estimasi WITA harus sesudah waktu penerimaan saat ini.');
    return new Date(parsed).toISOString();
  }
  function build(draft,catalog,role,now = Date.now()) {
    if (!id(draft.customer?.customer_id)) fail('customer-query','Pilih pelanggan terlebih dahulu.');
    const calculated = calculate(draft.lines,catalog),discount = integer(draft.discount,'pos-discount');
    if (discount > 0 && !['owner','admin'].includes(role)) fail('pos-discount','Diskon hanya dapat diberikan pemilik atau admin.');
    if (discount > calculated.subtotal) fail('pos-discount','Diskon tidak boleh melebihi subtotal.');
    const reason = text(draft.discountReason,250,'discount-reason',discount > 0);
    if (!discount && reason) fail('discount-reason','Kosongkan alasan jika tidak ada diskon.');
    const payload = {customer_id:draft.customer.customer_id,items:calculated.items.map(item=>item.request),discount,notes:text(draft.notes,1000,'pos-notes',false,true)};
    if (discount) payload.discount_reason = reason;
    const ready = estimate(draft.readyAt,now);
    if (ready) payload.estimated_ready_at = ready;
    return {payload,subtotal:calculated.subtotal,total:calculated.subtotal-discount};
  }
  function result(data) {
    const order = data?.order,money = value => Number.isSafeInteger(value) && value >= 0;
    if (!order || !id(order.id_pesanan) || typeof order.nama_customer !== 'string' || !['received','processing','ready','collected','cancelled'].includes(order.status) || !['unpaid','partially_paid','paid'].includes(order.payment_status) || !Number.isFinite(Date.parse(order.waktu_perkiraan_selesai)) || ![order.subtotal,order.discount,order.total_biaya].every(money) || !Array.isArray(order.items) || !order.items.length || order.items.length > 100 || typeof data.replayed !== 'boolean') throw new Error('Respons pesanan belum dapat dipastikan.');
    let subtotal = 0n;
    for (const item of order.items) {
      if (!id(item.service_id) || typeof item.service_name !== 'string' || !['kg','item'].includes(item.unit) || !money(item.unit_price) || !money(item.line_total) || !money(item.grams) || !money(item.pieces) || (item.unit === 'kg' ? item.grams < 1 || item.pieces !== 0 : item.pieces < 1 || item.grams !== 0)) throw new Error('Rincian pesanan belum dapat dipastikan.');
      subtotal += BigInt(item.line_total);
    }
    if (subtotal !== BigInt(order.subtotal) || BigInt(order.subtotal)-BigInt(order.discount) !== BigInt(order.total_biaya)) throw new Error('Total pesanan belum dapat dipastikan.');
    return order;
  }
  window.ULaundryPOSModel = Object.freeze({id,integer,quantity,text,calculate,estimate,build,result});
})();
