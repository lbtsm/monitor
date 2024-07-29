package tron

import (
	"github.com/mapprotocol/monitor/pkg/ethclient"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"google.golang.org/grpc"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/lbtsm/gotron-sdk/pkg/client"
)

type Connection struct {
	endpoint                  string
	cli                       *client.GrpcClient
	log                       log15.Logger
	stop                      chan int
	reqTime, cacheBlockNumber int64
}

func NewConn(endpoint string, log log15.Logger) *Connection {
	return &Connection{
		endpoint: endpoint,
		log:      log,
		stop:     make(chan int),
	}
}

// Connect starts the ethereum WS connection
func (c *Connection) Connect() error {
	c.log.Info("Connecting to tron chain...", "url", c.endpoint)
	c.cli = client.NewGrpcClient(c.endpoint)
	err := c.cli.Start(grpc.WithInsecure())
	if err != nil {
		return err
	}
	return nil
}

func (c *Connection) Keypair() *keystore.Key {
	return nil
}

func (c *Connection) Client() *ethclient.Client {
	return nil
}

func (c *Connection) Opts() *bind.TransactOpts {
	return nil
}

func (c *Connection) CallOpts() *bind.CallOpts {
	return nil
}

func (c *Connection) UnlockOpts() {
}

func (c *Connection) LockAndUpdateOpts(needNewNonce bool) error {
	return nil
}

// LatestBlock returns the latest block from the current chain
func (c *Connection) LatestBlock() (*big.Int, error) {
	// 1s req
	if time.Now().Unix()-c.reqTime < 3 {
		return big.NewInt(0).SetInt64(c.cacheBlockNumber), nil
	}

	bnum, err := c.cli.GetNowBlock()
	if err != nil {
		return nil, err
	}
	c.cacheBlockNumber = bnum.GetBlockHeader().GetRawData().Number
	c.reqTime = time.Now().Unix()

	return big.NewInt(0).SetInt64(bnum.GetBlockHeader().GetRawData().Number), nil
}

// EnsureHasBytecode asserts if contract code exists at the specified address
func (c *Connection) EnsureHasBytecode(addr ethcommon.Address) error {
	return nil
}

func (c *Connection) WaitForBlock(targetBlock *big.Int, delay *big.Int) error {
	return nil
}

func (c *Connection) Close() {
	if c.cli != nil {
		_ = c.cli.Conn.Close()
	}
	close(c.stop)
}
