package config

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var FlagsOfImportCmd = []cli.Flag{
	EthereumImportFlag,
	PrivateKeyFlag,
	Sr25519Flag,
	Secp256k1Flag,
	Ed25519Flag,
	PasswordFlag,
	SubkeyNetworkFlag,
}

var (
	FileFlag = &cli.StringFlag{
		Name:  "config",
		Usage: "JSON configuration file",
	}
	VerbosityFlag = &cli.StringFlag{
		Name:  "verbosity",
		Usage: "Supports levels crit (silent) to trce (trace)",
		Value: log.LvlInfo.String(),
	}
	KeystorePathFlag = &cli.StringFlag{
		Name:  "keystore",
		Usage: "Path to keystore directory",
		Value: DefaultKeystorePath,
	}
)

var (
	PasswordFlag = &cli.StringFlag{
		Name:  "password",
		Usage: "Password used to encrypt the keystore. Used with --generate, --import, or --unlock",
	}
	Sr25519Flag = &cli.BoolFlag{
		Name:  "sr25519",
		Usage: "Specify account/key type as sr25519.",
	}
	Secp256k1Flag = &cli.BoolFlag{
		Name:  "secp256k1",
		Usage: "Specify account/key type as secp256k1.",
	}
	Ed25519Flag = &cli.BoolFlag{
		Name:  "ed25519",
		Usage: "Specify account/key type as near.",
	}
	EthereumImportFlag = &cli.BoolFlag{
		Name:  "ethereum",
		Usage: "Import an existing ethereum keystore, such as from geth.",
	}
	SubkeyNetworkFlag = &cli.StringFlag{
		Name:        "network",
		Usage:       "Specify the network to use for the address encoding (substrate/polkadot/centrifuge)",
		DefaultText: "substrate",
	}
	PrivateKeyFlag = &cli.StringFlag{
		Name:  "privateKey",
		Usage: "Import a hex representation of a private key into a keystore.",
	}
)
