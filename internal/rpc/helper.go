package rpc

import (
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"github.com/silenceper/pool"

	"github.com/danl5/goelect/internal/log"
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

// NewRpcClient create a new rpc client
func NewRpcClient(
	addr string,
	timeout time.Duration,
	logger log.Logger,
	ping func(client *rpc.Client) error) (*Client, error) {

	poolConfig := &pool.Config{
		InitialCap:  poolInitCap,
		MaxIdle:     poolMaxIdle,
		MaxCap:      poolMaxCap,
		IdleTimeout: poolMaxIdleTime * time.Second,
		Factory: func() (interface{}, error) {
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				return nil, err
			}
			return jsonrpc.NewClient(conn), nil
		},
		Close: func(v interface{}) error { return v.(*rpc.Client).Close() },
		Ping:  func(i interface{}) error { return ping(i.(*rpc.Client)) },
	}
	p, err := pool.NewChannelPool(poolConfig)
	if err != nil {
		return nil, err
	}

	return &Client{connPool: p, logger: logger}, nil
}

// Client represents a rpc client
type Client struct {
	connPool pool.Pool
	logger   log.Logger
}

// Call the remote method
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

// NewRpcServer create a rpc server instance
func NewRpcServer(logger log.Logger) (*Server, error) {
	return &Server{
		logger: logger,
	}, nil
}

type Server struct {
	logger log.Logger
}

// Start registers the handler and starts the server
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
			s.logger.Error("RPC Server, error accepting", "error", err.Error())
			continue
		}
		go server.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
