package models

type Kasir struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	NamaLengkap string `json:"nama_lengkap,omitempty"`
}
