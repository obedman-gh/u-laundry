package models

// Struct untuk Pesanan
type Pesanan struct {
	IDPesanan             string  `json:"id_pesanan"`
	NamaCustomer          string  `json:"nama_customer"`
	Layanan               string  `json:"layanan"`
	TotalBiaya            int     `json:"total_biaya"`
	Status                string  `json:"status"`
	WaktuPesan            string  `json:"waktu_pesan"`
	WaktuPerkiraanSelesai *string `json:"waktu_perkiraan_selesai"` // Pake bintang (*) karena bisa NULL
	WaktuSelesaiReal      *string `json:"waktu_selesai_real"`      // Pake bintang (*) karena bisa NULL
}
