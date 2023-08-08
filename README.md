# Arbitrage-htc

Binance...

## Start

1. 安装docker docker-compose

```bash
sudo apt upgrade -y
sudo install docker docker-compose cmake -y
```

2. 创建.env文件, 填写环境变量

```txt
TG_TOKEN= tgtoken
BINANCE_API= binanceapi
BINANCE_SECRET= binancesecret
```

3. 启动

```bash
sudo make docker-run
```

4. 修改参数

在TG中使用机器人修改参数

5. Start task

## TG机器人指令

```
/tstart 启动task
/tstop 停止task
/settings 参数设置
/account 账户余额
/task 任务信息
/log_enable 开启日志
/log_disable 关闭日志
```
