package mapprotocol

import (
	"context"
	"github.com/ChainSafe/chainbridge-utils/msg"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/mapprotocol/monitor/pkg/ethclient"
	"github.com/pkg/errors"
	"math/big"
	"strings"
)

var (
	Height, _      = abi.JSON(strings.NewReader(config.HeightAbiJson))
	LightManger, _ = abi.JSON(strings.NewReader(config.LightMangerAbi))
	GlobalMapConn  *ethclient.Client
	Get2MapHeight  = func(chainId msg.ChainId) (*big.Int, error) { return nil, nil } // get other chain to map height
)

func InitOtherChain2MapHeight(lightManager common.Address) {
	Get2MapHeight = func(chainId msg.ChainId) (*big.Int, error) {
		input, err := PackInput(LightManger, config.MethodOfHeaderHeight, big.NewInt(int64(chainId)))
		if err != nil {
			return nil, errors.Wrap(err, "get other2map packInput failed")
		}

		height, err := HeaderHeight(lightManager, input)
		if err != nil {
			return nil, errors.Wrap(err, "get other2map headerHeight by lightManager failed")
		}
		return height, nil
	}
}

func PackInput(commonAbi abi.ABI, abiMethod string, params ...interface{}) ([]byte, error) {
	input, err := commonAbi.Pack(abiMethod, params...)
	if err != nil {
		return nil, err
	}
	return input, nil
}

func HeaderHeight(to common.Address, input []byte) (*big.Int, error) {
	output, err := GlobalMapConn.CallContract(context.Background(), ethereum.CallMsg{From: config.ZeroAddress, To: &to, Data: input}, nil)
	if err != nil {
		return nil, err
	}
	height, err := UnpackHeaderHeightOutput(output)
	if err != nil {
		return nil, err
	}
	return height, nil
}

func UnpackHeaderHeightOutput(output []byte) (*big.Int, error) {
	outputs := Height.Methods[config.MethodOfHeaderHeight].Outputs
	unpack, err := outputs.Unpack(output)
	if err != nil {
		return big.NewInt(0), err
	}

	height := new(big.Int)
	if err = outputs.Copy(&height, unpack); err != nil {
		return big.NewInt(0), err
	}
	return height, nil
}
