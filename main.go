package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fbsobreira/gotron-sdk/pkg/keys/hd"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"math/big"
	"time"
)

type Account struct {
	Sk      *ecdsa.PrivateKey
	Pk      *ecdsa.PublicKey
	Address common.Address
	Balance *big.Int
}

func FromMnemonicSeed(mnemonic string, index int) (*btcec.PrivateKey, *btcec.PublicKey) {
	seed := bip39.NewSeed(mnemonic, "")
	master, ch := hd.ComputeMastersFromSeed(seed, []byte("Bitcoin seed"))
	private, _ := hd.DerivePrivateKeyForPath(
		btcec.S256(),
		master,
		ch,
		fmt.Sprintf("44'/60'/0'/0/%d", index),
	)

	return btcec.PrivKeyFromBytes(private[:])
}

func GetSenderReceiver(accounts map[int]Account, limit, gas *big.Int) (int, int, *big.Int, error) {
	count := len(accounts)
	source := 0
	for i := 0; i < 100; i++ {
		temp, _ := rand.Int(rand.Reader, big.NewInt(int64(count)))
		if len(accounts[int(temp.Int64())].Balance.Bits()) != 0 {
			source = int(temp.Int64())
			break
		}
	}

	destT, _ := rand.Int(rand.Reader, big.NewInt(int64(count)))
	dest := int(destT.Int64())
	if dest == source {
		dest = (dest + 1) % count
	}

	sendBalance := accounts[source].Balance
	if sendBalance.Cmp(limit) == 1 {
		sendBalance = limit
	}

	sub := new(big.Int).Sub(accounts[source].Balance, sendBalance)

	if sub.Cmp(gas) == -1 {
		sendBalance = new(big.Int).Sub(accounts[source].Balance, gas)
	}
	//fmt.Println(gas.Int64())

	if sendBalance.Cmp(big.NewInt(0)) != 1 {
		return 0, 0, nil, errors.New("insufficient balance")
	}

	balance, _ := rand.Int(rand.Reader, sendBalance)
	balance.Add(balance, big.NewInt(1))

	return source, dest, balance, nil
}

func main() {
	config, err := GetConfig()
	if err != nil {
		panic(errors.Wrap(err, "wrong config"))
	}

	client, err := ethclient.Dial(config.RPC)
	if err != nil {
		panic(errors.Wrap(err, "failed to create connection"))
	}
	defer client.Close()

	//masterSK, masterPK := FromMnemonicSeed(config.Mnemonic, 0)
	//fmt.Println(masterSK.ToECDSA())
	//fmt.Println(masterPK.ToECDSA())
	//
	//fmt.Println(crypto.PubkeyToAddress(*masterPK.ToECDSA()).String())

	accounts := make(map[int]Account)

	fmt.Println("Start generating addresses and getting balances")
	now := time.Now()

	for i := 0; i < config.AddressesNumber; i++ {
		sk, pk := FromMnemonicSeed(config.Mnemonic, i)
		address := crypto.PubkeyToAddress(*pk.ToECDSA())

		var err error
		balance := big.NewInt(0)

		for j := 0; j < 5; j++ {
			balance, err = client.BalanceAt(context.Background(), address, nil)
			if err == nil {
				break
			}
			fmt.Println(err)
			time.Sleep(time.Second)
		}

		accounts[i] = Account{
			Sk:      sk.ToECDSA(),
			Pk:      pk.ToECDSA(),
			Address: address,
			Balance: balance,
		}
		//fmt.Println(address.String())
	}

	//accounts[0].Balance.Add(accounts[0].Balance, big.NewInt(10000))

	fmt.Println(time.Since(now))

	fmt.Println("Finish generating addresses and getting balances")

	sent := 0
	sentPrev := 0

	gasPrice, _ := client.SuggestGasPrice(context.Background())

	sendLimit := config.MaxAmountToSend
	chainID, _ := client.ChainID(context.Background())

	fmt.Println("Start sending txs")
	fmt.Println("Start time ", time.Now())
	now = time.Now()

	for int64(sent) < config.RequestsNumber {
		if sentPrev != sent {
			fmt.Printf("%d transactions sent\n", sent)
			sentPrev = sent
		}

		if sent%50 == 0 {
			gasPrice, _ = client.SuggestGasPrice(context.Background())
			gasPrice.Mul(gasPrice, big.NewInt(6))
			gasPrice.Quo(gasPrice, big.NewInt(5))
		}

		gas := new(big.Int).Mul(gasPrice, big.NewInt(21000))
		gas.Mul(gas, big.NewInt(6))
		gas.Quo(gas, big.NewInt(5))
		randAdd, err := rand.Int(rand.Reader, big.NewInt(2000))
		if err == nil {
			gas.Add(gas, randAdd)
		}

		sender, receiver, amount, err := GetSenderReceiver(accounts, sendLimit, gas)
		if err != nil {
			//fmt.Println(err)
			continue
		}

		fmt.Println(accounts[sender].Address.String(), " -> ", accounts[receiver].Address.String(), " - ", amount.String())

		nonce, err := client.PendingNonceAt(context.Background(), accounts[sender].Address)
		if err != nil {
			fmt.Println(err)
			continue
		}

		receiverAddr := accounts[receiver].Address

		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      uint64(21000),
			To:       &receiverAddr,
			Value:    amount,
			Data:     make([]byte, 0),
		})

		tx, err = types.SignTx(tx, types.NewLondonSigner(chainID), accounts[sender].Sk)
		if err != nil {
			fmt.Println(errors.Wrap(err, "failed to sign tx"))
			continue
		}

		err = client.SendTransaction(context.Background(), tx)
		if err != nil {
			fmt.Println(errors.Wrap(err, "failed to send tx"))
			continue
		}
		fmt.Println(tx.Hash().String())
		receipt, err := bind.WaitMined(context.Background(), client, tx)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println(receipt.BlockNumber.Int64())

		accounts[receiver].Balance.Add(accounts[receiver].Balance, amount)
		accounts[sender].Balance.Sub(accounts[sender].Balance, amount)
		accounts[sender].Balance.Sub(accounts[sender].Balance, gas)
		sent++
	}
	sum := big.NewInt(0)
	for _, account := range accounts {
		sum.Add(sum, account.Balance)
	}
	fmt.Println(sum.Int64())

	fmt.Println(time.Since(now))

	fmt.Println("Finish sending txs")
}
