package tron

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/lbtsm/gotron-sdk/pkg/address"
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

			m.checkEnergy(snap.Energies)
			for _, ct := range snap.ContractToken {
				m.checkToken(common.HexToAddress(ct.Address), ct.Tokens)
			}

			if !chain.SleepWithStop(m.Stop, config.BalanceRetryInterval) {
				return errors.New("polling terminated")
			}
		}
	}
}

func (m *Monitor) checkBalance(form, group string, waterLine *big.Int, report bool) {
	// get account balance
	account, err := m.conn.cli.GetAccount(form)
	if err != nil {
		m.Log.Error("CheckBalance GetAccount failed", "account", form, "err", err)
		return
	}
	balance, _ := big.NewFloat(0).Quo(big.NewFloat(0).SetInt64(account.Balance), wei).Float64()
	m.Log.Info("CheckBalance, account detail", "account", form, "balance", balance)
	if balance < float64(waterLine.Int64()) {
		util.Alarm(context.Background(),
			fmt.Sprintf("Balance Less than %d Balance,chains=%s group=%s addr=%s balance=%0.4f",
				waterLine.Int64(), m.Cfg.Name, group, form, balance))
		return
	}

}

func (m *Monitor) checkEnergy(energies []config.Energy) {
	for _, ele := range energies {
		resource, err := m.conn.cli.GetAccountResource(ele.Address)
		if err != nil {
			m.Log.Error("CheckEnergy GetAccountResource failed", "account", ele.Address, "err", err)
			continue
		}
		m.Log.Info("CheckEnergy, account detail", "account", ele.Address, "energy", resource.EnergyLimit, "used", resource.EnergyUsed)
		if (resource.EnergyLimit - resource.EnergyUsed) < ele.Waterline {
			util.Alarm(context.Background(),
				fmt.Sprintf("Energy Less than %d,chains=%s addr=%s energy=%d", ele.Waterline, m.Cfg.Name, ele.Address, resource.EnergyLimit-resource.EnergyUsed))
			continue
		}
	}
}

// hexToTronBase58 converts an ethereum hex address (0x...) to tron base58 format.
func hexToTronBase58(hexAddr string) string {
	tronHex := "41" + strings.TrimPrefix(strings.ToLower(hexAddr), "0x")
	return address.HexToAddress(tronHex).String()
}

func (m *Monitor) checkToken(contract common.Address, tokens []config.EthToken) {
	holderBase58 := hexToTronBase58(contract.Hex())
	for _, tk := range tokens {
		tokenBase58 := hexToTronBase58(tk.Addr)
		ret, err := m.conn.cli.TRC20ContractBalance(holderBase58, tokenBase58)
		if err != nil {
			m.Log.Error("CheckToken TRC20ContractBalance failed", "err", err, "token", tk.Name)
			continue
		}

		wei := tk.Wei
		if wei == 0 {
			wei = 18
		}

		retF, _ := ret.Float64()
		overage, _ := big.NewFloat(0).Quo(big.NewFloat(retF), util.ToWeiFloat(int64(1), int(wei))).Float64()
		m.Log.Info("Get Token result", "token", tk.Name, "overage", overage, "addr", tk.Addr)
		if overage < tk.WaterLine {
			util.Alarm(context.Background(),
				fmt.Sprintf("Token Less than %0.4f waterLine ,chains=%s token=%s addr=%s overage=%0.4f", tk.WaterLine, m.Cfg.Name, tk.Name, contract, overage))
		}
	}
}
