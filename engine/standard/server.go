package standard

import (
	"net/http"
	"sync"

	"github.com/lessgo/lessgo"
	"github.com/lessgo/lessgo/engine"
	"github.com/lessgo/lessgo/logs"
)

type (
	// Server implements `engine.Server`.
	Server struct {
		*http.Server
		config  engine.Config
		handler engine.Handler
		logger  logs.Logger
		pool    *pool
	}

	pool struct {
		request         sync.Pool
		response        sync.Pool
		responseAdapter sync.Pool
		header          sync.Pool
		url             sync.Pool
	}
)

// New returns an instance of `standard.Server` with provided listen address.
func New(addr string) engine.Server {
	c := engine.Config{Address: addr}
	return NewFromConfig(c)
}

// NewFromTLS returns an instance of `standard.Server` from TLS config.
func NewFromTLS(addr, certfile, keyfile string) engine.Server {
	c := engine.Config{
		Address:     addr,
		TLSCertfile: certfile,
		TLSKeyfile:  keyfile,
	}
	return NewFromConfig(c)
}

// NewFromConfig returns an instance of `standard.Server` from config.
func NewFromConfig(c engine.Config) engine.Server {
	var s *Server
	s = &Server{
		Server: new(http.Server),
		config: c,
		pool: &pool{
			request: sync.Pool{
				New: func() interface{} {
					return &Request{logger: s.logger}
				},
			},
			response: sync.Pool{
				New: func() interface{} {
					return &Response{logger: s.logger}
				},
			},
			responseAdapter: sync.Pool{
				New: func() interface{} {
					return &responseAdapter{}
				},
			},
			header: sync.Pool{
				New: func() interface{} {
					return &Header{}
				},
			},
			url: sync.Pool{
				New: func() interface{} {
					return &URL{}
				},
			},
		},
		handler: engine.HandlerFunc(func(rq engine.Request, rs engine.Response) {
			s.logger.Error("handler not set, use `SetHandler()` to set it.")
		}),
		logger: logs.NewLogger(),
	}
	s.Addr = c.Address
	s.Handler = s
	return s
}

// SetHandler implements `engine.Server#SetHandler` function.
func (s *Server) SetHandler(h engine.Handler) {
	s.handler = h
}

// SetLogger implements `engine.Server#SetLogger` function.
func (s *Server) SetLogger(l logs.Logger) {
	s.logger = l
}

// Start implements `engine.Server#Start` function.
func (s *Server) Start() error {
	if s.config.Listener == nil {
		return s.startDefaultListener()
	}
	return s.startCustomListener()
}

func (s *Server) startDefaultListener() error {
	c := s.config
	if c.TLSCertfile != "" && c.TLSKeyfile != "" {
		return s.ListenAndServeTLS(c.TLSCertfile, c.TLSKeyfile)
	}
	return s.ListenAndServe()
}

func (s *Server) startCustomListener() error {
	return s.Serve(s.config.Listener)
}

// ServeHTTP implements `http.Handler` interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Request
	rq := s.pool.request.Get().(*Request)
	rqHdr := s.pool.header.Get().(*Header)
	rqURL := s.pool.url.Get().(*URL)
	rqHdr.reset(r.Header)
	rqURL.reset(r.URL)
	rq.reset(r, rqHdr, rqURL)

	// Response
	rs := s.pool.response.Get().(*Response)
	rsAdpt := s.pool.responseAdapter.Get().(*responseAdapter)
	rsAdpt.reset(w, rs)
	rsHdr := s.pool.header.Get().(*Header)
	rsHdr.reset(w.Header())
	rs.reset(w, rsAdpt, rsHdr)

	s.handler.ServeHTTP(rq, rs)

	// Return to pool
	s.pool.request.Put(rq)
	s.pool.header.Put(rqHdr)
	s.pool.url.Put(rqURL)
	s.pool.response.Put(rs)
	s.pool.header.Put(rsHdr)
}

// WrapHandler wraps `http.Handler` into `lessgo.HandlerFunc`.
func WrapHandler(h http.Handler) lessgo.HandlerFunc {
	return func(c lessgo.Context) error {
		rq := c.Request().(*Request)
		rs := c.Response().(*Response)
		h.ServeHTTP(rs.ResponseWriter, rq.Request)
		return nil
	}
}

// WrapMiddleware wraps `func(http.Handler) http.Handler` into `lessgo.MiddlewareFunc`
func WrapMiddleware(m func(http.Handler) http.Handler) lessgo.MiddlewareFunc {
	return func(next lessgo.Handler) lessgo.Handler {
		return lessgo.HandlerFunc(func(c lessgo.Context) (err error) {
			rq := c.Request().(*Request)
			rs := c.Response().(*Response)
			m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err = next.Handle(c)
			})).ServeHTTP(rs.ResponseWriter, rq.Request)
			return
		})
	}
}
