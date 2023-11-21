package gosendcrypto

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/payourse/gosendcrypto/erc20"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
)

func sendEthereum(ctx context.Context, cfg *CryptoSender, privKey, to string, value float64, addrValues ...*SendToManyObj) (*Result, error) {
	client, err := ethclient.Dial(cfg.gateway)
	if err != nil {
		return nil, err
	}

	pk, err := crypto.ToECDSA(common.FromHex(privKey))
	if err != nil {
		return nil, err
	}

	fromAddress := crypto.PubkeyToAddress(pk.PublicKey)
	toAddress := common.HexToAddress(to)
	gasLimit := uint64(21000)

	balance, err := client.BalanceAt(ctx, fromAddress, nil)
	if err != nil {
		return nil, err
	}

	// amount := big.NewInt(int64(value * params.Ether))
	amountFloat := new(big.Float).Mul(big.NewFloat(value), big.NewFloat(params.Ether))
	amount, ok := new(big.Int).SetString(amountFloat.Text('f', 0), 10)

	if !ok {
		return nil, errors.New("error converting value to ether")
	}

	networkID, err := client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}

	nonce := cfg.nonce
	if nonce == 0 {
		nonce, err = client.NonceAt(ctx, fromAddress, nil)
		if err != nil {
			return nil, err
		}
	}

	feeCap, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	tip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, err
	}

	var tx *types.Transaction

	if cfg.contractAddr != "" {
		tx, err = senderc20Token(
			client,
			pk,
			common.HexToAddress(cfg.contractAddr),
			fromAddress,
			toAddress,
			networkID,
			tip,
			feeCap,
			big.NewInt(int64(nonce)),
			value,
		)
		if err != nil {
			return nil, err
		}
	} else {
		tx = types.NewTx(&types.DynamicFeeTx{
			ChainID:   networkID,
			Nonce:     nonce,
			GasFeeCap: feeCap,
			GasTipCap: tip,
			Gas:       gasLimit,
			To:        &toAddress,
			Value:     amount,
			Data:      []byte{},
		})
		tx, err = types.SignTx(tx, types.LatestSignerForChainID(networkID), pk)
		if err != nil {
			return nil, err
		}
		if balance.Cmp(amount) != 1 {
			return nil, errors.New("amount should be less than balance")
		}

		err = client.SendTransaction(ctx, tx)
		if err != nil {
			return nil, err
		}
	}

	if cfg.awaitConfirmation {
		_, err := bind.WaitMined(ctx, client, tx)
		if err != nil {
			return nil, err
		}
	}

	dataStr := ""
	data, err := tx.MarshalBinary()
	if err != nil {
		fmt.Println("tx marshal error for", tx.Hash().Hex(), err.Error())
	} else {
		dataStr = hexutil.Encode(data)
	}

	result := &Result{
		TxHash: tx.Hash().Hex(),
		Nonce:  nonce,
		Data:   dataStr,
	}
	return result, nil

}

func senderc20Token(client *ethclient.Client, privKey *ecdsa.PrivateKey, contractAddr, fromAddr, toAddr common.Address, networkID, tip, feeCap, nonce *big.Int, value float64) (*types.Transaction, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(privKey, networkID)
	if err != nil {
		return nil, err
	}

	callOpts := &bind.CallOpts{
		Pending: false,
		From:    fromAddr,
	}

	contract, err := erc20.NewErc20(contractAddr, client)
	if err != nil {
		return nil, err
	}

	balance, err := contract.BalanceOf(callOpts, fromAddr)
	if err != nil {
		return nil, err
	}

	decimals, err := contract.Decimals(callOpts)
	if err != nil {
		return nil, err
	}

	// amount := big.NewInt(int64(value * math.Pow10(int(decimals))))
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	amountFloat := new(big.Float).Mul(big.NewFloat(value), new(big.Float).SetInt(multiplier))
	amount, ok := new(big.Int).SetString(amountFloat.Text('f', 0), 10)
	if !ok {
		return nil, errors.New("error converting value to unit")
	}

	if balance.Cmp(amount) == -1 {
		return nil, errors.New("value should be equal or greater than balance")
	}

	if tip.String() == "0" {
		tip = new(big.Int).Add(tip, big.NewInt(1_000_000_000))
	}

	feeCap2 := new(big.Int).Mul(feeCap, big.NewInt(2))
	auth.GasTipCap = tip
	auth.GasFeeCap = feeCap2
	auth.From = fromAddr
	auth.Nonce = nonce

	tx, err := contract.Transfer(auth, toAddr, amount)
	if err != nil {
		fmt.Println("erc20 transfer err:", err)
		return nil, err
	}

	fmt.Println("nonce", tx.Nonce(), "feecap", tx.GasFeeCap().String(), "tip", tx.GasTipCap().String())

	return tx, nil
}
