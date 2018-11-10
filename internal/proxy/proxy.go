package proxy

import (
	"github.com/kyokan/chaind/pkg"
	"github.com/kyokan/chaind/internal/storage"
	"github.com/kyokan/chaind/pkg/log"
	"github.com/kyokan/chaind/pkg/config"
	"net/http"
	"fmt"
	"context"
	"time"
	"net/url"
	"net/http/httputil"
	"github.com/pkg/errors"
	"io/ioutil"
	"bytes"
	"github.com/kyokan/chaind/internal/audit"
)

var logger = log.NewLog("proxy")

type Proxy struct {
	backendSwitch *BackendSwitch
	store         storage.Store
	auditor       audit.Auditor
	config        *config.Config
	quitChan      chan bool
	errChan       chan error
}

func NewProxy(store storage.Store, auditor audit.Auditor, config *config.Config) *Proxy {
	return &Proxy{
		store:    store,
		auditor:  auditor,
		config:   config,
		quitChan: make(chan bool),
		errChan:  make(chan error),
	}
}

func (p *Proxy) Start() error {
	backends, err := p.store.GetBackends()
	if err != nil {
		return err
	}

	if len(backends) == 0 {
		return errors.New("no backends configured")
	}

	var ethBackends []pkg.Backend
	var btcBackends []pkg.Backend

	for _, backend := range backends {
		if backend.Type == pkg.EthereumBackendType {
			ethBackends = append(ethBackends, backend)
		} else {
			btcBackends = append(btcBackends, backend)
		}
	}

	backendSwitch, err := NewBackendSwitch(ethBackends, btcBackends)
	if err != nil {
		return err
	}
	p.backendSwitch = backendSwitch
	if err := p.backendSwitch.Start(); err != nil {
		return err
	}

	if p.config.UseTLS {
		panic("TLS not implemented yet")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/%s", p.config.ETHUrl), p.handleETHRequest)
	s := new(http.Server)
	s.Addr = fmt.Sprintf(":%d", p.config.RPCPort)
	s.Handler = mux

	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("proxy server error", "port", p.config.RPCPort, "err", err)
		}
	}()

	go func() {
		<-p.quitChan
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.Shutdown(ctx); err != nil {
			p.errChan <- err
		}
		if err := p.backendSwitch.Stop(); err != nil {
			p.errChan <- err
		}
		p.errChan <- nil
	}()

	logger.Info("started")
	return nil
}

func (p *Proxy) Stop() error {
	p.quitChan <- true
	return <-p.errChan
}

func (p *Proxy) handleETHRequest(res http.ResponseWriter, req *http.Request) {
	start := time.Now()
	backend, err := p.backendSwitch.BackendFor(pkg.EthereumBackendType)
	if err != nil {
		res.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	body, _ := ioutil.ReadAll(req.Body)
	bodyReader := ioutil.NopCloser(bytes.NewBuffer(body))
	wrapped := pkg.WrapRequest(req, body)
	err = p.auditor.RecordRequest(wrapped, pkg.EthereumBackendType)
	if err != nil {
		logger.Error("failed to record request in auditor", "err", err)
	}
	u, _ := url.Parse(backend.URL)
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.URL.Path = u.Path
	req.Host = u.Host
	req.Body = bodyReader
	proxy := httputil.NewSingleHostReverseProxy(u)
	ctx, _ := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	req = req.WithContext(ctx)
	proxy.ServeHTTP(res, req)
	elapsed := time.Since(start)
	logger.Info("completed ETH proxy request", "elapsed", elapsed)
}
