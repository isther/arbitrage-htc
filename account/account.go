package account

type Account struct {
	ExchangeInfo
	Balance
	OrderList
	CntOutputer
}

func NewAccount() *Account {
	counter := NewCounter()
	return &Account{
		ExchangeInfo: NewexchangeInfoer(),
		Balance:      NewBALANCE(),
		OrderList:    NewORDER(counter),
		CntOutputer:  counter,
	}
}

func (a *Account) Run() {
	a.ExchangeInfo.Run()
	a.Balance.Run()
	a.OrderList.Run()
}
