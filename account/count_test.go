package account

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

func TestCounter(t *testing.T) {
	var now = time.Now()
	viper.Set("MaxQty", "1.0")

	counter := NewCounter()
	counter.AddOrder(decimal.NewFromFloat(1.0), now.Add(time.Minute))
	counter.AddOrder(decimal.NewFromFloat(2.0), now.Add(20*time.Second))
	counter.AddOrder(decimal.NewFromFloat(1.0), now.Add(3*time.Minute))
	counter.AddOrder(decimal.NewFromFloat(1.0), now.Add(4*time.Minute))
	fmt.Println(counter.Chart())
}
