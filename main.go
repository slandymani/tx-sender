package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/fbsobreira/gotron-sdk/pkg/keys/hd"
	"github.com/tyler-smith/go-bip39"
	"gitlab.com/distributed_lab/logan/v3/errors"
)

type Account struct {
	Sk      *ecdsa.PrivateKey
	Pk      *ecdsa.PublicKey
	Address common.Address
	Balance *big.Int
	Nonce   *uint64
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

	accounts := make(map[int]Account)

	fmt.Println("Start generating addresses and getting balances")
	now := time.Now()
	var wg sync.WaitGroup

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

		if sent%15 == 0 {
			gasPrice, _ = client.SuggestGasPrice(context.Background())
		}

		randGasPrice := new(big.Int).Set(gasPrice)
		randAdd, err := rand.Int(rand.Reader, big.NewInt(2000000000))
		if err == nil {
			randGasPrice.Add(gasPrice, randAdd)
		}

		gas := new(big.Int).Mul(randGasPrice, big.NewInt(21000))

		sender, receiver, amount, err := GetSenderReceiver(accounts, sendLimit, gas)
		if err != nil {
			//fmt.Println(errors.Wrap(err, "failed to receive sender and receiver for tx"))
			continue
		}

		fmt.Println(accounts[sender].Address.String(), " -> ", accounts[receiver].Address.String(), " - ", amount.String())

		var nonce uint64
		if accounts[sender].Nonce != nil {
			nonce = *accounts[sender].Nonce
		} else {
			nonce, err = client.PendingNonceAt(context.Background(), accounts[sender].Address)
			if err != nil {
				fmt.Println(err)
				continue
			}

			senderVal := accounts[sender]
			senderVal.Nonce = &nonce
			accounts[sender] = senderVal
		}

		receiverAddr := accounts[receiver].Address

		tx := types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: randGasPrice,
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
		go func(tx *types.Transaction, num int) {
			wg.Add(1)
			defer wg.Done()
			receipt, err := bind.WaitMined(context.Background(), client, tx)
			if err != nil {
				fmt.Printf("%d failed tx wait mined %s\n", num, tx.Hash())
				fmt.Println(err)
				return
			}
			fmt.Printf("%d tx wait mined %s block: %d\n", num, tx.Hash(), receipt.BlockNumber.Int64())
		}(tx, sent)

		accounts[receiver].Balance.Add(accounts[receiver].Balance, amount)
		accounts[sender].Balance.Sub(accounts[sender].Balance, amount)
		accounts[sender].Balance.Sub(accounts[sender].Balance, gas)
		atomic.AddUint64(accounts[sender].Nonce, 1)
		sent++

		randDelay := new(big.Int)
		randDelay, err = rand.Int(rand.Reader, big.NewInt(7500))
		if err != nil {
			randDelay = big.NewInt(3000)
		}
		randDelay.Add(randDelay, big.NewInt(2000))
		time.Sleep(time.Duration(randDelay.Int64() * int64(time.Millisecond)))
	}
	//sum := big.NewInt(0)
	//for _, account := range accounts {
	//	sum.Add(sum, account.Balance)
	//}
	//fmt.Println(sum.Int64())

	fmt.Println(now)
	fmt.Println(time.Since(now))

	fmt.Println("Finish sending txs")
	wg.Wait()

}
