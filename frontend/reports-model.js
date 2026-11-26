(() => {
  'use strict';
  const zone = 'Asia/Makassar', day = 86400000;
  const statuses = ['received','processing','ready','collected','cancelled'];
  const methods = {cash:'Tunai',bank_transfer:'Transfer bank',qris:'QRIS',unknown:'Metode belum dicatat'};
  const fail = () => { throw new Error('Data laporan tidak valid atau melampaui batas tampilan. Muat ulang atau hubungi pemilik.'); };
  const integer = (v,signed = false) => { if (!Number.isSafeInteger(v) || (!signed && v < 0)) fail();return v; };
  const text = v => { if (typeof v !== 'string') fail();return v; };
  const instant = v => { if (typeof v !== 'string' || !Number.isFinite(Date.parse(v))) fail();return v; };
  const date = at => new Intl.DateTimeFormat('en-CA',{timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date(at));
  const time = at => new Intl.DateTimeFormat('id-ID',{timeZone:zone,dateStyle:'medium',timeStyle:'short'}).format(new Date(instant(at)))+' WITA';
  const sum = values => { const v = Number(values.reduce((n,x) => n+BigInt(integer(x,true)),0n));return integer(v,true); };
  function period(first,last) {
    const parse = raw => { if (!/^\d{4}-\d{2}-\d{2}$/.test(raw) || raw.startsWith('0000')) throw new Error('Isi tanggal awal dan akhir yang sah.');const n = Date.parse(raw+'T00:00:00+08:00');if (!Number.isFinite(n) || date(n) !== raw) throw new Error('Tanggal tidak valid.');return n; };
    const from = parse(first), to = parse(last)+day;
    if (to <= from || to-from > 366*day || new Date(to).getUTCFullYear() > 9999) throw new Error('Pilih periode berurutan, maksimal 366 hari termasuk tanggal akhir.');
    return Object.freeze({from:new Date(from).toISOString(),to:new Date(to).toISOString(),first,last});
  }
  function meta(v,p) {
    if (!v || !text(v.id) || v.timezone !== zone || Date.parse(instant(v.from)) !== Date.parse(p.from) || Date.parse(instant(v.to)) !== Date.parse(p.to)) fail();instant(v.as_of);return v;
  }
  function dashboard(v,p) {
    meta(v,p);
    for (const k of ['created','received','processing','ready','overdue','receivable_orders','receivable']) integer(v.orders[k]);
    for (const k of ['payments','refunds','expenses','cash_in','cash_out','unassigned_payments','unassigned_refunds']) integer(v.money[k]);
    integer(v.money.net,true);if (v.money.net !== sum([v.money.payments,-v.money.refunds])) fail();
    for (const k of ['opened','closed','active','system','physical']) integer(v.shifts[k]);integer(v.shifts.difference,true);
    if (v.shifts.difference !== sum([v.shifts.physical,-v.shifts.system]) || !Array.isArray(v.methods) || v.methods.length !== 4 || new Set(v.methods.map(x => x.method)).size !== 4 || !Array.isArray(v.popular_services)) fail();
    for (const m of v.methods) { if (!Object.hasOwn(methods,m.method)) fail();integer(m.payments);integer(m.refunds); }
    for (const s of v.popular_services) { text(s.name);text(s.service_id);if (!['kg','item'].includes(s.unit)) fail();for (const k of ['orders','pieces','grams','gross_before_discount']) integer(s[k]); }
    return v;
  }
  function page(v,p,kind,offset,limit,status = '') {
    meta(v,p);
    if (v.kind !== kind || v.offset !== offset || v.limit !== limit || typeof v.has_more !== 'boolean' || !Array.isArray(v.items) || v.items.length > limit) fail();
    for (const x of v.items) {
      text(x.id);
      if (kind === 'ledger') { if (!['DEBIT','KREDIT'].includes(x.type) || (x.method !== '' && !Object.hasOwn(methods,x.method))) fail();integer(x.amount);text(x.description);instant(x.occurred_at); }
      else {text(x.customer_name);if (!statuses.includes(x.status) || (status && x.status !== status)) fail();integer(x.total);integer(x.paid);integer(x.due);if (x.status === 'cancelled'?(x.paid !== 0 || x.due !== 0):sum([x.paid,x.due]) !== x.total) fail();instant(x.created_at);instant(x.estimated_ready_at);}
    }
    return v;
  }
  const totalKeys = ['cash_payments','cash_refunds','cash_in','cash_out','bank_payments','bank_refunds','qris_payments','qris_refunds','unknown_in','unknown_out'];
  function totals(v) { for (const k of totalKeys) integer(v[k]);return v; }
  const receipts = t => sum([t.cash_payments,t.bank_payments,t.qris_payments,-t.cash_refunds,-t.bank_refunds,-t.qris_refunds]);
  function shift(v,actor) {
    if (!v || !text(v.id) || !text(v.cashier_id) || (actor && v.cashier_id !== actor) || !['open','closed'].includes(v.status)) fail();
    integer(v.opening);integer(v.expected);instant(v.opened_at);totals(v.totals);
    if (v.expected !== sum([v.opening,v.totals.cash_payments,v.totals.cash_in,-v.totals.cash_refunds,-v.totals.cash_out])) fail();
    if (v.status === 'closed') {instant(v.closed_at);integer(v.physical);integer(v.difference,true);if (v.difference !== sum([v.physical,-v.expected])) fail();}
    else if (v.closed_at !== null || v.physical !== null || v.difference !== null) fail();
    return v;
  }
  function daily(v,selected,session) {
    if (v.date !== selected || !Array.isArray(v.shifts)) fail();totals(v.totals);totals(v.unassigned);
    for (const x of v.shifts) shift(x,session.role === 'cashier'?session.user_id:undefined);
    return v;
  }
  window.ULaundryReports = Object.freeze({zone,date,time,period,meta,dashboard,page,shift,daily,totals,receipts,sum,integer,methods,statuses});
})();
