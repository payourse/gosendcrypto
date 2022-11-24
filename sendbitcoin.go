package gosendcrypto

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/imroc/req/v3"
)

var networks = map[string]*chaincfg.Params{
	"testnet": &chaincfg.TestNet3Params,
	"":        &chaincfg.MainNetParams,
}

func sendBitcoin(ctx context.Context, cfg *CryptoSender, privKey, toAddressStr string, amount float64, addrValues ...*SendToManyObj) (*Result, error) {
	chain := networks[string(cfg.network)]
	satValue := amount * math.Pow10(8)

	wif, err := btcutil.DecodeWIF(privKey)
	if err != nil {
		return nil, err
	}

	if len(addrValues) > 0 {
		return sendBitcoinToMany(ctx, cfg, privKey, toAddressStr, amount, addrValues...)
	}

	addrPubKey, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(wif.PrivKey.PubKey().SerializeCompressed()), chain)
	if err != nil {
		return nil, err
	}

	spendAddr := addrPubKey.EncodeAddress()
	if err != nil {
		return nil, err
	}

	spenderAddrByte, err := txscript.PayToAddrScript(addrPubKey)
	if err != nil {
		return nil, err
	}

	destAddr, err := btcutil.DecodeAddress(toAddressStr, chain)
	if err != nil {
		return nil, err
	}

	destAddrByte, err := txscript.PayToAddrScript(destAddr)
	if err != nil {
		return nil, err
	}

	if spendAddr == toAddressStr {
		log.Println("send to self")
	}

	body := struct {
		Jsonrpc string   `json:"jsonrpc"`
		Method  string   `json:"method"`
		ID      string   `json:"id"`
		Params  []string `json:"params"`
	}{
		Jsonrpc: "2.0",
		Method:  "getaddressunspent",
		ID:      "1101",
		Params:  []string{spendAddr},
	}

	var result struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  []struct {
			Height int    `json:"height"`
			TxHash string `json:"tx_hash"`
			TxPos  int    `json:"tx_pos"`
			Value  int    `json:"value"`
		} `json:"result"`
	}

	resp, err := req.R().
		SetHeader("Accept", "application/json").
		SetResult(&result).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	var txHash string
	var position int
	var balance int
	for _, utxoRes := range result.Result {
		if uint64(satValue) < uint64(utxoRes.Value) {
			txHash = utxoRes.TxHash
			position = utxoRes.TxPos
			balance = utxoRes.Value
		}
	}

	if txHash == "" {
		return nil, err
	}

	utxoHash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, err
	}

	body.Method = "getinfo"
	body.Params = []string{}

	var infoResult struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  struct {
			Path             string `json:"path"`
			Server           string `json:"server"`
			BlockchainHeight int    `json:"blockchain_height"`
			ServerHeight     int    `json:"server_height"`
			SpvNodes         int    `json:"spv_nodes"`
			Connected        bool   `json:"connected"`
			AutoConnect      bool   `json:"auto_connect"`
			Version          string `json:"version"`
			DefaultWallet    string `json:"default_wallet"`
			FeePerKb         int    `json:"fee_per_kb"`
		} `json:"result"`
	}

	resp, err = req.R().
		SetHeader("Accept", "application/json").
		SetResult(&infoResult).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	redeemTx := wire.NewMsgTx(2)
	outPoint := wire.NewOutPoint(utxoHash, uint32(position))
	txIn := wire.NewTxIn(outPoint, nil, [][]byte{})
	txIn.Sequence = txIn.Sequence - 2
	redeemTx.AddTxIn(txIn)

	redeemTxOut0 := wire.NewTxOut(int64(satValue), destAddrByte)
	redeemTxOut1 := wire.NewTxOut(int64(balance), spenderAddrByte)

	redeemTx.AddTxOut(redeemTxOut1) // add the change first (index=0)
	redeemTx.AddTxOut(redeemTxOut0)
	redeemTx.LockTime = uint32(infoResult.Result.BlockchainHeight)

	size := redeemTx.SerializeSize()
	redeemTxOut1.Value = int64(balance - 50 - int(satValue) - (size * infoResult.Result.FeePerKb / 1000))

	a := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		*outPoint: {},
	})
	sigHashes := txscript.NewTxSigHashes(redeemTx, a)

	signature, err := txscript.WitnessSignature(redeemTx, sigHashes, 0, int64(balance), spenderAddrByte, txscript.SigHashAll, wif.PrivKey, true)
	if err != nil {
		return nil, err
	}
	redeemTx.TxIn[0].Witness = signature

	var signedTx bytes.Buffer
	redeemTx.Serialize(&signedTx)

	hexSignedTx := hex.EncodeToString(signedTx.Bytes())

	body.Method = "broadcast"
	body.Params = []string{hexSignedTx}

	var broadcastResult struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  string `json:"result"`
	}

	resp, err = req.R().
		SetHeader("Accept", "application/json").
		SetResult(&broadcastResult).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	res := &Result{
		TxHash:     broadcastResult.Result,
		TxPosition: 0,
	}
	return res, nil
}

func sendBitcoinToMany(ctx context.Context, cfg *CryptoSender, privKey, toAddressStr string, amount float64, addrValues ...*SendToManyObj) (*Result, error) {
	chain := networks[string(cfg.network)]
	// satValue := amount * math.Pow10(8)

	wif, err := btcutil.DecodeWIF(privKey)
	if err != nil {
		return nil, err
	}

	addrPubKey, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(wif.PrivKey.PubKey().SerializeCompressed()), chain)
	if err != nil {
		return nil, err
	}

	spendAddr := addrPubKey.EncodeAddress()
	if err != nil {
		return nil, err
	}

	spenderAddrByte, err := txscript.PayToAddrScript(addrPubKey)
	if err != nil {
		return nil, err
	}

	destAddrBytes := [][]byte{}
	destAddrValues := []int{}
	totalSatValue := 0

	for _, addrValue := range addrValues {
		destAddr, err := btcutil.DecodeAddress(addrValue.Address, chain)
		if err != nil {
			return nil, err
		}

		destAddrByte, err := txscript.PayToAddrScript(destAddr)
		if err != nil {
			return nil, err
		}

		if spendAddr == toAddressStr {
			log.Println("send to self")
		}
		destAddrBytes = append(destAddrBytes, destAddrByte)
		satValue := int(addrValue.Amount * math.Pow10(8))
		totalSatValue = totalSatValue + satValue
		destAddrValues = append(destAddrValues, satValue)
	}

	body := struct {
		Jsonrpc string   `json:"jsonrpc"`
		Method  string   `json:"method"`
		ID      string   `json:"id"`
		Params  []string `json:"params"`
	}{
		Jsonrpc: "2.0",
		Method:  "getaddressunspent",
		ID:      "1101",
		Params:  []string{spendAddr},
	}

	var result struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  []struct {
			Height int    `json:"height"`
			TxHash string `json:"tx_hash"`
			TxPos  int    `json:"tx_pos"`
			Value  int    `json:"value"`
		} `json:"result"`
	}

	resp, err := req.R().
		SetHeader("Accept", "application/json").
		SetResult(&result).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	var txHash string
	var position int
	var balance int
	for _, utxoRes := range result.Result {
		if totalSatValue < utxoRes.Value {
			txHash = utxoRes.TxHash
			position = utxoRes.TxPos
			balance = utxoRes.Value
		}
	}

	if txHash == "" {
		return nil, errors.New("insufficient balance")
	}

	utxoHash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return nil, err
	}

	body.Method = "getinfo"
	body.Params = []string{}

	var infoResult struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  struct {
			Path             string `json:"path"`
			Server           string `json:"server"`
			BlockchainHeight int    `json:"blockchain_height"`
			ServerHeight     int    `json:"server_height"`
			SpvNodes         int    `json:"spv_nodes"`
			Connected        bool   `json:"connected"`
			AutoConnect      bool   `json:"auto_connect"`
			Version          string `json:"version"`
			DefaultWallet    string `json:"default_wallet"`
			FeePerKb         int    `json:"fee_per_kb"`
		} `json:"result"`
	}

	resp, err = req.R().
		SetHeader("Accept", "application/json").
		SetResult(&infoResult).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	redeemTx := wire.NewMsgTx(2)
	outPoint := wire.NewOutPoint(utxoHash, uint32(position))
	txIn := wire.NewTxIn(outPoint, nil, [][]byte{})
	txIn.Sequence = txIn.Sequence - 2
	redeemTx.AddTxIn(txIn)

	redeemTxOut1 := wire.NewTxOut(int64(balance), spenderAddrByte)
	redeemTx.AddTxOut(redeemTxOut1) // add the change first (index=0)

	for n, destAddrByte := range destAddrBytes {
		redeemTxOut := wire.NewTxOut(int64(destAddrValues[n]), destAddrByte)
		redeemTx.AddTxOut(redeemTxOut)
	}

	redeemTx.LockTime = uint32(infoResult.Result.BlockchainHeight)

	size := redeemTx.SerializeSize()
	redeemTxOut1.Value = int64(balance - 50 - totalSatValue - (size * infoResult.Result.FeePerKb / 1000))

	a := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		*outPoint: {},
	})
	sigHashes := txscript.NewTxSigHashes(redeemTx, a)

	signature, err := txscript.WitnessSignature(redeemTx, sigHashes, 0, int64(balance), spenderAddrByte, txscript.SigHashAll, wif.PrivKey, true)
	if err != nil {
		return nil, err
	}
	redeemTx.TxIn[0].Witness = signature

	var signedTx bytes.Buffer
	redeemTx.Serialize(&signedTx)

	hexSignedTx := hex.EncodeToString(signedTx.Bytes())

	body.Method = "broadcast"
	body.Params = []string{hexSignedTx}

	var broadcastResult struct {
		ID      string `json:"id"`
		Jsonrpc string `json:"jsonrpc"`
		Result  string `json:"result"`
	}

	resp, err = req.R().
		SetHeader("Accept", "application/json").
		SetResult(&broadcastResult).
		EnableDump().
		SetBody(&body).
		Post(cfg.gateway)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, errors.New("http req failed")
	}

	res := &Result{
		TxHash:     broadcastResult.Result,
		TxPosition: 0,
	}
	return res, nil
}
