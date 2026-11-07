-- Optional synthetic data, never part of schema migrations or server startup.
-- Run only on an EMPTY disposable database with a name ending in _demo.
-- psql -v ON_ERROR_STOP=1 -d ulaundry_demo -f scripts/demo.sql
BEGIN;
DO $$
BEGIN
    IF current_database() NOT LIKE '%\_demo' ESCAPE '\' THEN
        RAISE EXCEPTION 'Demo requires a disposable database ending in _demo';
    END IF;
    IF (SELECT count(*) FROM public.schema_migrations) <> 4 THEN
        RAISE EXCEPTION 'Apply schema migrations first';
    END IF;
    LOCK TABLE public.pesanan, public.kasir, public.keuangan IN EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM public.pesanan) OR EXISTS (SELECT 1 FROM public.kasir)
        OR EXISTS (SELECT 1 FROM public.keuangan)
        OR EXISTS (SELECT 1 FROM public.customers) OR EXISTS (SELECT 1 FROM public.services)
        OR EXISTS (SELECT 1 FROM public.staff) OR EXISTS (SELECT 1 FROM public.orders)
        OR EXISTS (SELECT 1 FROM public.order_items) OR EXISTS (SELECT 1 FROM public.payments)
        OR EXISTS (SELECT 1 FROM public.ledger_entries) OR EXISTS (SELECT 1 FROM public.audit_events) THEN
        RAISE EXCEPTION 'Demo requires empty business tables; no existing data changed';
    END IF;
END $$;
INSERT INTO public.pesanan (id_pesanan,nama_customer,layanan,total_biaya,status)
VALUES ('DEMO-001','Pelanggan Contoh','Cuci Setrika',12000,'Diproses');
INSERT INTO public.keuangan (tipe,keterangan,nominal)
VALUES ('KREDIT','Contoh pembelian sabun',45000);
-- No account/password is seeded. Secure owner bootstrap is a separate task.
COMMIT;
