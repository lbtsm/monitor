package xrp

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/lbtsm/xrpl-go/model/client/account"
	"github.com/lbtsm/xrpl-go/model/transactions/types"
	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/mapprotocol/monitor/pkg/util"
	"github.com/pkg/errors"
)

var (
	wei = big.NewFloat(1000000)
)

type Monitor struct {
	*chain.Common
	conn                  *Connection
	heightCount           int64
	balance, syncedHeight *big.Int
	timestamp             int64
	balMapping            map[string]float64
}

func NewMonitor(cs *chain.Common, tronConn *Connection) *Monitor {
	return &Monitor{
		Common:       cs,
		conn:         tronConn,
		balance:      new(big.Int),
		syncedHeight: new(big.Int),
		balMapping:   make(map[string]float64),
	}
}

func (m *Monitor) Sync() error {
	m.Log.Debug("Starting listener...")
	m.Wg.Add(1)
	go func() {
		defer m.Wg.Done()
		if err := m.sync(); err != nil {
			m.Log.Error("Polling Account balance failed", "err", err)
		}
	}()

	return nil
}

func (m *Monitor) sync() error {
	for {
		select {
		case <-m.Stop:
			return errors.New("polling terminated")
		default:
			snap := m.Snapshot()
			waterLine, ok := new(big.Int).SetString(snap.WaterLine, 10)
			if !ok {
				m.SysErr <- fmt.Errorf("%s waterLine Not Number", snap.Name)
				return nil
			}

			for _, ele := range snap.From {
				if ele == "" {
					continue
				}
				m.checkBalance(ele, "unknown", waterLine, true)
			}

			for _, ele := range snap.Users {
				wl, ok := new(big.Int).SetString(ele.WaterLine, 10)
				if !ok {
					m.SysErr <- fmt.Errorf("%s waterLine Not Number", snap.Name)
					return nil
				}
				for _, addr := range strings.Split(ele.From, ",") {
					m.checkBalance(addr, ele.Group, wl, false)
				}
			}
			if !chain.SleepWithStop(m.Stop, config.BalanceRetryInterval) {
				return errors.New("polling terminated")
			}
		}
	}
}

func (m *Monitor) checkBalance(form, group string, waterLine *big.Int, report bool) {
	// get account balance
	account, _, err := m.conn.cli.Account.AccountInfo(
		&account.AccountInfoRequest{Account: types.Address(form)})
	if err != nil {
		m.Log.Error("CheckBalance GetAccount failed", "account", form, "err", err)
		return
	}

	balance, _ := big.NewFloat(0).Quo(big.NewFloat(0).SetInt64(int64(account.AccountData.Balance)),
		wei).Float64()
	m.Log.Info("CheckBalance, account detail", "account", form, "balance", balance, "waterLine", waterLine)
	if balance < float64(waterLine.Int64()) {
		util.Alarm(context.Background(),
			fmt.Sprintf("Balance Less than %d Balance,chains=%s group=%s addr=%s balance=%0.4f",
				waterLine.Int64(), m.Cfg.Name, group, form, balance))
	}

}
