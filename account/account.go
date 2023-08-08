package account

type Account struct {
	ExchangeInfo
	Balance
	OrderList
}

func NewAccount() *Account {
	return &Account{
		ExchangeInfo: NewexchangeInfoer(),
		Balance:      NewBALANCE(),
		OrderList:    NewORDER(),
	}
}

func (a *Account) Run() {
	a.ExchangeInfo.Run()
	a.Balance.Run()
	a.OrderList.Run()
}
