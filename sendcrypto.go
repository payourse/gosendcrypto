package gosendcrypto

import (
	"context"
	"errors"
	"strings"
)

type blockchain string

var Blockchain = struct {
	Ethereum blockchain
	Tron     blockchain
	Bitcoin  blockchain
}{
	Ethereum: "ethereum",
	Tron:     "tron",
	Bitcoin:  "bitcoin",
}

type network string

var Network = struct {
	Testnet network
	Mainnet network
}{
	Testnet: "testnet",
	Mainnet: "",
}

var addrPrefixNetwork = map[string]string{
	"T":   string(Blockchain.Tron),
	"0x":  string(Blockchain.Ethereum),
	"1":   string(Blockchain.Bitcoin),
	"3":   string(Blockchain.Bitcoin),
	"bc1": string(Blockchain.Bitcoin),
	"2":   string(Blockchain.Bitcoin) + string(Network.Testnet),
	"m":   string(Blockchain.Bitcoin) + string(Network.Testnet),
	"n":   string(Blockchain.Bitcoin) + string(Network.Testnet),
	"tb1": string(Blockchain.Bitcoin) + string(Network.Testnet),
}

var senders = map[blockchain]func(ctx context.Context, cfg *CryptoSender, privKey, to string, amount float64, addrValues ...*SendToManyObj) (*Result, error){
	Blockchain.Ethereum: sendEthereum,
	Blockchain.Bitcoin:  sendBitcoin,
	Blockchain.Tron:     sendTron,
}

func NewCryptoSender(blockchain blockchain, network network, gatewayURL string) *CryptoSender {
	return &CryptoSender{
		blockchain: blockchain,
		network:    network,
		gateway:    gatewayURL,
	}
}

type Result struct {
	TxHash     string
	TxPosition int
	Nonce      uint64
	Balance    float64
}

type SendToManyResult struct {
	Success []*sendToManyResObj
	Failed  []*sendToManyResObj
}

type sendToManyResObj struct {
	Address    string
	Amount     float64
	TxHash     string
	TxPosition int
	Nonce      uint64
	Balance    float64
	err        error
}

type SendToManyObj struct {
	Address         string
	Amount          float64
	TerminateOnFail bool
}

type CryptoSender struct {
	blockchain        blockchain
	network           network
	gateway           string
	contractAddr      string
	apiKey            string
	hash              string
	txPosition        int
	balance           float64
	nonce             uint64
	awaitConfirmation bool
}

func (c *CryptoSender) SetAPIKey(apiKey string) *CryptoSender {
	c.apiKey = apiKey
	return c
}
func (c *CryptoSender) SetTxPosition(position int) *CryptoSender {
	c.txPosition = position
	return c
}
func (c *CryptoSender) SetLastHash(hash string) *CryptoSender {
	c.hash = hash
	return c
}
func (c *CryptoSender) SetBalance(balance float64) *CryptoSender {
	c.balance = balance
	return c
}
func (c *CryptoSender) SetNonce(nonce uint64) *CryptoSender {
	c.nonce = nonce
	return c
}
func (c *CryptoSender) SetAwaitConfirmation(wait bool) *CryptoSender {
	c.awaitConfirmation = wait
	return c
}
func (c *CryptoSender) SetContractAddress(contractAddr string) *CryptoSender {
	c.contractAddr = contractAddr
	return c
}

func (c *CryptoSender) Sendcrypto(ctx context.Context, privateKey, toAddress string, amount float64) (res *Result, err error) {
	net := ""
	for prefix := range addrPrefixNetwork {
		if strings.HasPrefix(toAddress, prefix) {
			net = addrPrefixNetwork[prefix]
		}
	}

	networkCheck := string(c.blockchain)
	if c.blockchain == Blockchain.Bitcoin {
		networkCheck += string(c.network)
	}

	if net != networkCheck {
		return nil, errors.New("invalid network or toAddress")
	}

	sender := senders[c.blockchain]
	res, err = sender(ctx, c, privateKey, toAddress, amount)
	return
}

func (c *CryptoSender) SendToMany(ctx context.Context, privateKey string, addrValues []*SendToManyObj) (res *SendToManyResult, err error) {
	if len(addrValues) < 1 {
		return nil, errors.New("invalid addrValues length")
	}
	toAddress := addrValues[0].Address
	net := ""
	prefix := ""
	for prefix := range addrPrefixNetwork {
		if strings.HasPrefix(toAddress, prefix) {
			net = addrPrefixNetwork[prefix]
		}
	}

	networkCheck := string(c.blockchain)
	if c.blockchain == Blockchain.Bitcoin {
		networkCheck += string(c.network)
	}

	if net != networkCheck {
		return nil, errors.New("invalid network or toAddress")
	}

	sender := senders[c.blockchain]
	res = &SendToManyResult{
		Success: []*sendToManyResObj{},
		Failed:  []*sendToManyResObj{},
	}
	if c.blockchain == Blockchain.Bitcoin {
		result, err := sender(ctx, c, privateKey, "", 0, addrValues...)
		if err != nil {
			return res, err
		}
		resList := []*sendToManyResObj{}
		for n, addrVal := range addrValues {
			if !strings.HasPrefix(addrVal.Address, prefix) {
				return nil, errors.New("invalid address found: " + addrVal.Address)
			}
			resList = append(resList, &sendToManyResObj{
				Address:    addrVal.Address,
				Amount:     addrVal.Amount,
				TxPosition: n + 1,
				TxHash:     result.TxHash,
			})
		}
		res.Success = resList
	} else {
		nonce := c.nonce
		for _, addrVal := range addrValues {
			c.nonce = nonce
			result, err := sender(ctx, c, privateKey, addrVal.Address, addrVal.Amount)
			if err != nil {
				res.Failed = append(res.Failed, &sendToManyResObj{
					Address: addrVal.Address,
					Amount:  addrVal.Amount,
					err:     err,
				})
				if addrVal.TerminateOnFail {
					return res, err
				}

			}
			res.Success = append(res.Success, &sendToManyResObj{
				Address: addrVal.Address,
				Amount:  addrVal.Amount,
				Nonce:   result.Nonce,
				TxHash:  result.TxHash,
			})
			nonce = result.Nonce + 1
		}
	}

	return
}
