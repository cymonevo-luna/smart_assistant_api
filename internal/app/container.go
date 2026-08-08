package app

import (
	"context"
	"fmt"

	"github.com/cymonevo/go_template/internal/config"
	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/cymonevo/go_template/internal/handler"
	"github.com/cymonevo/go_template/internal/infra/mongo"
	"github.com/cymonevo/go_template/internal/infra/postgres"
	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/queue"
	"github.com/cymonevo/go_template/pkg/ratelimit"
	"github.com/cymonevo/go_template/pkg/store"
	"github.com/cymonevo/go_template/pkg/validator"
	"github.com/cymonevo/go_template/pkg/worker"
	"github.com/redis/go-redis/v9"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

// Container is a simple, explicit dependency-injection container. Every
// dependency is constructed once here via constructor injection and shared by
// reference. This keeps wiring transparent and testable; for larger projects
// the same constructors can be fed to a framework such as uber-go/fx or
// google/wire without changing the components themselves.
type Container struct {
	Cfg       *config.Config
	Log       logger.Logger
	Validator *validator.Validator
	Tokens    *auth.TokenManager

	Cache       cache.Cache
	Queue       queue.Queue
	RateLimiter ratelimit.Limiter
	TxManager   store.TxManager
	Scheduler   *worker.Scheduler

	UserService *user.Service

	redisClient *redis.Client
	pingers     map[string]handler.Pinger
	closers     []func(context.Context) error
}

// BuildContainer constructs all dependencies, selecting concrete database,
// cache, and queue backends based on configuration.
func BuildContainer(ctx context.Context, cfg *config.Config, log logger.Logger) (*Container, error) {
	c := &Container{
		Cfg:       cfg,
		Log:       log,
		Validator: validator.New(),
		Tokens:    auth.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.JWTTTL, cfg.Auth.RefreshTTL, cfg.Auth.Issuer),
		Scheduler: worker.NewScheduler(log),
		pingers:   map[string]handler.Pinger{},
	}

	if err := c.buildRedis(ctx); err != nil {
		return nil, err
	}
	if err := c.buildCache(ctx); err != nil {
		return nil, err
	}
	if err := c.buildQueue(); err != nil {
		return nil, err
	}
	c.buildRateLimiter()

	// The database backend is chosen here and ONLY here. The returned store and
	// transaction manager satisfy store.Store[user.User] and store.TxManager
	// regardless of engine, so no repository/service/handler code is aware of
	// the choice.
	userStore, err := c.buildUserStore(ctx)
	if err != nil {
		return nil, err
	}

	userRepo := user.NewRepository(userStore)
	userCache := cache.NewTyped[user.User](c.Cache, cfg.Cache.TTL)
	c.UserService = user.NewService(userRepo, userCache, c.TxManager, c.Queue, c.Tokens, log)

	c.registerJobs()
	return c, nil
}

func (c *Container) buildRedis(ctx context.Context) error {
	if !c.Cfg.UsesRedis() {
		return nil
	}
	client := redis.NewClient(&redis.Options{
		Addr:     c.Cfg.Cache.Addr,
		Password: c.Cfg.Cache.Password,
		DB:       c.Cfg.Cache.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	c.redisClient = client
	c.pingers["redis"] = pingerFunc(func(ctx context.Context) error { return client.Ping(ctx).Err() })
	c.closers = append(c.closers, func(context.Context) error { return client.Close() })
	return nil
}

func (c *Container) buildCache(ctx context.Context) error {
	switch c.Cfg.Cache.Driver {
	case config.CacheRedis:
		c.Cache = cache.NewRedisWithClient(c.redisClient)
		c.Log.Info("cache backend ready", logger.String("driver", "redis"))
	case config.CacheMemory:
		mc := cache.NewMemory()
		c.Cache = mc
		c.closers = append(c.closers, func(context.Context) error { return mc.Close() })
		c.Log.Info("cache backend ready", logger.String("driver", "memory"))
	default:
		return fmt.Errorf("unsupported cache driver %q", c.Cfg.Cache.Driver)
	}
	return nil
}

func (c *Container) buildQueue() error {
	policy := queue.RetryPolicy{MaxAttempts: c.Cfg.Queue.MaxAttempts, Backoff: c.Cfg.Queue.Backoff}
	switch c.Cfg.Queue.Driver {
	case config.QueueRedis:
		c.Queue = queue.NewRedis(c.redisClient, c.Log, policy)
		c.Log.Info("queue backend ready", logger.String("driver", "redis"))
	case config.QueueMemory:
		c.Queue = queue.NewMemory(c.Log, policy)
		c.Log.Info("queue backend ready", logger.String("driver", "memory"))
	default:
		return fmt.Errorf("unsupported queue driver %q", c.Cfg.Queue.Driver)
	}
	c.closers = append(c.closers, func(context.Context) error { return c.Queue.Close() })
	return nil
}

func (c *Container) buildRateLimiter() {
	if c.redisClient != nil {
		c.RateLimiter = ratelimit.NewRedis(c.redisClient)
		c.Log.Info("rate limiter ready", logger.String("driver", "redis"))
		return
	}
	ml := ratelimit.NewMemory()
	c.RateLimiter = ml
	c.closers = append(c.closers, func(context.Context) error { return ml.Close() })
	c.Log.Info("rate limiter ready", logger.String("driver", "memory"))
}

// buildUserStore is the single switch point between databases. It also builds
// the matching transaction manager.
func (c *Container) buildUserStore(ctx context.Context) (store.Store[user.User], error) {
	schema := store.Schema{Name: user.TableName, IDColumn: "id"}

	switch c.Cfg.Database.Driver {
	case config.DriverPostgres:
		pool, err := postgres.Connect(ctx, c.Cfg.Database)
		if err != nil {
			return nil, err
		}
		c.Log.Info("database ready", logger.String("driver", "postgres"))
		c.TxManager = store.NewPostgresTxManager(pool)
		c.pingers["postgres"] = pingerFunc(func(ctx context.Context) error { return pool.Ping(ctx) })
		c.closers = append(c.closers, func(context.Context) error { pool.Close(); return nil })
		return store.NewPostgresStore[user.User](pool, schema), nil

	case config.DriverMongo:
		client, db, err := mongo.Connect(ctx, c.Cfg.Database)
		if err != nil {
			return nil, err
		}
		c.Log.Info("database ready", logger.String("driver", "mongo"))
		c.TxManager = store.NewMongoTxManager(client)
		c.pingers["mongo"] = pingerFunc(func(ctx context.Context) error { return client.Ping(ctx, nil) })
		c.closers = append(c.closers, func(ctx context.Context) error { return disconnectMongo(ctx, client) })
		return store.NewMongoStore[user.User](db, schema), nil

	default:
		return nil, fmt.Errorf("unsupported database driver %q", c.Cfg.Database.Driver)
	}
}

// registerJobs wires background job handlers and scheduled tasks. This is the
// consumer side of the message queue.
func (c *Container) registerJobs() {
	worker.Register(c.Queue, user.TopicUserCreated, func(ctx context.Context, evt user.UserCreatedEvent) error {
		// Stand-in for real side effects: send a welcome email, provision
		// resources, emit analytics, etc.
		c.Log.Info("processing user.created job",
			logger.String("user_id", evt.UserID),
			logger.String("email", evt.Email))
		return nil
	})
}

// StartBackground launches the queue consumers and the periodic scheduler.
func (c *Container) StartBackground(ctx context.Context) error {
	if err := c.Queue.Start(ctx); err != nil {
		return err
	}
	c.Scheduler.Start(ctx)
	// Stop the scheduler first during shutdown so in-flight task runs drain
	// (within the shutdown grace period) before the queue and datastores close.
	c.closers = append(c.closers, func(ctx context.Context) error {
		c.Scheduler.Stop(ctx)
		return nil
	})
	return nil
}

// Close runs all registered closers in reverse construction order.
func (c *Container) Close(ctx context.Context) {
	for i := len(c.closers) - 1; i >= 0; i-- {
		if err := c.closers[i](ctx); err != nil {
			c.Log.Error("error during shutdown", logger.Err(err))
		}
	}
}

func disconnectMongo(ctx context.Context, client *mongodriver.Client) error {
	return client.Disconnect(ctx)
}

// pingerFunc adapts a function to the handler.Pinger interface.
type pingerFunc func(ctx context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }
