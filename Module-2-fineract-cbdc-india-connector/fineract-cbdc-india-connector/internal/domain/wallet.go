package domain

// Wallet identifies a CBDC holder account at the sponsor bank.
type Wallet struct {
	ID       string `json:"id"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

// Balance is a point-in-time balance snapshot for a wallet. Amounts are held as
// strings to avoid floating-point rounding on monetary values.
type Balance struct {
	WalletID  string `json:"wallet_id"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
	Currency  string `json:"currency"`
}
