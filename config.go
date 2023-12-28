package main

import (
	"math/big"

	"gitlab.com/distributed_lab/figure/v3"
	"gitlab.com/distributed_lab/kit/kv"
)

type EthConfig struct {
	Mnemonic        string   `fig:"mnemonic"`
	RequestsNumber  int64    `fig:"requests_number"`
	AddressesNumber int      `fig:"addresses_number"`
	RPC             string   `fig:"rpc,required"`
	MaxAmountToSend *big.Int `fig:"max_amount_to_send"`
}

func GetConfig() (EthConfig, error) {
	var result EthConfig

	err := figure.
		Out(&result).
		With(figure.BaseHooks, figure.EthereumHooks).
		From(kv.MustGetStringMap(kv.MustFromEnv(), "ethereum")).
		Please()

	return result, err
}
