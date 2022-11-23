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

var senders = map[blockchain]func(ctx context.Context, cfg *CryptoSender, privKey, to string, amount float64) (*Result, error){
	Blockchain.Ethereum: sendEthereum,
	Blockchain.Bitcoin:  sendBitcoin,
	Blockchain.Tron:     sendTron,
}

func NewCryptoSender(blockchain blockchain, network network, contractAddr, gatewayURL string) *CryptoSender {
	return &CryptoSender{
		blockchain:   blockchain,
		network:      network,
		gateway:      gatewayURL,
		contractAddr: contractAddr,
	}
}

type Result struct {
	TxHash     string
	TxPosition int
	Nonce      uint64
	Balance    float64
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

	// chain := c.contractAddr

	// if c.blockchain == Blockchain.Bitcoin {
	// 	chain = string(c.network)
	// }

	sender := senders[c.blockchain]
	res, err = sender(ctx, c, privateKey, toAddress, amount)
	return
}
