package config

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

// RawChainConfig is parsed directly from the config file and should be using to construct the core.ChainConfig
type RawChainConfig struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Id            string            `json:"id"`       // ChainID
	Endpoint      string            `json:"endpoint"` // url for rpc endpoint
	From          string            `json:"from"`     // address of key to use
	Network       string            `json:"network"`
	KeystorePath  string            `json:"keystorePath"`
	Opts          map[string]string `json:"opts"`
	Users         []From            `json:"users"`
	ContractToken []ContractToken   `json:"contractToken"`
	Energies      []Energy          `json:"energy"`
	Tss           *Tss              `json:"tss"`
}

type Defaults struct {
	Users         []From            `json:"users"`
	Opts          map[string]string `json:"opts"`
	From          string            `json:"from"`
	ContractToken []ContractToken   `json:"contractToken"`
}

type Config struct {
	Defaults     Defaults         `json:"defaults"`
	Chains       []RawChainConfig `json:"chains"`
	KeystorePath string           `json:"keystorePath,omitempty"`
	Tk           Token            `json:"token"`
	Genni        Api              `json:"genni"`
}

// MapChainConfig returns the map chain config from the chains list.
func (c *Config) MapChainConfig() *RawChainConfig {
	for i := range c.Chains {
		if strings.ToLower(c.Chains[i].Name) == "map" {
			return &c.Chains[i]
		}
	}
	return nil
}

type Token struct {
	BridgeAddr    string   `json:"bridge_addr"`
	BtcBridgeAddr string   `json:"btc_bridge_addr"`
	MapBridge     string   `json:"map_bridge"`
	Token         []string `json:"token"`
	Contracts     []string `json:"contracts"`
}

type ContractToken struct {
	Address string     `json:"address"`
	Tokens  []EthToken `json:"tokens"`
}

type Energy struct {
	Address   string `json:"address"`
	Waterline int64  `json:"waterline"`
	// Expiry monitoring (Stake 2.0 delegation lock). Disabled when
	// ProtectedThreshold <= 0; remaining fields are defaulted by
	// ApplyExpiryDefaults.
	ProtectedThreshold   int64 `json:"protectedThreshold"`
	RecoveryThreshold    int64 `json:"recoveryThreshold"`
	LookaheadHours       int64 `json:"lookaheadHours"`
	CheckIntervalMinutes int64 `json:"checkIntervalMinutes"`
	RepeatIntervalHours  int64 `json:"repeatIntervalHours"`
}

// ApplyExpiryDefaults fills unset expiry-monitoring fields. No-op when the
// feature is disabled (ProtectedThreshold <= 0).
func (e *Energy) ApplyExpiryDefaults() {
	if e.ProtectedThreshold <= 0 {
		return
	}
	if e.RecoveryThreshold <= 0 {
		// protected × 1.05, rounded up
		e.RecoveryThreshold = e.ProtectedThreshold + (e.ProtectedThreshold+19)/20
	}
	if e.LookaheadHours <= 0 {
		e.LookaheadHours = 72
	}
	if e.CheckIntervalMinutes <= 0 {
		e.CheckIntervalMinutes = 60
	}
	if e.RepeatIntervalHours <= 0 {
		e.RepeatIntervalHours = 12
	}
}

type EthToken struct {
	Name      string  `json:"name"`
	Addr      string  `json:"addr"`
	WaterLine float64 `json:"waterLine"`
	Wei       int64   `json:"wei"`
}

type Api struct {
	Key      string `json:"key"`
	Endpoint string `json:"endpoint"`
}

type From struct {
	Group     string `json:"group"`
	From      string `json:"from"`
	WaterLine string `json:"waterLine"`
}

type Tss struct {
	Maintainer     string `json:"maintainer"`
	ScannerGap     int64  `json:"scannerGap"`
	BtcAddress     string `json:"btcAddress,omitempty"`
	BlockstreamUrl string `json:"blockstreamUrl,omitempty"`
	TssApiUrl      string `json:"tssApiUrl,omitempty"`
	CrossTxLimit   int    `json:"crossTxLimit,omitempty"`
}

func (c *Config) validate() error {
	for _, chain := range c.Chains {
		if chain.Endpoint == "" {
			return fmt.Errorf("required field chains.Endpoint empty for chain %s", chain.Name)
		}
		if chain.Name == "" {
			return fmt.Errorf("required field chains.Name empty for chain with id %s", chain.Id)
		}
	}
	if mc := c.MapChainConfig(); mc == nil {
		return fmt.Errorf("map chain not found in chains list, please add a chain with name \"map\"")
	}
	return nil
}

func GetConfig(ctx *cli.Context) (*Config, error) {
	var fig Config
	path := DefaultConfigPath
	if file := ctx.String(FileFlag.Name); file != "" {
		path = file
	}
	err := loadConfig(path, &fig)
	if err != nil {
		log.Warn("err loading json file", "err", err.Error())
		return &fig, err
	}
	if ksPath := ctx.String(KeystorePathFlag.Name); ksPath != "" {
		fig.KeystorePath = ksPath
	}
	log.Debug("Loaded config", "path", path)

	// apply defaults to chains
	fig.applyDefaults()

	err = fig.validate()
	if err != nil {
		return nil, err
	}
	return &fig, nil
}

// applyDefaults fills in missing chain fields from the top-level defaults and
// resolves chain IDs from well-known chain names when not explicitly set.
func (c *Config) applyDefaults() {
	applyChainDefaults := func(chain *RawChainConfig) {
		// default type is ethereum
		if chain.Type == "" {
			chain.Type = "ethereum"
		}
		// inherit from address
		if chain.From == "" && c.Defaults.From != "" {
			chain.From = c.Defaults.From
		}
		// inherit users: merge by group name so chains can omit repeated addresses
		if len(c.Defaults.Users) > 0 {
			if len(chain.Users) == 0 {
				chain.Users = make([]From, len(c.Defaults.Users))
				copy(chain.Users, c.Defaults.Users)
			} else {
				defaultUserMap := make(map[string]From)
				for _, u := range c.Defaults.Users {
					defaultUserMap[u.Group] = u
				}
				for i, u := range chain.Users {
					if du, ok := defaultUserMap[u.Group]; ok {
						if u.From == "" {
							chain.Users[i].From = du.From
						}
						if u.WaterLine == "" {
							chain.Users[i].WaterLine = du.WaterLine
						}
					}
				}
			}
		}
		// inherit contractToken
		if len(chain.ContractToken) == 0 && len(c.Defaults.ContractToken) > 0 {
			chain.ContractToken = c.Defaults.ContractToken
		}
		// inherit opts (merge: default opts first, chain opts override)
		if len(c.Defaults.Opts) > 0 {
			if chain.Opts == nil {
				chain.Opts = make(map[string]string)
			}
			for k, v := range c.Defaults.Opts {
				if _, exists := chain.Opts[k]; !exists {
					chain.Opts[k] = v
				}
			}
		}
		// ensure opts is not nil
		if chain.Opts == nil {
			chain.Opts = make(map[string]string)
		}
		// resolve chain ID from well-known chain name if not set
		if chain.Id == "" {
			if id, ok := wellKnownChainIDs[strings.ToLower(chain.Name)]; ok {
				chain.Id = id
			}
		}
	}

	for i := range c.Chains {
		applyChainDefaults(&c.Chains[i])
	}
}

// wellKnownChainIDs maps chain names to their mainnet chain IDs.
var wellKnownChainIDs = map[string]string{
	"bsc":    "56",
	"eth":    "1",
	"base":   "8453",
	"arb":    "42161",
	"opt":    "10",
	"uni":    "130",
	"pol":    "137",
	"avax":   "43114",
	"xlayer": "196",
	"map":    "22776",
	"btc":    "1360095883558913",
	"doge":   "1360121653362689",
	"xrp":    "1360117358395393",
	"tron":   "728126428",
	"sol":    "1360108768460801",
	"klaytn": "8217",
	"zksync": "324",
	"merlin": "4200",
}

func loadConfig(file string, config *Config) error {
	ext := filepath.Ext(file)
	fp, err := filepath.Abs(file)
	if err != nil {
		return err
	}

	log.Debug("Loading configuration", "path", filepath.Clean(fp))

	f, err := os.Open(filepath.Clean(fp))
	if err != nil {
		return err
	}

	if ext == ".json" {
		if err = json.NewDecoder(f).Decode(&config); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unrecognized extention: %s", ext)
	}

	return nil
}

type OptConfig struct {
	Name           string   // Human-readable chain name
	Id             ChainId  // ChainID
	Endpoint       string   // url for rpc endpoint
	From           []string // address of key to use
	KeystorePath   string   // Location of keyfiles
	GasLimit       *big.Int
	MaxGasPrice    *big.Int
	GasMultiplier  *big.Float
	WaterLine      string
	ChangeInterval string
	ApiUrl         string
	StartBlock     *big.Int
	MapChainID     ChainId
	LightNode      common.Address // the lightnode to sync header
	Tk             *Token
	Genni          *Api
	CheckHgtCount  int64
	Users          []From
	ContractToken  []ContractToken
	Energies       []Energy
	Tss            *Tss
}

// ParseOptConfig uses a core.ChainConfig to construct a corresponding Config
func ParseOptConfig(chainCfg *ChainConfig, tks *Token, genni *Api, users []From) (*OptConfig, error) {
	config := &OptConfig{
		Id:             chainCfg.Id,
		From:           strings.Split(chainCfg.From, ","),
		Name:           chainCfg.Name,
		Endpoint:       chainCfg.Endpoint,
		KeystorePath:   DefaultKeystorePath,
		WaterLine:      "",
		ChangeInterval: "",
		StartBlock:     big.NewInt(0),
		GasLimit:       big.NewInt(DefaultGasLimit),
		MaxGasPrice:    big.NewInt(DefaultGasPrice),
		GasMultiplier:  big.NewFloat(DefaultGasMultiplier),
		Tk:             tks,
		Genni:          genni,
		CheckHgtCount:  DefaultCheckHgtCount,
		ContractToken:  chainCfg.ContractToken,
		Energies:       chainCfg.Energies,
		Users:          users,
		Tss:            chainCfg.Tss,
	}

	if chainCfg.NearKeystorePath != "" {
		config.KeystorePath = chainCfg.NearKeystorePath
	}

	if mapChainID, ok := chainCfg.Opts[MapChainID]; ok {
		// key exist anyway
		chainId, errr := strconv.Atoi(mapChainID)
		if errr != nil {
			return nil, errr
		}
		config.MapChainID = ChainId(chainId)
	}

	if waterLine, ok := chainCfg.Opts[WaterLine]; ok && waterLine != "" {
		config.WaterLine = waterLine
	}

	if lightnode, ok := chainCfg.Opts[LightNode]; ok && lightnode != "" {
		config.LightNode = common.HexToAddress(lightnode)
	}

	if alarmSecond, ok := chainCfg.Opts[ChangeInterval]; ok && alarmSecond != "" {
		config.ChangeInterval = alarmSecond
	}

	if apiUrl, ok := chainCfg.Opts[ApiUrl]; ok && apiUrl != "" {
		config.ApiUrl = apiUrl
	}

	if checkHeightCount, ok := chainCfg.Opts[CheckHeightCount]; ok && checkHeightCount != "" {
		count, err := strconv.Atoi(checkHeightCount)
		if err != nil {
			return nil, err
		}
		config.CheckHgtCount = int64(count)
	}

	return config, nil
}
