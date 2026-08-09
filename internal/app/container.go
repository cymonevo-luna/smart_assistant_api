package app

import (
	"context"
	"fmt"
	"time"

	"github.com/cymonevo/go_template/internal/config"
	"github.com/cymonevo/go_template/internal/domain/assistant"
	"github.com/cymonevo/go_template/internal/domain/assistant_settings"
	"github.com/cymonevo/go_template/internal/domain/calendar/availability"
	"github.com/cymonevo/go_template/internal/domain/plugin"
	"github.com/cymonevo/go_template/internal/domain/plugin_credential"
	"github.com/cymonevo/go_template/internal/domain/plugin_setup/composio_form"
	"github.com/cymonevo/go_template/internal/domain/plugin_setup/oauth_google"
	"github.com/cymonevo/go_template/internal/domain/reminder"
	"github.com/cymonevo/go_template/internal/domain/user"
	"github.com/cymonevo/go_template/internal/domain/user_plugin"
	"github.com/cymonevo/go_template/internal/handler"
	"github.com/cymonevo/go_template/internal/infra/mongo"
	"github.com/cymonevo/go_template/internal/infra/postgres"
	"github.com/cymonevo/go_template/pkg/auth"
	"github.com/cymonevo/go_template/pkg/cache"
	"github.com/cymonevo/go_template/pkg/composio"
	"github.com/cymonevo/go_template/pkg/crypto"
	"github.com/cymonevo/go_template/pkg/llm"
	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/cymonevo/go_template/pkg/places"
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

	UserService              *user.Service
	AssistantSettingsService *assistantsettings.Service
	AssistantService         *assistant.Service
	PluginService            *plugin.Service
	UserPluginService        *userplugin.Service
	UserPluginRepo           userplugin.Repository
	PluginRepo               plugin.Repository
	PluginCredentialService  *plugincredential.Service
	ReminderService          *reminder.Service
	ReminderRepo             reminder.Repository
	GoogleOAuthSetupService  *oauthgoogle.Service
	ComposioFormSetupService *composioform.Service
	PlacesProvider           places.Provider
	CalendarAvailability     availability.AvailabilityService

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

	// The database backend is chosen here and ONLY here. The returned stores and
	// transaction manager satisfy store.Store[T] and store.TxManager regardless
	// of engine, so no repository/service/handler code is aware of the choice.
	userStore, assistantSettingsStore, pluginStore, userPluginStore, pluginCredentialStore, reminderStore, assistantSessionStore, assistantMessageStore, err := c.buildStores(ctx)
	if err != nil {
		return nil, err
	}

	userRepo := user.NewRepository(userStore)
	userCache := cache.NewTyped[user.User](c.Cache, cfg.Cache.TTL)
	c.UserService = user.NewService(userRepo, userCache, c.TxManager, c.Queue, c.Tokens, log)

	assistantSettingsRepo := assistantsettings.NewRepository(assistantSettingsStore)
	c.AssistantSettingsService = assistantsettings.NewService(assistantSettingsRepo)

	pluginRepo := plugin.NewRepository(pluginStore)
	c.PluginRepo = pluginRepo
	pluginCache := cache.NewTyped[plugin.Plugin](c.Cache, cfg.Cache.TTL)
	c.PluginService = plugin.NewService(pluginRepo, pluginCache, c.TxManager, log)

	userPluginRepo := userplugin.NewRepository(userPluginStore)
	c.UserPluginRepo = userPluginRepo

	encKey := c.Cfg.Credentials.EncryptionKey
	if encKey == "" {
		encKey = c.Cfg.Auth.JWTSecret
	}
	encryptor, err := crypto.NewEncryptor(encKey)
	if err != nil {
		return nil, fmt.Errorf("credential encryptor: %w", err)
	}

	pluginCredentialRepo := plugincredential.NewRepository(pluginCredentialStore)
	c.PluginCredentialService = plugincredential.NewService(pluginCredentialRepo, encryptor)
	credentialsCleaner := plugincredential.NewCleaner(c.PluginCredentialService, userPluginRepo)

	reminderRepo := reminder.NewRepository(reminderStore)
	c.ReminderRepo = reminderRepo
	c.ReminderService = reminder.NewService(reminderRepo)
	reminderCleaner := reminder.NewCleaner(c.ReminderService)
	c.UserPluginService = userplugin.NewService(userPluginRepo, pluginRepo, credentialsCleaner, reminderCleaner)

	googleCfg := oauthgoogle.Config{
		ClientID:     c.Cfg.OAuthGoogle.ClientID,
		ClientSecret: c.Cfg.OAuthGoogle.ClientSecret,
		RedirectURL:  c.Cfg.OAuthGoogle.RedirectURL,
		TokenURL:     c.Cfg.OAuthGoogle.TokenURL,
	}
	googleExchanger := oauthgoogle.NewHTTPClient(googleCfg, nil)
	c.GoogleOAuthSetupService = oauthgoogle.NewService(
		googleCfg,
		c.Cfg.Auth.JWTSecret,
		userPluginRepo,
		pluginRepo,
		c.PluginCredentialService,
		googleExchanger,
	)

	c.ComposioFormSetupService = composioform.NewService(
		composioform.Config{BaseURL: c.Cfg.Composio.BaseURL},
		userPluginRepo,
		pluginRepo,
		c.PluginCredentialService,
		nil,
	)

	placesProvider, err := places.NewProvider(places.Config{
		Provider:  c.Cfg.Places.Provider,
		APIKey:    c.Cfg.Places.APIKey,
		BaseURL:   c.Cfg.Places.BaseURL,
		UserAgent: c.Cfg.App.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("places provider: %w", err)
	}
	c.PlacesProvider = places.NewCachingProvider(placesProvider, 5*time.Minute)

	classifier := llm.NewClassifier(c.Cfg.LLM.Provider, c.Cfg.LLM.APIKey, c.Cfg.LLM.Model)
	composioAgent := llm.NewComposioAgent(c.Cfg.LLM.Provider, c.Cfg.LLM.APIKey, c.Cfg.LLM.Model)
	stubExecutor := assistant.NewStubExecutor(log)
	builtinExecutor := assistant.NewBuiltinExecutor(c.ReminderService, c.AssistantSettingsService, c.PlacesProvider, log)
	var composioExec *assistant.ComposioExecutor
	var composioMCPExec *assistant.ComposioMCPExecutor
	if c.Cfg.Composio.APIKey != "" {
		composioClient := composio.New(composio.Config{
			APIKey:  c.Cfg.Composio.APIKey,
			BaseURL: c.Cfg.Composio.BaseURL,
		})
		composioExec = assistant.NewComposioExecutor(composioClient, log)
		c.CalendarAvailability = availability.NewService(composioClient)
		c.Log.Info("composio client ready")
	}
	composioMCPExec = assistant.NewComposioMCPExecutor(c.PluginCredentialService, composioAgent, nil, log)
	executor := assistant.NewRoutingExecutor(composioExec, composioMCPExec, builtinExecutor, stubExecutor)

	assistantSessionRepo := assistant.NewSessionRepository(assistantSessionStore)
	assistantMessageRepo := assistant.NewMessageRepository(assistantMessageStore)
	c.AssistantService = assistant.NewService(
		assistantSessionRepo,
		assistantMessageRepo,
		c.AssistantSettingsService,
		userPluginRepo,
		pluginRepo,
		classifier,
		executor,
		c.CalendarAvailability,
		log,
	)

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

// buildStores is the single switch point between databases. It also builds the
// matching transaction manager and all entity stores from one connection.
func (c *Container) buildStores(ctx context.Context) (
	store.Store[user.User],
	store.Store[assistantsettings.Settings],
	store.Store[plugin.Plugin],
	store.Store[userplugin.UserPlugin],
	store.Store[plugincredential.Credential],
	store.Store[reminder.Reminder],
	store.Store[assistant.Session],
	store.Store[assistant.Message],
	error,
) {
	userSchema := store.Schema{Name: user.TableName, IDColumn: "id"}
	settingsSchema := store.Schema{Name: assistantsettings.TableName, IDColumn: "user_id"}
	pluginSchema := store.Schema{Name: plugin.TableName, IDColumn: "id"}
	userPluginSchema := store.Schema{Name: userplugin.TableName, IDColumn: "id"}
	pluginCredentialSchema := store.Schema{Name: plugincredential.TableName, IDColumn: "id"}
	reminderSchema := store.Schema{Name: reminder.TableName, IDColumn: "id"}
	sessionSchema := store.Schema{Name: assistant.TableNameSessions, IDColumn: "id"}
	messageSchema := store.Schema{Name: assistant.TableNameMessages, IDColumn: "id"}

	switch c.Cfg.Database.Driver {
	case config.DriverPostgres:
		pool, err := postgres.Connect(ctx, c.Cfg.Database)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, err
		}
		c.Log.Info("database ready", logger.String("driver", "postgres"))
		c.TxManager = store.NewPostgresTxManager(pool)
		c.pingers["postgres"] = pingerFunc(func(ctx context.Context) error { return pool.Ping(ctx) })
		c.closers = append(c.closers, func(context.Context) error { pool.Close(); return nil })
		return store.NewPostgresStore[user.User](pool, userSchema),
			store.NewPostgresStore[assistantsettings.Settings](pool, settingsSchema),
			store.NewPostgresStore[plugin.Plugin](pool, pluginSchema),
			store.NewPostgresStore[userplugin.UserPlugin](pool, userPluginSchema),
			store.NewPostgresStore[plugincredential.Credential](pool, pluginCredentialSchema),
			store.NewPostgresStore[reminder.Reminder](pool, reminderSchema),
			store.NewPostgresStore[assistant.Session](pool, sessionSchema),
			store.NewPostgresStore[assistant.Message](pool, messageSchema), nil

	case config.DriverMongo:
		client, db, err := mongo.Connect(ctx, c.Cfg.Database)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, err
		}
		c.Log.Info("database ready", logger.String("driver", "mongo"))
		c.TxManager = store.NewMongoTxManager(client)
		c.pingers["mongo"] = pingerFunc(func(ctx context.Context) error { return client.Ping(ctx, nil) })
		c.closers = append(c.closers, func(ctx context.Context) error { return disconnectMongo(ctx, client) })
		return store.NewMongoStore[user.User](db, userSchema),
			store.NewMongoStore[assistantsettings.Settings](db, settingsSchema),
			store.NewMongoStore[plugin.Plugin](db, pluginSchema),
			store.NewMongoStore[userplugin.UserPlugin](db, userPluginSchema),
			store.NewMongoStore[plugincredential.Credential](db, pluginCredentialSchema),
			store.NewMongoStore[reminder.Reminder](db, reminderSchema),
			store.NewMongoStore[assistant.Session](db, sessionSchema),
			store.NewMongoStore[assistant.Message](db, messageSchema), nil

	default:
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("unsupported database driver %q", c.Cfg.Database.Driver)
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

	c.Scheduler.Register("reminder-dispatch", 30*time.Second, reminder.Dispatch(c.ReminderService, c.Log))
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
