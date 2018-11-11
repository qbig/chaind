package proxy

import (
	"github.com/kyokan/chaind/pkg"
	"time"
	"github.com/inconshreveable/log15"
	"github.com/kyokan/chaind/pkg/log"
	"fmt"
	"net/http"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
		"encoding/json"
)

const ethCheckBody = "{\"jsonrpc\":\"2.0\",\"method\":\"eth_syncing\",\"params\":[],\"id\":%d}"

type BackendSwitch struct {
	ethBackends []pkg.Backend
	btcBackends []pkg.Backend
	currEth     int32
	currBtc     int32
	quitChan    chan bool
	logger      log15.Logger
}

func NewBackendSwitch(ethBackends []pkg.Backend, btcBackends []pkg.Backend) (*BackendSwitch, error) {
	if len(ethBackends) == 0 && len(btcBackends) == 0 {
		return nil, errors.New("no backends configured")
	}

	return &BackendSwitch{
		ethBackends: ethBackends,
		btcBackends: btcBackends,
		quitChan:    make(chan bool),
		logger:      log.NewLog("proxy/backend_switch"),
	}, nil
}

func (h *BackendSwitch) Start() error {
	if len(h.ethBackends) > 0 {
		var selected int32
		for i, backend := range h.ethBackends {
			if backend.IsMain {
				selected = int32(i)
				break
			}
		}
		h.currEth = selected
	} else {
		h.currEth = -1
	}

	if len(h.btcBackends) > 0 {
		var selected int32
		for i, backend := range h.btcBackends {
			if backend.IsMain {
				selected = int32(i)
				break
			}
		}
		h.currBtc = selected
	} else {
		h.currBtc = -1
	}

	go func() {
		tick := time.NewTicker(5 * time.Second)

		for {
			select {
			case <-tick.C:
				var wg sync.WaitGroup
				if h.currEth != -1 {
					wg.Add(1)
					go func() {
						idx := h.doHealthcheck(atomic.LoadInt32(&h.currEth), h.ethBackends)
						atomic.StoreInt32(&h.currEth, idx)
						wg.Done()
					}()
				}
				if h.currBtc != -1 {
					wg.Add(1)
					go func() {
						idx := h.doHealthcheck(atomic.LoadInt32(&h.currBtc), h.btcBackends)
						atomic.StoreInt32(&h.currBtc, idx)
						wg.Done()
					}()
				}
				wg.Wait()
			case <-h.quitChan:
				return
			}
		}
	}()

	return nil
}

func (h *BackendSwitch) Stop() error {
	h.quitChan <- true
	return nil
}

func (h *BackendSwitch) BackendFor(t pkg.BackendType) (*pkg.Backend, error) {
	var idx int32

	if t == pkg.EthBackend {
		idx = atomic.LoadInt32(&h.currEth)
	} else {
		idx = atomic.LoadInt32(&h.currBtc)
	}

	if idx == -1 {
		return nil, errors.New("no backends available")
	}

	return &h.ethBackends[idx], nil
}

func (h *BackendSwitch) doHealthcheck(idx int32, list []pkg.Backend) int32 {
	if idx == -1 {
		return -1
	}

	backend := list[idx]
	logger.Debug("performing healthcheck", "type", backend.Type, "name", backend.Name, "url", backend.URL)
	checker := NewChecker(&backend)
	ok := checker.Check()

	if !ok {
		logger.Warn("backend is unhealthy, trying another", "type", backend.Type, "name", backend.Name, "url", backend.URL)
		return h.doHealthcheck(h.nextBackend(idx, list))
	}

	logger.Debug("backend is ok", "type", backend.Type, "name", backend.Name, "url", backend.URL)
	return idx
}

func (h *BackendSwitch) nextBackend(idx int32, list []pkg.Backend) (int32, []pkg.Backend) {
	backend := list[idx]
	if len(list) == 1 {
		h.logger.Error("no more backends to try", "type", backend.Type)
		return -1, list
	}

	if idx < int32(len(list)-1) {
		return idx + 1, list
	}

	return 0, list
}

func NewChecker(backend *pkg.Backend) Checker {
	if backend.Type == pkg.EthBackend {
		return &ETHChecker{
			backend: backend,
			logger: log.NewLog("proxy/eth_checker"),
		}
	}

	return nil
}

type Checker interface {
	Check() bool
}

type ETHChecker struct {
	backend *pkg.Backend
	logger log15.Logger
}

func (e *ETHChecker) Check() bool {
	id := time.Now().Unix()
	data := fmt.Sprintf(ethCheckBody, id)
	client := &http.Client{
		Timeout: time.Duration(2 * time.Second),
	}
	res, err := client.Post(e.backend.URL, "application/json", strings.NewReader(data))
	defer res.Body.Close()
	if err != nil {
		e.logger.Warn("backend returned non-200 response", "name", e.backend.Name, "url", e.backend.URL)
		return false
	}
	var dec map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&dec)
	if err != nil {
		logger.Warn("backend returned invalid JSON", "name", e.backend.Name, "url", e.backend.URL)
		return false
	}
	if _, ok := dec["result"].(bool); !ok {
		logger.Warn("backend is either completing initial sync or has fallen behind", "name", e.backend.Name, "url", e.backend.URL)
		return false
	}
	return true
}


