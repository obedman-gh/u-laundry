# ULaundry POS & Management System 👕

ULaundry adalah sistem *Point of Sales* (POS) dan *Enterprise Resource Planning* (ERP) mini yang dirancang khusus untuk operasional bisnis laundry. Dibangun dengan pendekatan *Clean Architecture* menggunakan Go (Golang) dan PostgreSQL, sistem ini menangani alur kerja pesanan, manajemen kasir dengan keamanan JWT, pencetakan struk termal otomatis, hingga pembukuan neraca keuangan (*ledger*) secara *real-time*.

## ✨ Fitur Utama

- **Order Workflow Management**: Pelacakan status pesanan secara *real-time* dari antrean hingga selesai.
- **Secure Authentication**: Sistem login kasir menggunakan enkripsi kata sandi **Bcrypt** dan otorisasi API berbasis **JSON Web Token (JWT)**.
- **Automated Ledger & Accounting**: Pencatatan otomatis arus kas (Debit/Kredit) setiap kali pesanan dilunasi, lengkap dengan kalkulasi neraca akhir.
- **Thermal Receipt Integration**: *Endpoint* khusus yang menghasilkan format HTML/CSS responsif yang dioptimalkan untuk langsung ditembak ke *printer thermal* kasir (58mm/80mm).
- **Public Tracking System**: Akses aman bagi pelanggan untuk melacak status cucian dan memantau estimasi waktu selesai.
- **Cloudflare Tunnels Ready**: Arsitektur jaringan yang mendukung eksposur lokal ke internet publik tanpa konfigurasi *port-forwarding* router (CGNAT-friendly).

## 🛠️ Tech Stack

- **Backend**: Go (Golang) standard library (`net/http`)
- **Database**: PostgreSQL (Ubuntu-based)
- **Security**: `golang.org/x/crypto/bcrypt`, `github.com/golang-jwt/jwt/v5`
- **Frontend**: HTML5, Vanilla JavaScript, CSS3
- **Deployment & Networking**: Cloudflare Zero Trust (Tunnels)

## 🚀 Cara Menjalankan (Local Development)

### 1. Prasyarat
- Go 1.20+ terinstal
- PostgreSQL berjalan di sistem atau VM lokal

### 2. Kloning Repositori
```bash
git clone [https://github.com/obedman-gh/u-laundry.git](https://github.com/obedman-gh/u-laundry.git)
cd u-laundry