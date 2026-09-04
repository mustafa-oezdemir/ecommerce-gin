package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

type HTTPConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

type Group struct {
	servers         []namedServer
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

type namedServer struct {
	name   string
	server *http.Server
}

func NewGroup(applicationAddress string, applicationHandler http.Handler, metricsAddress string, metricsHandler http.Handler, config HTTPConfig, logger *slog.Logger) (*Group, error) {
	if applicationAddress == "" || metricsAddress == "" {
		return nil, errors.New("server: application and metrics addresses are required")
	}
	if applicationHandler == nil || metricsHandler == nil {
		return nil, errors.New("server: application and metrics handlers are required")
	}
	if config.ReadHeaderTimeout <= 0 || config.ReadTimeout <= 0 || config.WriteTimeout <= 0 || config.IdleTimeout <= 0 || config.ShutdownTimeout <= 0 {
		return nil, errors.New("server: all timeouts must be positive")
	}
	if config.MaxHeaderBytes < 8192 {
		return nil, errors.New("server: MaxHeaderBytes must be at least 8192")
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Group{
		servers: []namedServer{
			{name: "application", server: newHTTPServer(applicationAddress, applicationHandler, config, logger)},
			{name: "metrics", server: newHTTPServer(metricsAddress, metricsHandler, config, logger)},
		},
		shutdownTimeout: config.ShutdownTimeout,
		logger:          logger,
	}, nil
}

func newHTTPServer(address string, handler http.Handler, config HTTPConfig, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

func (group *Group) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("server: run context is required")
	}
	serveContext, cancelServing := context.WithCancel(context.Background())
	defer cancelServing()
	listeners := make([]net.Listener, 0, len(group.servers))
	for _, configuredServer := range group.servers {
		configuredServer.server.BaseContext = func(net.Listener) context.Context { return serveContext }
		listener, err := net.Listen("tcp", configuredServer.server.Addr)
		if err != nil {
			for _, openedListener := range listeners {
				_ = openedListener.Close()
			}
			return fmt.Errorf("listen for %s server: %w", configuredServer.name, err)
		}
		listeners = append(listeners, listener)
	}

	serverErrors := make(chan error, len(group.servers))
	for index, configuredServer := range group.servers {
		configuredServer := configuredServer
		listener := listeners[index]
		go func() {
			group.logger.Info("http server listening", "server", configuredServer.name, "address", listener.Addr().String())
			if err := configuredServer.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- fmt.Errorf("%s server: %w", configuredServer.name, err)
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
		group.logger.Info("http server shutdown requested")
	case err := <-serverErrors:
		group.logger.Error("http server failure triggered shutdown", "error", err)
		runErr = err
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), group.shutdownTimeout)
	defer cancelShutdown()
	shutdownErrors := make(chan error, len(group.servers))
	var shutdowns sync.WaitGroup
	for _, configuredServer := range group.servers {
		configuredServer := configuredServer
		shutdowns.Add(1)
		go func() {
			defer shutdowns.Done()
			if err := configuredServer.server.Shutdown(shutdownContext); err != nil {
				closeErr := configuredServer.server.Close()
				shutdownErrors <- errors.Join(fmt.Errorf("shutdown %s server: %w", configuredServer.name, err), closeErr)
			}
		}()
	}
	shutdowns.Wait()
	cancelServing()
	close(shutdownErrors)
	for err := range shutdownErrors {
		runErr = errors.Join(runErr, err)
	}
	return runErr
}
