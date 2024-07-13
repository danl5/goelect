package rpc

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/danl5/goelect/pkg/model"
	"github.com/mitchellh/mapstructure"
	"github.com/silenceper/pool"
	"github.com/ugorji/go/codec"
)

const (
	// initial capacity of the pool
	poolInitCap = 0
	// maximum number of idle connections in the pool
	poolMaxIdle = 5
	// maximum time a connection can be idle before being closed
	poolMaxIdleTime = 15
	// maximum number of connections in the pool
	poolMaxCap = 20
)

func NewRPC(logger *slog.Logger) (*RPC, error) {
	if logger == nil {
		return nil, fmt.Errorf("new rpc, logger is nil")
	}

	rpc := &RPC{
		Server: Server{
			logger: logger.With("component", "rpc server"),
		},
		Client: Client{
			logger: logger.With("component", "rpc client"),
		},
	}

	return rpc, nil
}

type RPCHandler struct {
	CmdHandler model.CommandHandler
}

func (h *RPCHandler) Handle(request *model.Request, response *model.Response) error {
	return h.CmdHandler(request, response)
}

func (h *RPCHandler) Ping(_ struct{}, reply *string) error {
	*reply = "pong"
	return nil
}

type RPC struct {
	Server
	Client
}

func (r *RPC) Decode(raw any, target any) error {
	decodeHook := func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if t.Kind() == reflect.String && f.Kind() == reflect.Slice {
			if bytes, ok := data.([]uint8); ok {
				return string(bytes), nil
			}
		}
		return data, nil
	}

	paramCheck := func(a any) bool {
		t := reflect.TypeOf(a)
		if t.Kind() == reflect.Ptr {
			return !reflect.ValueOf(a).IsNil()
		}

		return false
	}

	if !paramCheck(target) {
		return fmt.Errorf("wrong receiver for decode")
	}

	decoderConfig := &mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     &target,
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return err
	}
	if err := decoder.Decode(raw); err != nil {
		return err
	}

	return nil
}

type Server struct {
	rpcHandler *RPCHandler
	logger     *slog.Logger
}

// Start initiates the server to begin listening on the specified address.
func (s *Server) Start(listenAddress string, handler model.CommandHandler, serverConfig model.TransportConfig) error {
	cfg, ok := serverConfig.(*Config)
	if !ok {
		return errors.New("not a valid rpc server config")
	}

	err := cfg.Validate()
	if err != nil {
		return err
	}

	s.rpcHandler = &RPCHandler{
		CmdHandler: handler,
	}

	err = s.startServer(listenAddress, s.rpcHandler, cfg)
	if err != nil {
		s.logger.Error("failed to start rpc server", "error", err.Error())
		return err
	}

	s.logger.Info("rpc server started", "listenAddress", listenAddress)
	return nil
}

func (s *Server) startServer(listenAddress string, handler *RPCHandler, cfg *Config) error {
	tlsConfig, err := s.loadTLSConfig(cfg)
	if err != nil {
		return err
	}

	rpcServer := rpc.NewServer()
	err = rpcServer.Register(handler)
	if err != nil {
		return err
	}

	var l net.Listener
	if tlsConfig != nil {
		l, err = tls.Listen("tcp", listenAddress, tlsConfig)
		if err != nil {
			return err
		}
	} else {
		l, err = net.Listen("tcp", listenAddress)
		if err != nil {
			return err
		}
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				s.logger.Error("failed to accept rpc connection", "error", err.Error())
			}

			rpcCodec := codec.MsgpackSpecRpc.ServerCodec(conn, &codec.MsgpackHandle{})
			go rpcServer.ServeCodec(rpcCodec)
		}
	}()
	return nil
}

func (s *Server) loadTLSConfig(cfg *Config) (*tls.Config, error) {
	// if no TLS config is provided, return nil
	if cfg.ServerCert == "" || cfg.ServerKey == "" {
		return nil, nil
	}

	config := &tls.Config{}
	if cfg.ServerCert != "" && cfg.ServerKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ServerCert, cfg.ServerKey)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}

	caCertPool := x509.NewCertPool()
	for _, serverCA := range cfg.ServerCAs {
		caCert, err := os.ReadFile(serverCA)
		if err != nil {
			return nil, err
		}
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, err
		}
	}
	config.ClientCAs = caCertPool
	config.ClientAuth = tls.RequireAndVerifyClientCert
	if cfg.ServerSkipVerify {
		config.ClientAuth = tls.NoClientCert
	}

	return config, nil
}

type Client struct {
	// node id to client
	// string -> pool.Pool
	clients sync.Map

	logger *slog.Logger
}

// InitConnections initializes a set of connections to the given nodes.
// It returns an error if any connection fails.
func (c *Client) InitConnections(nodes []*model.Node, cfg model.TransportConfig) error {
	for _, node := range nodes {
		cfg, ok := cfg.(*Config)
		if !ok {
			return errors.New("not a valid rpc client config")
		}

		p, err := c.createClient(*node, cfg)
		if err != nil {
			c.logger.Error("error connecting to node", "node", node.ID)
			return err
		}
		c.clients.Store(node.ID, p)
	}
	return nil
}

// SendRequest sends the command request
func (c *Client) SendRequest(nodeId string, request *model.Request, response *model.Response) error {
	rpcClient, err := c.getClient(nodeId)
	if err != nil {
		return err
	}
	if rpcClient == nil {
		return fmt.Errorf("no rpc client found for node %s", nodeId)
	}

	err = rpcClient.Call("RPCHandler.Handle", request, response)
	if err != nil {
		return fmt.Errorf("failed to call rpc handler: %s", err.Error())
	}
	// put back to pool if no error
	defer func() {
		err := c.putClient(nodeId, rpcClient)
		if err != nil {
			c.logger.Error("failed to put rpc client back to pool", "error", err.Error())
		}
	}()

	c.logger.Debug("send rpc request", "command", request.CommandCode.String(), "to", nodeId)
	return nil
}

func (c *Client) createClient(node model.Node, cfg *Config) (pool.Pool, error) {
	poolConfig := &pool.Config{
		InitialCap:  poolInitCap,
		MaxIdle:     poolMaxIdle,
		MaxCap:      poolMaxCap,
		IdleTimeout: poolMaxIdleTime * time.Second,
		Factory: func() (interface{}, error) {
			tlsConfig, err := c.loadTLSConfig(cfg)
			if err != nil {
				return nil, err
			}
			connectTimeout := time.Duration(cfg.ConnectTimeout) * time.Second
			var conn net.Conn
			dialer := &net.Dialer{
				Timeout: connectTimeout,
			}
			if tlsConfig != nil {
				conn, err = tls.DialWithDialer(dialer, "tcp", node.Address, tlsConfig)
				if err != nil {
					return nil, err
				}
			} else {
				conn, err = dialer.Dial("tcp", node.Address)
				if err != nil {
					return nil, err
				}
			}

			rpcCodec := codec.MsgpackSpecRpc.ClientCodec(conn, &codec.MsgpackHandle{})
			return rpc.NewClientWithCodec(rpcCodec), nil
		},
		Close: func(v interface{}) error { return v.(*rpc.Client).Close() },
		Ping: func(v interface{}) error {
			var reply string
			return v.(*rpc.Client).Call("RPCHandler.Ping", nil, &reply)
		},
	}
	p, err := pool.NewChannelPool(poolConfig)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (c *Client) getClient(nodeId string) (*rpc.Client, error) {
	clientPoolInf, ok := c.clients.Load(nodeId)
	if !ok {
		return nil, fmt.Errorf("no client pool found for node %s", nodeId)
	}
	clientPool := clientPoolInf.(pool.Pool)
	conn, err := clientPool.Get()
	if err != nil {
		return nil, fmt.Errorf("can not get client from pool for node %s: %s", nodeId, err.Error())
	}

	return conn.(*rpc.Client), nil
}

func (c *Client) putClient(nodeId string, client *rpc.Client) error {
	clientPoolInf, ok := c.clients.Load(nodeId)
	if !ok {
		return fmt.Errorf("no client pool found for node %s", nodeId)
	}
	clientPool := clientPoolInf.(pool.Pool)
	err := clientPool.Put(client)
	if err != nil {
		return fmt.Errorf("failed to put client back to pool for node %s: %s", nodeId, err.Error())
	}

	return nil
}

func (c *Client) loadTLSConfig(cfg *Config) (*tls.Config, error) {
	// if no TLS config is provided, return nil
	if cfg.ClientCert == "" || cfg.ClientKey == "" {
		return nil, nil
	}

	config := &tls.Config{}
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}

	caCertPool := x509.NewCertPool()
	for _, clientCA := range cfg.ClientCAs {
		caCert, err := os.ReadFile(clientCA)
		if err != nil {
			return nil, err
		}
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, err
		}
	}
	config.RootCAs = caCertPool
	config.InsecureSkipVerify = false
	if cfg.ClientSkipVerify {
		config.InsecureSkipVerify = true
	}

	return config, nil
}
