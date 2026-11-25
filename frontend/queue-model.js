(() => {
  'use strict';
  const statuses = ['received','processing','ready','collected','cancelled'];
  const methods = ['cash','bank_transfer','qris'];
  const manager = role => role === 'owner' || role === 'admin';
  const open = order => ['received','processing','ready'].includes(order.status);
  function validate(order) {
    if (!order || typeof order.id_pesanan !== 'string' || !/^[A-Za-z0-9_.-]{1,100}$/.test(order.id_pesanan) || typeof order.nama_customer !== 'string' || !statuses.includes(order.status) || !['paid','partially_paid','unpaid'].includes(order.payment_status) || !Number.isSafeInteger(order.version) || order.version < 1 || ![order.total_biaya,order.paid,order.balance].every(v => Number.isSafeInteger(v) && v >= 0) || order.paid + order.balance !== order.total_biaya || !Number.isFinite(Date.parse(order.waktu_perkiraan_selesai))) throw new Error('Respons pesanan tidak dapat digunakan.');
    return order;
  }
  function actions(order,role) {
    return {next:order.status === 'received'?'processing':order.status === 'processing'?'ready':order.status === 'ready' && order.balance === 0?'collected':'', cancel:manager(role) && ['received','processing'].includes(order.status) && order.paid === 0, payment:open(order) && order.balance > 0, refund:manager(role) && open(order)};
  }
  function amount(raw,balance) {
    const value = Number(raw);
    if (!/^[0-9]+$/.test(raw) || !Number.isSafeInteger(value) || value < 1 || value > balance) throw new Error('Isi rupiah bulat positif, maksimal sisa tagihan.');
    return value;
  }
  function payment(value,id) {
    if (!value || typeof value.payment_id !== 'string' || !value.payment_id || value.order_id !== id || !Number.isSafeInteger(value.amount) || value.amount < 1 || !methods.includes(value.method) || !['posted','refunded'].includes(value.status) || !Number.isFinite(Date.parse(value.received_at))) throw new Error('Respons pembayaran tidak dapat digunakan.');
    return value;
  }
  function detail(value) {
    validate(value);
    if (!Array.isArray(value.items) || !value.items.length || ![value.subtotal,value.discount].every(v => Number.isSafeInteger(v) && v >= 0) || value.subtotal-value.discount !== value.total_biaya || !Number.isFinite(Date.parse(value.waktu_pesan))) throw new Error('Rincian pesanan tidak dapat digunakan.');
    for (const item of value.items) if (typeof item.service_name !== 'string' || !['kg','item'].includes(item.unit) || ![item.unit_price,item.line_total,item.grams,item.pieces].every(v => Number.isSafeInteger(v) && v >= 0)) throw new Error('Rincian layanan tidak dapat digunakan.');
    return value;
  }
  const overdue = (order,now = Date.now()) => ['received','processing'].includes(order.status) && Date.parse(order.waktu_perkiraan_selesai) < now;
  window.ULaundryQueue = Object.freeze({validate,detail,payment,actions,amount,overdue,methods,manager});
})();
