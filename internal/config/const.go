package config

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"time"
)

const (
	DefaultConfigPath   = "./config.json"
	DefaultKeystorePath = "./keys"
	MapChainID          = "mapChainId"
)

const (
	Near = "near"
)

const (
	BalanceRetryInterval = time.Second * 60
	RetryLongInterval    = time.Second * 10
)

var (
	Wei          = new(big.Int).SetUint64(1000000000)
	WeiOfNear, _ = new(big.Int).SetString("1000000000000000000000000", 10)
)

const (
	DefaultGasLimit      = 6721975
	DefaultGasPrice      = 20000000000
	DefaultGasMultiplier = 1
)

// Chain specific options
var (
	LightNode      = "lightnode"
	WaterLine      = "waterLine"
	ChangeInterval = "changeInterval"
)

const (
	MethodOfHeaderHeight = "headerHeight"
)

var (
	ZeroAddress = common.HexToAddress("0x0000000000000000000000000000000000000000")
)

var (
	HeightAbiJson = `[
		{
		  "inputs": [],
		  "name": "headerHeight",
		  "outputs": [
			{
			  "internalType": "uint256",
			  "name": "",
			  "type": "uint256"
			}
		  ],
		  "stateMutability": "view",
		  "type": "function"
		}
	]`
	LightMangerAbi = `[
		{
			"inputs": [
				{
					"internalType": "uint256",
					"name": "_chainId",
					"type": "uint256"
				}
			],
			"name": "headerHeight",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`
)
