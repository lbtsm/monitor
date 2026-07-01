package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/cockroachdb/errors"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	mapoabi "github.com/lbtsm/mapo-lib/abi"
	abiJson "github.com/mapprotocol/monitor/internal/abi"
	"github.com/mapprotocol/monitor/internal/chain"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/mapprotocol/monitor/internal/mapprotocol"
	"github.com/mapprotocol/monitor/pkg/mempool"
	"github.com/mapprotocol/monitor/pkg/util"
)

var dece = big.NewInt(1000000000000000000)

type Monitor struct {
	*chain.Common
	heightCount           int64
	balance, syncedHeight *big.Int
	timestamp             int64
	balMapping            map[string]float64
}

func New(cs *chain.Common) *Monitor {
	return &Monitor{
		Common:       cs,
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

// prepareTick takes a Snapshot of the live OptConfig and parses the chain-
// level WaterLine. It returns ok=false when WaterLine is malformed so the
// caller can shut the chain down. It is invoked at the top of every poll
// iteration so reconfigs (waterLine / users / from / contractToken) take
// effect on the next tick without restarting the goroutine.
func (m *Monitor) prepareTick() (config.OptConfig, *big.Int, bool) {
	snap := m.Snapshot()
	wl, ok := config.ParseNativeWaterLine(snap.WaterLine, 18)
	if !ok {
		return snap, nil, false
	}
	return snap, wl, true
}

func (m *Monitor) sync() error {
	for {
		select {
		case <-m.Stop:
			return errors.New("polling terminated")
		default:
			snap, waterLine, ok := m.prepareTick()
			if !ok {
				m.SysErr <- fmt.Errorf("%s waterLine Not Number", snap.Name)
				return nil
			}

			// rebuild balMapping each tick so user add/remove reload takes effect
			m.balMapping = make(map[string]float64)
			for _, from := range snap.From {
				m.balMapping[from] = 0
			}
			for _, user := range snap.Users {
				for _, from := range strings.Split(user.From, ",") {
					m.balMapping[from] = 0
				}
			}

			for _, ele := range snap.From {
				if ele == "" {
					continue
				}
				m.checkBalance(common.HexToAddress(ele), waterLine, "unknown", false)
			}

			for _, user := range snap.Users {
				wl, ok := config.ParseNativeWaterLine(user.WaterLine, 18)
				if !ok {
					m.SysErr <- fmt.Errorf("%s waterLine Not Number", snap.Name)
					return nil
				}
				for _, from := range strings.Split(user.From, ",") {
					m.checkBalance(common.HexToAddress(from), wl, user.Group, false)
				}
			}

			for _, ct := range snap.ContractToken {
				m.checkToken(common.HexToAddress(ct.Address), ct.Tokens)
			}

			if snap.Id == snap.MapChainID {
				m.mapCheck()
			} else {
				m.OtherChainCheck()
			}

			if !chain.SleepWithStop(m.Stop, config.BalanceRetryInterval) {
				return errors.New("polling terminated")
			}
		}
	}
}

func (m *Monitor) reportUser() {
	if m.timestamp != 0 && time.Now().Unix()-m.timestamp < 86400 {
		return
	}
	var (
		now                = make(map[string]float64)
		yesTotal, nowTotal = float64(0), float64(0)
	)

	for addr, yesHave := range m.balMapping {
		if addr == "" || addr == config.ZeroAddress.String() {
			continue
		}
		balance, err := m.Conn.Client().BalanceAt(context.Background(), common.HexToAddress(addr), nil)
		if err != nil {
			m.Log.Error("Unable to get user balance failed", "from", addr, "err", err)
			chain.SleepWithStop(m.Stop, config.RetryLongInterval)
			return
		}

		bal := float64(new(big.Int).Div(balance, config.Wei).Int64()) / float64(config.Wei.Int64())
		now[addr] = bal
		nowTotal += bal
		yesTotal += yesHave
	}

	//if time.Now().Unix()-m.timestamp > 86400 {
	//	util.Alarm(context.Background(),
	//		fmt.Sprintf("Report balance detail,chains=%s,yesterday=%0.4f,now=%0.4f",
	//			m.Cfg.Name, yesTotal, nowTotal))
	//}

	m.timestamp = time.Now().Unix()
	m.balMapping = make(map[string]float64)
	m.balMapping = now
}

func (m *Monitor) checkBalance(addr common.Address, waterLine *big.Int, group string, report bool) {
	balance, err := m.Conn.Client().BalanceAt(context.Background(), addr, nil)
	if err != nil {
		m.Log.Error("Unable to get user balance failed", "from", addr, "err", err)
		chain.SleepWithStop(m.Stop, config.RetryLongInterval)
		return
	}

	if balance.Cmp(m.balance) != 0 {
		m.balance = balance
	}

	wl := float64(new(big.Int).Div(waterLine, config.Wei).Int64()) / float64(config.Wei.Int64())
	bal := float64(new(big.Int).Div(balance, config.Wei).Int64()) / float64(config.Wei.Int64())
	m.Log.Info("Get balance result", "account", addr, "balance", bal, "wl", wl, "balance", balance)
	if balance.Cmp(waterLine) == -1 {
		// alarm
		util.Alarm(context.Background(),
			fmt.Sprintf("Balance Less than %0.4f Balance,chains=%s group=%s addr=%s balance=%0.4f", wl, m.Cfg.Name, group, addr, bal))
	}

	now := time.Now().UTC()
	if report && now.Weekday() == time.Monday && now.Hour() == 11 && now.Minute() == 10 {
		util.Alarm(context.Background(),
			fmt.Sprintf("Report Address Balance have,chains=%s addr=%s balance=%0.4f,waterLine=%0.4f", m.Cfg.Name, addr, bal, wl))
	}
}

func (m *Monitor) checkToken(contract common.Address, tokens []config.EthToken) {
	for _, tk := range tokens {
		ad := common.HexToAddress(tk.Addr)
		input, err := mapprotocol.TokenAbi.PackInput("balanceOf", contract)
		if err != nil {
			continue
		}
		outPut, err := m.Conn.Client().CallContract(context.Background(),
			ethereum.CallMsg{To: &ad, Data: input}, nil)
		if err != nil {
			m.Log.Error("CheckToken callContract failed", "err", err.Error(), "to", ad)
			continue
		}

		var ret *big.Int
		if err = mapprotocol.TokenAbi.UnpackOutput("balanceOf", &ret, outPut); err != nil {
			m.Log.Error("CheckToken unpack failed", "err", err.Error())
			continue
		}

		wei := tk.Wei
		if wei == 0 {
			wei = 18
		}

		retF, _ := ret.Float64()
		overage, _ := big.NewFloat(0).Quo(big.NewFloat(retF), util.ToWeiFloat(int64(1), int(wei))).Float64()
		m.Log.Info("Get Token result", "token", tk.Name, "contract", contract, "overage", overage, "addr", tk.Addr)
		if overage < tk.WaterLine {
			// alarm
			util.Alarm(context.Background(),
				fmt.Sprintf("Token Less than %0.4f,chains=%s token=%s addr=%s overage=%0.4f ", tk.WaterLine, m.Cfg.Name, tk.Name, contract, overage))
		}
	}
}

func (m *Monitor) mapCheck() {
	for idx, contract := range m.Cfg.Tk.Contracts {
		if m.Cfg.Tk.Token[idx] == "btc" {
			m.nativeCheck(contract)
			continue
		}
		contractAmount, err := mapprotocol.TotalSupply(contract)
		if err != nil {
			m.Log.Error("Check brc20 balance, get amount by contract", "token", m.Cfg.Tk.Token[idx], "err", err)
			continue
		}
		contractAmount = contractAmount.Div(contractAmount, dece)

		lockAmount, err := mapprotocol.BalanceOf(contract, common.HexToAddress(m.Cfg.Tk.MapBridge))
		if err != nil {
			m.Log.Error("Check brc20 balance, get lock amount by contract", "token", m.Cfg.Tk.Token[idx], "err", err)
			continue
		}
		lockAmount = lockAmount.Div(lockAmount, dece)

		afterBridgeBal, err := GetMulAddBalance(m.Cfg.Genni.Endpoint, m.Cfg.Genni.Key, m.Cfg.Tk.BridgeAddr, m.Cfg.Tk.Token[idx])
		if err != nil {
			m.Log.Error("Check brc20 balance, get amount by genii", "token", m.Cfg.Tk.Token[idx], "err", err)
			continue
		}
		if m.Cfg.Tk.Token[idx] == "roup" {
			afterBridgeBal = afterBridgeBal + 900000
		}
		m.Log.Info("Check brc20 balance, get amount", "token", m.Cfg.Tk.Token[idx], "bridgeBal", afterBridgeBal,
			"contractAmount", contractAmount, "lockAmount", lockAmount)
		if afterBridgeBal < (contractAmount.Int64() - lockAmount.Int64()) {
			util.Alarm(context.Background(), fmt.Sprintf("check brc20 balance token=%s, bridgeBal=%d, contractAmount=%v",
				m.Cfg.Tk.Token[idx], afterBridgeBal, contractAmount))
		}
		if !chain.SleepWithStop(m.Stop, time.Second) {
			return
		}
	}

	if m.Cfg.Tss != nil {
		m.tssCheck()
		m.crossTxCheck()
	}
}

type EpochInfo struct {
	ElectedBlock  uint64
	StartBlock    uint64
	EndBlock      uint64
	MigratedBlock uint64
	Maintainers   []common.Address
}

type MaintainerInfo struct {
	Status            uint8          `json:"status,omitempty"`
	Account           common.Address `json:"account,omitempty"`
	LastHeartbeatTime *big.Int       `json:"last_heartbeat_time,omitempty"`
	LastActiveEpoch   *big.Int       `json:"last_active_epoch,omitempty"`
	Secp256Pubkey     []byte         `json:"secp_256_pubkey,omitempty"`
	Ed25519Pubkey     []byte         `json:"ed_25519_pubkey,omitempty"`
	P2pAddress        string         `json:"p_2_p_address,omitempty"`
}

func (m *Monitor) tssCheck() {
	maintainerAddr := m.Cfg.Tss.Maintainer
	mainAbi, err := mapoabi.New(abiJson.MaintainerABI)
	if err != nil {
		m.Log.Error("failed to parse maintainer abi", "err", err)
		return
	}

	// get epoch id
	var epoch *big.Int
	if err = m.callContract(&epoch, maintainerAddr, "currentEpoch", mainAbi); err != nil {
		m.Log.Error("failed to call contract", "method", "currentEpoch", "err", err)
		return
	}
	if epoch.Int64() == 0 {
		m.Log.Info("epoch is zero")
		return
	}

	// get epoch info
	epochInfo := struct{ Info EpochInfo }{}
	if err = m.callContract(&epochInfo, maintainerAddr, "getEpochInfo", mainAbi, epoch); err != nil {
		m.Log.Error("failed to call contract", "method", "getEpochInfo", "err", err)
		return
	}

	// get maintainer info
	type Back struct {
		Infos []MaintainerInfo `json:"infos"`
	}
	var ret Back
	if err = m.callContract(&ret, maintainerAddr, "getMaintainerInfos", mainAbi, epochInfo.Info.Maintainers); err != nil {
		m.Log.Error("failed to call contract", "method", "getMaintainerInfos", "err", err)
		return
	}
	m.checkNodeHealth(ret.Infos)
	m.checkScanner(ret.Infos)
	m.checkP2pStatus(ret.Infos)
}

func (m *Monitor) checkNodeHealth(infos []MaintainerInfo) {
	for _, info := range infos {
		url := fmt.Sprintf("http://%s:6040/ping", info.P2pAddress)

		resp, err := http.Get(url)
		if err != nil {
			m.Log.Error("failed to get node health", "address", info.P2pAddress, "err", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			util.Alarm(context.Background(), fmt.Sprintf("node(%s) is unhealthy ", info.Account.Hex()))
		}
	}
}

func (m *Monitor) checkP2pStatus(infos []MaintainerInfo) {
	for _, info := range infos {
		p2pStatus, err := m.GetP2PStatus(info.P2pAddress)
		if err != nil {
			util.Alarm(context.Background(),
				fmt.Sprintf("failed to get P2P status, address=%s ip=%s, err=%s", info.Account, info.P2pAddress, err))
			continue
		}
		if p2pStatus == nil {
			continue
		}
		if p2pStatus.Errors != nil {
			util.Alarm(context.Background(),
				fmt.Sprintf("P2P status error, address=%s ip=%s, errors=%s", info.Account, info.P2pAddress, p2pStatus.Errors))
			continue
		}
		if len(p2pStatus.Peers) == 0 {
			util.Alarm(context.Background(),
				fmt.Sprintf("P2P peerNode is empty, address=%s ip=%s", info.Account, info.P2pAddress))
			continue
		}
		m.Log.Info("P2P status", "address", info.P2pAddress, "status", p2pStatus)
	}
}

type P2PStatusPeer struct {
	Address        string `json:"address"`
	IP             string `json:"ip"`
	Status         string `json:"status"`
	StoredPeerID   string `json:"stored_peer_id"`
	NodesPeerID    string `json:"nodes_peer_id"`
	ReturnedPeerID string `json:"returned_peer_id"`
	P2PPortOpen    bool   `json:"p2p_port_open"`
	P2PDialMs      int    `json:"p2p_dial_ms"`
}

// P2PStatusResponse represents the response from /status/p2p endpoint
type P2PStatusResponse struct {
	Peers     []P2PStatusPeer `json:"peers"`
	PeerCount int             `json:"peer_count"`
	Errors    interface{}     `json:"errors"`
}

// GetP2PStatus fetches P2P status from the given IP address
func (m *Monitor) GetP2PStatus(ipAddress string) (*P2PStatusResponse, error) {
	url := fmt.Sprintf("http://%s:6040/status/p2p", ipAddress)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request P2P status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var statusResponse P2PStatusResponse
	if err := json.Unmarshal(body, &statusResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &statusResponse, nil
}

func (m *Monitor) checkScanner(infos []MaintainerInfo) {
	for _, info := range infos {
		scanner, err := m.GetScannerStatus(info.P2pAddress)
		if err != nil {
			util.Alarm(context.Background(),
				fmt.Sprintf("failed to node(%s) get scanner status for node :%v",
					info.Account.Hex(), err.Error()))
			continue
		}
		for k, v := range scanner {
			if v.ScannerHeightDiff < m.Cfg.Tss.ScannerGap {
				continue
			}
			util.Alarm(context.Background(),
				fmt.Sprintf("node(%s) scanner height difference too high for %s chain: latest:%d, current:%d diff:%d",
					info.Account.Hex(), k, v.ChainHeight, v.BlockScannerHeight, v.ScannerHeightDiff))
		}
	}
}

type ScannerStatus struct {
	Chain              string `json:"chain"`
	ChainHeight        int64  `json:"chain_height"`
	BlockScannerHeight int64  `json:"block_scanner_height"`
	ScannerHeightDiff  int64  `json:"scanner_height_diff"`
}

// ScannerStatusResponse represents the response from /status/scanner endpoint
type ScannerStatusResponse map[string]ScannerStatus

// GetScannerStatus fetches scanner status from the given IP address
func (m *Monitor) GetScannerStatus(ipAddress string) (ScannerStatusResponse, error) {
	url := fmt.Sprintf("http://%s:6040/status/scanner", ipAddress)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to request scanner status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var statusResponse ScannerStatusResponse
	if err := json.Unmarshal(body, &statusResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return statusResponse, nil
}

func (m *Monitor) callContract(ret interface{}, addr, method string, abiInst *mapoabi.Abi, params ...interface{}) error {
	input, err := abiInst.PackInput(method, params...)
	if err != nil {
		return errors.Wrapf(err, "pack input for %s", method)
	}
	to := common.HexToAddress(addr)
	output, err := mapprotocol.GlobalMapConn.CallContract(context.Background(),
		ethereum.CallMsg{To: &to, Data: input}, nil)
	if err != nil {
		return errors.Wrapf(err, "call contract %s", method)
	}
	return abiInst.UnpackOutput(method, ret, output)
}

func (m *Monitor) nativeCheck(contract string) {
	de := big.NewInt(10000000000)
	first := strings.Split(m.Cfg.Tk.BtcBridgeAddr, ",")[0]
	btcSrcAfter, err := getBtcBalanceByMem(first)
	if err != nil {
		m.Log.Error("Native check  ", "addr", first, "err ", err)
		return
	}
	m.Log.Info("Native check ", "total", btcSrcAfter)

	ret := mapprotocol.MinterCapResp{}
	err = mapprotocol.Call(contract, mapprotocol.MinterCapMethod, common.HexToAddress(m.Cfg.Tk.MapBridge), &ret)
	if err != nil {
		m.Log.Error("Native check, get amount by map contract", "err", err)
		return
	}
	contractAmount := ret.Total.Div(ret.Total, de)
	m.Log.Info("Check Native BTC balance, get amount", "bridgeBal", btcSrcAfter, "contractAmount", contractAmount)
	if btcSrcAfter < (contractAmount.Int64()) {
		util.Alarm(context.Background(), fmt.Sprintf("check brc20 balance token=btc, bridgeBal=%d, contractAmount=%v", btcSrcAfter, contractAmount))
	}
	chain.SleepWithStop(m.Stop, time.Second)
}

func (m *Monitor) OtherChainCheck() {
	if m.Cfg.LightNode == config.ZeroAddress {
		return
	}
	height, err := mapprotocol.Get2MapHeight(m.Cfg.Id)
	m.Log.Info("Check Height", "syncHeight", height, "record", m.syncedHeight, "heightCount", m.heightCount)
	if err != nil {
		m.Log.Error("get2MapHeight failed", "err", err)
	} else {
		if m.syncedHeight.Uint64() == height.Uint64() {
			m.heightCount = m.heightCount + 1
			if m.heightCount >= m.Cfg.CheckHgtCount {
				util.Alarm(context.Background(),
					fmt.Sprintf("Sync Height No change within %d minutes chains=%s, height=%d",
						m.Cfg.CheckHgtCount, m.Cfg.Name, height.Uint64()))
			}
		} else {
			m.heightCount = 0
		}
		m.syncedHeight = height
	}
}

func GetMulAddBalance(endpoint, key, bridge, token string) (int64, error) {
	var ret int64
	for _, b := range strings.Split(bridge, ",") {
		afterBridgeBal, err := TokenBalanceGD(endpoint, key, b, token)
		if err != nil {
			return 0, err
		}
		ret += afterBridgeBal
	}

	return ret, nil
}

func TokenBalanceGD(endpoint, key, address, token string) (int64, error) {
	path := fmt.Sprintf("/api/1/brc20/balance?address=%s&tick=%s&limit=1&offset=0", address, token)
	url := fmt.Sprintf("%s%s", endpoint, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("assamble req failed, err is %v", err)
	}
	req.Header.Set("api-key", key)

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do req failed, err is %v", err)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return 0, fmt.Errorf("io.ReadAll failed, err is %v", err)
	}
	_ = r.Body.Close()

	ret := &gdTokenBalanceResponse{}
	if err = json.Unmarshal(body, ret); err != nil {
		return 0, err
	}
	if ret.Code != 0 || ret.Message != "success" {
		return 0, fmt.Errorf("failed to get token balance, code: %v, msg: %s", ret.Code, ret.Message)
	}

	if len(ret.Data.List) == 0 || ret.Data.List[0].OverallBalance == "" {
		return 0, nil
	}
	balance, err := strconv.ParseFloat(ret.Data.List[0].OverallBalance, 64)
	if err != nil {
		return 0, fmt.Errorf("strconv.ParseFloat failed, err is %v", err)
	}
	return int64(balance), nil
}

type gdTokenBalanceResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		List []struct {
			Tick                string `json:"tick"`
			Address             string `json:"address"`
			OverallBalance      string `json:"overall_balance"`
			TransferableBalance string `json:"transferable_balance"`
			AvailableBalance    string `json:"available_balance"`
		} `json:"list"`
	} `json:"data"`
}

func getBtcBalanceByMem(bridgeAddr string) (int64, error) {
	netParams := &chaincfg.MainNetParams
	client := mempool.NewClient(netParams)
	address, _ := btcutil.DecodeAddress(bridgeAddr, netParams)
	b, err := client.GetBalance(address)
	if err != nil {
		return 0, err
	}
	log.Info("get res by mem", "balance", b)

	return b, nil
}
