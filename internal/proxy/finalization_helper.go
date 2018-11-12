package proxy

import (
	"time"
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/log"
	"github.com/kyokan/chaind/pkg"
	"net/http"
	"strings"
	"io/ioutil"
	"github.com/kyokan/chaind/pkg/rpc"
	"encoding/json"
	"sync/atomic"
)

const blockNumberRequest = "{\"jsonrpc\":\"2.0\",\"method\":\"eth_blockNumber\",\"params\":[],\"id\":0}"

const FinalityDepth = 7

type FinalizationHelper struct {
	blockHeight uint64
	sw          *BackendSwitch
	quitChan    chan bool
	logger      log15.Logger
	client      *http.Client
}

func NewFinalizationHelper(sw *BackendSwitch) *FinalizationHelper {
	return &FinalizationHelper{
		sw:       sw,
		quitChan: make(chan bool),
		logger:   log.NewLog("proxy/finalization_helper"),
		client: &http.Client{
			Timeout: time.Second,
		},
	}
}

func (b *FinalizationHelper) Start() error {
	b.updateBlockHeight()

	go func() {
		ticker := time.NewTicker(5 * time.Second)

		for {
			select {
			case <-ticker.C:
				b.updateBlockHeight()
			case <-b.quitChan:
				return
			}
		}
	}()

	return nil
}

func (b *FinalizationHelper) Stop() error {
	b.quitChan <- true
	return nil
}

func (b *FinalizationHelper) IsFinalized(blockNum uint64) bool {
	height := atomic.LoadUint64(&b.blockHeight)
	return height-blockNum >= FinalityDepth
}

func (b *FinalizationHelper) IsFinalizedHex(blockNum string) bool {
	num, err := rpc.Hex2Uint64(blockNum)
	if err != nil {
		return false
	}

	return b.IsFinalized(num)
}

func (b *FinalizationHelper) updateBlockHeight() {
	backend, err := b.sw.BackendFor(pkg.EthBackend)
	if err != nil {
		b.logger.Error("no backend available", "err", err)
	}

	res, err := b.client.Post(backend.URL, "application/json", strings.NewReader(blockNumberRequest))
	if err != nil {
		b.logger.Error("failed to fetch block height", "err", err)
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		b.logger.Error("failed to fetch response body", "err", err)
		return
	}
	var rpcRes rpc.JSONRPCRes
	err = json.Unmarshal(body, &rpcRes)
	if err != nil {
		b.logger.Error("failed to unmarshal response body", "err", err)
		return
	}
	var heightStr string
	err = json.Unmarshal(rpcRes.Result, &heightStr)
	if err != nil {
		b.logger.Error("failed to unmarshal RPC result", "err", err)
		return
	}
	heightBig, err := rpc.Hex2Big(heightStr)
	if err != nil {
		b.logger.Error("failed to create big num from response body", "err", err, "height", heightStr)
		return
	}

	b.logger.Debug("updated block height cache", "from", atomic.LoadUint64(&b.blockHeight), "to", heightBig.Uint64())
	atomic.StoreUint64(&b.blockHeight, heightBig.Uint64())
}
