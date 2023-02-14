package config

import (
	"encoding/json"
	"fmt"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
)

// RawChainConfig is parsed directly from the config file and should be using to construct the core.ChainConfig
type RawChainConfig struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Id           string            `json:"id"`       // ChainID
	Endpoint     string            `json:"endpoint"` // url for rpc endpoint
	From         string            `json:"from"`     // address of key to use
	Network      string            `json:"network"`
	KeystorePath string            `json:"keystorePath"`
	Opts         map[string]string `json:"opts"`
}

type Config struct {
	MapChain     RawChainConfig   `json:"mapchain"`
	Chains       []RawChainConfig `json:"chains"`
	KeystorePath string           `json:"keystorePath,omitempty"`
}

func (c *Config) validate() error {
	for _, chain := range c.Chains {
		if chain.Id == "" {
			return fmt.Errorf("required field chains.Id empty for chains %s", chain.Id)
		}
		if chain.Type == "" {
			return fmt.Errorf("required field chains.Type empty for chains %s", chain.Id)
		}
		if chain.Endpoint == "" {
			return fmt.Errorf("required field chains.Endpoint empty for chains %s", chain.Id)
		}
		if chain.Name == "" {
			return fmt.Errorf("required field chains.Name empty for chains %s", chain.Id)
		}
		if chain.From == "" {
			return fmt.Errorf("required field chains.From empty for chains %s", chain.Id)
		}
	}
	// check map chains
	if c.MapChain.Id == "" {
		return fmt.Errorf("required field chains.Id empty for chains %s", c.MapChain.Id)
	}
	if c.MapChain.Endpoint == "" {
		return fmt.Errorf("required field mapchain.Endpoint empty for chains %s", c.MapChain.Id)
	}
	if c.MapChain.From == "" {
		return fmt.Errorf("required field chains.From empty for chains %s", c.MapChain.Id)
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
	err = fig.validate()
	// fill map chains config
	fig.MapChain.Type = "ethereum"
	fig.MapChain.Name = "map"

	if err != nil {
		return nil, err
	}
	return &fig, nil
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
	Name           string      // Human-readable chain name
	Id             msg.ChainId // ChainID
	Endpoint       string      // url for rpc endpoint
	From           string      // address of key to use
	KeystorePath   string      // Location of keyfiles
	GasLimit       *big.Int
	MaxGasPrice    *big.Int
	GasMultiplier  *big.Float
	WaterLine      string
	ChangeInterval string
	StartBlock     *big.Int
	MapChainID     msg.ChainId
	LightNode      common.Address // the lightnode to sync header
	EgsApiKey      string         // API key for ethgasstation to query gas prices
	EgsSpeed       string         // The speed which a transaction should be processed: average, fast, fastest. Default: fast
}

// ParseOptConfig uses a core.ChainConfig to construct a corresponding Config
func ParseOptConfig(chainCfg *ChainConfig) (*OptConfig, error) {
	config := &OptConfig{
		Id:             chainCfg.Id,
		From:           chainCfg.From,
		Name:           chainCfg.Name,
		Endpoint:       chainCfg.Endpoint,
		KeystorePath:   DefaultKeystorePath,
		WaterLine:      "",
		ChangeInterval: "",
		EgsApiKey:      "",
		EgsSpeed:       "",
		StartBlock:     big.NewInt(0),
		GasLimit:       big.NewInt(DefaultGasLimit),
		MaxGasPrice:    big.NewInt(DefaultGasPrice),
		GasMultiplier:  big.NewFloat(DefaultGasMultiplier),
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
		config.MapChainID = msg.ChainId(chainId)
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

	return config, nil
}
