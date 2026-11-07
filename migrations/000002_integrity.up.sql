-- Existing invalid values cause the entire up transaction to roll back.
-- Do not repair, delete, clamp or deduplicate user data here.
ALTER TABLE public.pesanan
    ALTER COLUMN total_biaya SET NOT NULL,
    ADD CONSTRAINT pesanan_total_biaya_nonnegative CHECK (total_biaya >= 0),
    ALTER COLUMN waktu_pesan SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN waktu_pesan SET NOT NULL,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE public.kasir
    ALTER COLUMN username SET NOT NULL,
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

-- The original init.sql already has this unique constraint. Do not duplicate it.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attname = 'username'
        WHERE c.conrelid = 'public.kasir'::regclass AND c.contype IN ('u', 'p')
          AND c.conkey = ARRAY[a.attnum]
    ) THEN
        ALTER TABLE public.kasir ADD CONSTRAINT kasir_username_key UNIQUE (username);
    END IF;
END $$;

ALTER TABLE public.keuangan
    ALTER COLUMN nominal SET NOT NULL,
    ADD CONSTRAINT keuangan_nominal_nonnegative CHECK (nominal >= 0),
    ALTER COLUMN tipe SET NOT NULL,
    ADD CONSTRAINT keuangan_tipe_allowed CHECK (tipe IN ('DEBIT', 'KREDIT')),
    ALTER COLUMN tanggal SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN tanggal SET NOT NULL,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN id_pesanan VARCHAR(50),
    ADD CONSTRAINT keuangan_pesanan_fk FOREIGN KEY (id_pesanan)
        REFERENCES public.pesanan (id_pesanan) ON UPDATE RESTRICT ON DELETE RESTRICT;

-- Existing timestamp columns retain their type and values. New metadata uses TZ.
CREATE FUNCTION public.ulaundry_touch_updated_at() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END $$;

CREATE TRIGGER pesanan_updated_at BEFORE UPDATE ON public.pesanan
    FOR EACH ROW EXECUTE FUNCTION public.ulaundry_touch_updated_at();
CREATE TRIGGER kasir_updated_at BEFORE UPDATE ON public.kasir
    FOR EACH ROW EXECUTE FUNCTION public.ulaundry_touch_updated_at();
CREATE TRIGGER keuangan_updated_at BEFORE UPDATE ON public.keuangan
    FOR EACH ROW EXECUTE FUNCTION public.ulaundry_touch_updated_at();

-- GetNeraca: WHERE tipe = ... and SUM(nominal). Planner may prefer a seq scan.
CREATE INDEX keuangan_tipe_nominal_idx ON public.keuangan (tipe) INCLUDE (nominal);
-- Parent FK checks on update/delete must locate linked ledger rows efficiently.
CREATE INDEX keuangan_id_pesanan_idx ON public.keuangan (id_pesanan)
    WHERE id_pesanan IS NOT NULL;
