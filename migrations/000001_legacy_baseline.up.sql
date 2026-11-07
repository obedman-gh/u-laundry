-- Adopt the known legacy schema without replacing tables or rows.
CREATE TABLE IF NOT EXISTS public.pesanan (
    id_pesanan VARCHAR(50) PRIMARY KEY,
    nama_customer VARCHAR(100) NOT NULL,
    layanan VARCHAR(50) NOT NULL,
    total_biaya INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    waktu_pesan TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    waktu_perkiraan_selesai TIMESTAMP,
    waktu_selesai_real TIMESTAMP
);

CREATE TABLE IF NOT EXISTS public.kasir (
    id_kasir SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    nama_lengkap VARCHAR(100) NOT NULL
);

CREATE TABLE IF NOT EXISTS public.keuangan (
    id_transaksi SERIAL PRIMARY KEY,
    tanggal TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tipe VARCHAR(10) NOT NULL,
    keterangan TEXT NOT NULL,
    nominal INT NOT NULL
);

-- Also covers the older schema served by cmd/legacy/upgrade-pesanan.
-- Missing historical timestamps receive adoption time, not an invented order time.
ALTER TABLE public.pesanan
    ADD COLUMN IF NOT EXISTS waktu_pesan TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS waktu_perkiraan_selesai TIMESTAMP,
    ADD COLUMN IF NOT EXISTS waktu_selesai_real TIMESTAMP;

-- IF NOT EXISTS is adoption, not permission to accept an unknown money type.
-- An unexpected schema needs inspection and a separately reviewed migration.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'public.pesanan'::regclass AND attname = 'total_biaya'
          AND atttypid = 'integer'::regtype AND NOT attisdropped
    ) OR NOT EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'public.keuangan'::regclass AND attname = 'nominal'
          AND atttypid = 'integer'::regtype AND NOT attisdropped
    ) THEN
        RAISE EXCEPTION 'Unsupported legacy monetary column type' USING ERRCODE = '42804';
    END IF;
END $$;
