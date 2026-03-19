// Package redis provides Redis integration for caching, rate limiting, and session management.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds Redis connection settings.
type Config struct {
	Addr       string
	Username   string
	Password   string
	DB         int
	TLSEnable  bool
	PoolSize   int
	OpTimeout  time.Duration
}

// Client wraps go-redis with telekube-specific operations.
type Client struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// New creates a new Redis client.
func New(cfg Config, logger *zap.Logger) (*Client, error) {
	opts := &redis.Options{
		Addr:            cfg.Addr,
		Username:        cfg.Username,
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        cfg.PoolSize,
		ReadTimeout:     cfg.OpTimeout,
		WriteTimeout:    cfg.OpTimeout,
		MinIdleConns:    5,
		ConnMaxIdleTime: 5 * time.Minute,
	}

	if opts.PoolSize == 0 {
		opts.PoolSize = 20
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 2 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 2 * time.Second
	}

	if cfg.TLSEnable {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	logger.Info("redis connected",
		zap.String("addr", cfg.Addr),
		zap.Int("pool_size", opts.PoolSize),
	)

	return &Client{
		rdb:    rdb,
		logger: logger,
	}, nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Ping checks Redis connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Underlying returns the underlying go-redis client.
func (c *Client) Underlying() *redis.Client {
	return c.rdb
}
