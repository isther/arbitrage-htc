package account

type Account struct {
	ExchangeInfo
	Balance
	OrderList
}

func NewAccount(
	isFuture *bool,
	useBNB *bool,
	bnbMinQty *float64,
	autoBuyBNB *bool,
	autoBuyBNBQty *float64, // U
) *Account {
	return &Account{
		ExchangeInfo: NewexchangeInfoer(isFuture),
		Balance:      NewBALANCE(isFuture, useBNB, bnbMinQty, autoBuyBNB, autoBuyBNBQty),
		OrderList:    NewORDER(isFuture),
	}
}

func (a *Account) Run() {
	a.ExchangeInfo.Run()
	a.Balance.Run()
	a.OrderList.Run()
}
