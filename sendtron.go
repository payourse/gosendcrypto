package gosendcrypto

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"math/big"

	"github.com/craftto/go-tron/pkg/client"
	"github.com/craftto/go-tron/pkg/keystore"
	"github.com/craftto/go-tron/pkg/trc20"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func sendTron(ctx context.Context, cfg *CryptoSender, privKey, to string, amount float64, addrValues ...*SendToManyObj) (*Result, error) {
	client, err := client.NewGrpcClient(cfg.gateway, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	err = client.SetAPIKey(cfg.apiKey)
	if err != nil {
		return nil, err
	}

	ks, err := keystore.ImportFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	var txHash string

	if cfg.contractAddr != "" {
		contract, err := trc20.NewTrc20(client, cfg.contractAddr)
		if err != nil {
			return nil, err
		}

		decimals, err := contract.GetDecimals()
		if err != nil {
			return nil, err
		}

		multiplier, ok := new(big.Float).SetString((new(big.Int).Exp(big.NewInt(10), decimals, nil)).String())
		if !ok {
			return nil, errors.New("error calculating decimal multiplier")
		}
		value, ok := new(big.Int).SetString((new(big.Float).Mul(big.NewFloat(amount), multiplier)).Text('f', 0), 10)
		if !ok {
			return nil, errors.New("error calculating unit value")
		}

		tx, err := contract.Transfer(ks, to, value)
		if err != nil {
			return nil, err
		}
		txHash = tx.TransactionHash[2:]
	} else {
		amountFloat := new(big.Float).Mul(big.NewFloat(amount), big.NewFloat(math.Pow10(6)))
		amount, ok := new(big.Int).SetString(amountFloat.Text('f', 0), 10)
		if !ok {
			return nil, errors.New("error converting trx value to unit value")
		}
		txEx, err := client.Transfer(ks.Address.String(), to, amount.Int64())
		if err != nil {
			return nil, err
		}

		signedTx, err := ks.SignTx(txEx.Transaction)
		if err != nil {
			return nil, err
		}
		r, _ := client.Broadcast(signedTx)
		if err != nil {
			return nil, err
		}
		if r.Code.String() != "SUCCESS" {
			return nil, errors.New("trx transaction failed")
		}
		txHash = hex.EncodeToString(txEx.Txid)

	}

	res := &Result{
		TxHash: txHash,
	}
	return res, nil
}
