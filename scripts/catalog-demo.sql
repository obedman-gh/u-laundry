-- Synthetic catalog only. Never run on an operational database.
-- psql -v ON_ERROR_STOP=1 -d ulaundry_demo -f scripts/catalog-demo.sql
BEGIN;
DO $$
BEGIN
    IF current_database() NOT LIKE '%\_demo' ESCAPE '\' THEN
        RAISE EXCEPTION 'Catalog demo requires a disposable database ending in _demo';
    END IF;
    IF (SELECT count(*) FROM public.schema_migrations) <> 8 THEN
        RAISE EXCEPTION 'Apply schema version 8 before catalog demo';
    END IF;
    LOCK TABLE public.services,public.service_revisions IN EXCLUSIVE MODE;
    IF EXISTS(SELECT 1 FROM public.services) OR EXISTS(SELECT 1 FROM public.service_revisions)
        OR EXISTS(SELECT 1 FROM public.customers) OR EXISTS(SELECT 1 FROM public.orders)
        OR EXISTS(SELECT 1 FROM public.payments) OR EXISTS(SELECT 1 FROM public.ledger_entries)
        OR EXISTS(SELECT 1 FROM public.pesanan) OR EXISTS(SELECT 1 FROM public.keuangan) THEN
        RAISE EXCEPTION 'Catalog demo requires empty business data; no existing data changed';
    END IF;
END $$;
INSERT INTO public.services(id,name,unit,unit_price,turnaround_ns,active,display_order,created_at,updated_at)
VALUES ('DEMO-WASH','Contoh Cuci Setrika','kg',7000,86400000000000,true,10,now(),now()),
       ('DEMO-BLANKET','Contoh Selimut','piece',20000,172800000000000,true,20,now(),now()),
       ('DEMO-IRON','Contoh Setrika','kg',5000,86400000000000,false,30,now(),now());
INSERT INTO public.service_revisions(service_id,version,name,unit,unit_price,turnaround_ns,active,display_order,changed_at,source)
SELECT id,version,name,unit,unit_price,turnaround_ns,active,display_order,updated_at,'demo' FROM public.services;
-- No accounts, credentials, customers, orders, payments or personal data seeded.
COMMIT;
