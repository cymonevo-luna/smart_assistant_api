// Package mongo owns the lifecycle of the MongoDB client.
package mongo

import (
	"context"
	"fmt"

	"github.com/cymonevo/go_template/internal/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect builds and verifies a MongoDB client and returns the handle to the
// configured database.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*mongo.Client, *mongo.Database, error) {
	opts := options.Client().
		ApplyURI(cfg.URI).
		SetMaxPoolSize(uint64(cfg.MaxOpenConns)).
		SetMinPoolSize(uint64(cfg.MinConns)).
		SetConnectTimeout(cfg.ConnTimeout)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("connect mongo: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}

	return client, client.Database(cfg.MongoDatabase), nil
}
