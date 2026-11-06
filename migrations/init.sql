-- Skema Database ULaundry (PostgreSQL)

-- 1. Tabel Pesanan
CREATE TABLE IF NOT EXISTS pesanan (
    id_pesanan VARCHAR(50) PRIMARY KEY,
    nama_customer VARCHAR(100) NOT NULL,
    layanan VARCHAR(50) NOT NULL,
    total_biaya INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    waktu_pesan TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    waktu_perkiraan_selesai TIMESTAMP,
    waktu_selesai_real TIMESTAMP
);

-- 2. Tabel Kasir (Untuk Auth JWT)
CREATE TABLE IF NOT EXISTS kasir (
    id_kasir SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    nama_lengkap VARCHAR(100) NOT NULL
);

-- 3. Tabel Keuangan (Buku Kas & Neraca)
CREATE TABLE IF NOT EXISTS keuangan (
    id_transaksi SERIAL PRIMARY KEY,
    tanggal TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tipe VARCHAR(10) NOT NULL, -- 'DEBIT' atau 'KREDIT'
    keterangan TEXT NOT NULL,
    nominal INT NOT NULL
);