package rpc

import (
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"github.com/silenceper/pool"

	"github.com/danli001/goelect/internal/log"
)

const (
	poolInitCap     = 2
	poolMaxIdle     = 20
	poolMaxIdleTime = 15
	poolMaxCap      = 30
)

func NewRpcClient(addr string, logger log.Logger) (*Client, error) {
	poolConfig := &pool.Config{
		InitialCap:  poolInitCap,
		MaxIdle:     poolMaxIdle,
		MaxCap:      poolMaxCap,
		IdleTimeout: poolMaxIdleTime * time.Second,
		Factory:     func() (interface{}, error) { return jsonrpc.Dial("tcp", addr) },
		Close:       func(v interface{}) error { return v.(*rpc.Client).Close() },
	}
	p, err := pool.NewChannelPool(poolConfig)
	if err != nil {
		return nil, err
	}

	return &Client{connPool: p, logger: logger}, nil
}

type Client struct {
	connPool pool.Pool
	logger   log.Logger
}

func (c *Client) Call(method string, args any, reply any) error {
	conn, err := c.connPool.Get()
	if err != nil {
		return err
	}
	defer c.connPool.Put(conn)

	client := conn.(*rpc.Client)
	c.logger.Debug("rpc call", "method", method, "args", args)
	err = client.Call(method, args, reply)
	if err != nil {
		return err
	}

	return nil
}

func NewRpcServer(logger log.Logger) (*Server, error) {
	return &Server{
		logger: logger,
	}, nil
}

type Server struct {
	logger log.Logger
}

func (s *Server) Start(addr string, handler any) error {
	server := rpc.NewServer()
	// register handler
	err := server.Register(handler)
	if err != nil {
		return err
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer l.Close()

	s.logger.Info("RPC Server, start to listen", "address", addr)
	// start listener
	for {
		conn, err := l.Accept()
		if err != nil {
			s.logger.Error("RPC Server, error accepting: %v", err)
			continue
		}
		go server.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
