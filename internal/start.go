package internal

import (
	"github.com/kyokan/chaind/pkg/config"
	"github.com/kyokan/chaind/internal/storage"
	"github.com/kyokan/chaind/internal/proxy"
	"os"
	"os/signal"
	"syscall"
	"github.com/kyokan/chaind/pkg/log"
	"github.com/kyokan/chaind/internal/audit"
)

func Start(cfg *config.Config) error {
	store, err := storage.StorageFromURL(cfg.DBUrl)
	if err != nil {
		return err
	}
	if err := store.Start(); err != nil {
		return err
	}

	auditor, err := audit.NewLogAuditor(cfg.LogAuditorConfig)
	if err != nil {
		return err
	}

	prox := proxy.NewProxy(store, auditor, cfg)
	if err := prox.Start(); err != nil {
		return err
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	logger := log.NewLog("")

	go func() {
		<-sigs
		logger.Info("interrupted, shutting down")
		if err := store.Stop(); err != nil {
			logger.Error("failed to stop storage", "err", err)
		}
		if err := prox.Stop(); err != nil {
			logger.Error("failed to stop proxy", "err", err)
		}
		done <- true
	}()

	<-done
	logger.Info("goodbye")
	return nil
}
