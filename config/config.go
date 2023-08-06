package config

import (
	"bytes"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
)

var (
	Config *AppConfig = &AppConfig{
		Binance:  &Binance{},
		Telegram: &Telegram{},
		Mode:     &Mode{},
		Symbols:  &Symbols{},
		Fee: &Fee{
			UseBNB:        false,
			BNBMinQty:     0.0,
			AutoBuyBNB:    false,
			AutoBuyBNBQty: 0.0,
		},
		Params: &Params{
			CycleNumber:             0,
			RatioMax:                0,
			RatioMin:                0,
			RatioProfit:             0,
			CloseTimeOut:            0,
			PauseMinKlineRatio:      0,
			PauseMaxKlineRatio:      0,
			PauseClientTimeOutLimit: 0,
			WaitDuration:            0,
			MaxQty:                  "0.0",
		},
		DingDingLogConfig: &DingDing{},
		DingDingErrConfig: &DingDing{},
	}
)

func (c *AppConfig) Load(filename string) {
	viper.SetConfigFile(filename)
	if err := viper.ReadInConfig(); err != nil {
		logrus.Errorf("Read config error: %v, create a new config file", err.Error())
		config, err := yaml.Marshal(Config)
		if err != nil {
			logrus.Error("Marshal and save config error")
		}
		err = viper.ReadConfig(bytes.NewBuffer(config))
		if err != nil {
			panic("Read config error")
		}
		viper.SafeWriteConfig()
	}

	viper.Unmarshal(c)
	viper.WatchConfig()
}

func (c *AppConfig) Save(filename string) {
	res, err := yaml.Marshal(Config)
	if err != nil {
		panic("Marshal config error")
	}
	viper.ReadConfig(bytes.NewBuffer(res))
	viper.WriteConfig()
}

// AppConfig defines the config of the server
type AppConfig struct {
	*Binance
	*Telegram
	*Mode
	*Symbols
	*Fee
	*Params
	DingDingLogConfig *DingDing
	DingDingErrConfig *DingDing
}

type Binance struct {
	Api    string
	Secret string
}

type Mode struct {
	UI        bool
	IsFOK     bool
	IsFuture  bool
	OnlyMode1 bool
}

type Symbols struct {
	BookTickerASymbol string
	StableCoinSymbol  string
	BookTickerBSymbol string
}

type Fee struct {
	UseBNB        bool
	BNBMinQty     float64
	AutoBuyBNB    bool
	AutoBuyBNBQty float64 // U
}

type Params struct {
	CycleNumber             int
	RatioMax                float64 // base 10000
	RatioMin                float64 // base 10000
	RatioProfit             float64 // base 10000
	CloseTimeOut            int64   // ms
	PauseMinKlineRatio      float64 // base 10000
	PauseMaxKlineRatio      float64 // base 10000
	PauseClientTimeOutLimit int64   // ms
	WaitDuration            int64   // ms
	MaxQty                  string
}

type DingDing struct {
	AccessToken string
	Secrect     string
}

type Telegram struct {
	Token string
}
