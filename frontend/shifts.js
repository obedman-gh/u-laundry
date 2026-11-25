(() => {
  const el = id => document.getElementById(id);
  const message = (text, kind = 'info') => { const target = el('message'); if (window.ULaundryUI) window.ULaundryUI.message(target, text, kind); else target.textContent = text; };
  let session, active, busy = false, reportReady = false, selected = '', offset = 0, last = true;
  const pending = new Map(), limit = 20;
  const money = value => {
    if (!Number.isSafeInteger(value)) throw new Error('Nominal melampaui batas tampilan. Hubungi owner untuk pemeriksaan.');
    return new Intl.NumberFormat('id-ID', {style:'currency',currency:'IDR',maximumFractionDigits:0}).format(value);
  };
  const time = value => value ? new Intl.DateTimeFormat('id-ID',{dateStyle:'short',timeStyle:'short',timeZone:'Asia/Makassar'}).format(new Date(value)) : 'Masih aktif';
  const today = () => new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Makassar',year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date());
  function controls(value) {
    busy = value;
    el('workspace').setAttribute('aria-busy', String(value));
    document.querySelectorAll('main button, main input, main select').forEach(node => { node.disabled = value; });
    if (document.getElementById('logout')) document.getElementById('logout').disabled = value;
    el('print').disabled = value || !reportReady;
    el('previous').disabled = value || offset === 0;
    el('next').disabled = value || last;
  }
  async function api(path, options = {}) {
    let response;
    try { response = await fetch(path, {credentials:'same-origin',cache:'no-store',...options,
      headers:{...(options.body ? {'Content-Type':'application/json','X-CSRF-Token':session.csrf_token} : {}),...options.headers}}); }
    catch (_) { throw new Error('Koneksi terputus. Untuk transaksi yang belum mendapat kepastian, coba kembali dengan isian yang sama tanpa memuat ulang halaman.'); }
    const data = await response.json().catch(() => { throw new Error('Respons belum dapat dipastikan. Coba kembali dengan isian yang sama.'); });
    if (response.status === 401) { el('workspace').hidden = true; window.location.href='/login.html'; throw new Error('Sesi berakhir. Silakan masuk kembali.'); }
    if (!response.ok) { const error = new Error(data?.error?.message || 'Permintaan tidak dapat diproses.'); error.status = response.status; throw error; }
    return data;
  }
  function row(target, values) {
    const tr = document.createElement('tr');
    for (const value of values) { const td = document.createElement('td'); td.textContent=value; tr.appendChild(td); }
    target.appendChild(tr); return tr;
  }
  function totals(data) {
    el('totals').replaceChildren();
    for (const [name,inflow,outflow] of [['Pembayaran tunai',data.cash_payments,data.cash_refunds],['Transfer bank',data.bank_payments,data.bank_refunds],['QRIS',data.qris_payments,data.qris_refunds],['Mutasi laci kas',data.cash_in,data.cash_out],['Metode belum dicatat',data.unknown_in,data.unknown_out]]) {
      row(el('totals'),[name,money(inflow),money(outflow),money(inflow-outflow)]);
    }
  }
  async function refresh() {
    reportReady = false; el('print').disabled=true;
    const [current, report] = await Promise.all([api('/api/shifts/current'),api(`/api/shifts/daily?date=${encodeURIComponent(el('report-date').value)}`)]);
    active = current;
    el('open-form').hidden = Boolean(active); el('cash-form').hidden = !active; el('close-form').hidden = !active;
    el('current').textContent = active ? `Shift ${active.id}, dibuka ${time(active.opened_at)}. Saldo sistem tunai: ${money(active.expected)}.` : 'Belum ada shift aktif. Buka kas sebelum menerima atau mengembalikan pembayaran tunai.';
    totals(report.totals);
    const u = report.unassigned;
    el('unassigned').textContent = `Tanpa shift: tunai masuk ${money(u.cash_payments)}, refund tunai ${money(u.cash_refunds)}; transfer masuk ${money(u.bank_payments)}, refund ${money(u.bank_refunds)}; QRIS masuk ${money(u.qris_payments)}, refund ${money(u.qris_refunds)}. Metode belum dicatat: masuk ${money(u.unknown_in)}, keluar ${money(u.unknown_out)}. Transaksi lama tetap tanpa shift.`;
    el('report-title').textContent=`Tanggal ${report.date} WITA — ${session.role==='cashier' ? 'kasir sendiri' : 'semua kasir'}. Diperbarui ${time(new Date().toISOString())}.`;
    el('shift-list').replaceChildren();
    for (const shift of report.shifts) {
      const tr = row(el('shift-list'),[`${shift.cashier_name || shift.cashier_id} / ${shift.id}`,`${time(shift.opened_at)} / ${time(shift.closed_at)}`,money(shift.opening),money(shift.expected),shift.physical === null ? 'Belum dihitung' : money(shift.physical),shift.difference === null ? 'Belum ditutup' : money(shift.difference),`${shift.closed_by || 'Belum ditutup'} / ${shift.opening_notes || '-'} / ${shift.closing_notes || '-'}`]);
      const td = document.createElement('td'), button = document.createElement('button'); td.className='no-print';button.type='button';button.textContent='Lihat';button.addEventListener('click',() => inspect(shift.id));td.appendChild(button);tr.appendChild(td);
    }
    el('empty').hidden=report.shifts.length!==0;
    el('entries-panel').hidden=true; selected='';offset=0;
    reportReady=true;
  }
  async function entries() {
    const data = await api(`/api/shifts/${encodeURIComponent(selected)}/entries?limit=${limit}&offset=${offset}`);
    el('entries').replaceChildren();
    for (const e of data) row(el('entries'),[time(e.occurred_at),`${e.method} / ${e.type === 'DEBIT' ? 'Masuk' : 'Keluar'}`,money(e.amount),`${e.payment_id || e.refund_id || e.id} / ${e.order_id || '-'} / ${e.notes}`]);
    el('entries-title').textContent=`Shift ${selected}. Daftar ini dipaginasi; cetakan harian memakai ringkasan penuh di atas.`;
    el('page-info').textContent=data.length ? `Transaksi ${offset+1}–${offset+data.length}` : 'Belum ada transaksi.';
    last=data.length<limit; el('entries-panel').hidden=false;
  }
  async function inspect(id) {
    if (busy) return; controls(true);selected=id;offset=0;
    try { await entries();el('entries-panel').scrollIntoView({behavior:'smooth',block:'start'}); } catch(error) { message(error.message, 'error'); } finally { controls(false); }
  }
  async function command(name,path,body,form) {
    if (busy) return;
    if (name==='close' && !window.confirm('Tutup shift dengan hasil hitung ini? Penutupan tidak dapat diubah.')) return;
    const fingerprint=JSON.stringify({path,body});
    let attempt=pending.get(name);
    if (!attempt || attempt.fingerprint!==fingerprint) {
      const bytes=crypto.getRandomValues(new Uint8Array(16));
      attempt={fingerprint,key:Array.from(bytes,b => b.toString(16).padStart(2,'0')).join('')};pending.set(name,attempt);
    }
    controls(true);
    try {
      await api(path,{method:'POST',headers:{'Idempotency-Key':attempt.key},body:JSON.stringify(body)});
      pending.delete(name);form.reset();reportReady=false;
      message('Transaksi berhasil disimpan.');
      try { await refresh(); } catch(error) { message(`Transaksi berhasil disimpan. Ringkasan belum dapat diperbarui: ${error.message}`); }
    } catch(error) { if(error.status >= 400 && error.status < 500) pending.delete(name); message(error.message, 'error'); } finally { controls(false); }
  }
  const amount = id => { const value=Number(el(id).value);if(!Number.isSafeInteger(value)||value<0)throw new Error('Isi nominal rupiah bulat yang valid.');return value; };
  const submit = (id,fn) => el(id).addEventListener('submit',event => { event.preventDefault();try { fn(); } catch(error) { message(error.message, 'error'); } });
  submit('open-form',() => command('open','/api/shifts',{opening:amount('opening'),notes:el('opening-notes').value},el('open-form')));
  submit('cash-form',() => { if(active)command('cash',`/api/shifts/${encodeURIComponent(active.id)}/cash`,{amount:amount('amount'),direction:el('direction').value,notes:el('cash-notes').value},el('cash-form')); });
  submit('close-form',() => { if(active)command('close',`/api/shifts/${encodeURIComponent(active.id)}/close`,{physical:amount('physical'),notes:el('closing-notes').value},el('close-form')); });
  async function reload() { if(busy)return;controls(true);try{await refresh();message('Ringkasan diperbarui.');}catch(error){message(error.message, 'error');}finally{controls(false);} }
  submit('date-form',reload);el('reload').addEventListener('click',reload);
  el('report-date').addEventListener('change',() => {reportReady=false;el('print').disabled=true;});
  el('print').addEventListener('click',() => {if(reportReady&&!busy)window.print();});
  el('previous').addEventListener('click',async() => {if(busy||!selected||offset===0)return;controls(true);offset=Math.max(0,offset-limit);try{await entries();}catch(error){message(error.message, 'error');}finally{controls(false);}});
  el('next').addEventListener('click',async() => {if(busy||!selected||last)return;controls(true);offset+=limit;try{await entries();}catch(error){message(error.message, 'error');}finally{controls(false);}});
  window.addEventListener('offline',() => {reportReady=false;el('print').disabled=true;message('Sedang offline. Transaksi baru memerlukan koneksi; isian belum dikirim otomatis.');});
  window.addEventListener('beforeunload',event => {if(pending.size){event.preventDefault();event.returnValue='';}});
  (async() => {controls(true);try{session=await api('/api/session');window.ULaundryUI?.setSession(session);el('scope').textContent=session.role==='cashier'?'Kelola kas sendiri dan lihat ringkasan transaksi Anda.':'Kelola kas sendiri dan lihat ringkasan semua kasir.';el('report-date').value=today();el('workspace').hidden=false;await refresh();message('');}catch(error){message(error.message, 'error');}finally{controls(false);}})();
})();
