package graph

import (
	"context"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/xerrors"
)

// Neo4jAdapter 封装了 Neo4j 驱动程序，提供常用的图查询能力.
type Neo4jAdapter struct {
	driver neo4j.DriverWithContext
	logger *logging.Logger
}

// NewNeo4jAdapter 初始化 Neo4j 适配器.
func NewNeo4jAdapter(uri, username, password string, logger *logging.Logger) (*Neo4jAdapter, error) {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, xerrors.WrapInternal(err, "failed to create neo4j driver")
	}

	return &Neo4jAdapter{
		driver: driver,
		logger: logger,
	}, nil
}

// ExecuteWrite 执行写操作.
func (a *Neo4jAdapter) ExecuteWrite(ctx context.Context, cypher string, params map[string]any) (any, error) {
	session := a.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		return res.Collect(ctx)
	})
	if err != nil {
		a.logger.Error("failed to execute neo4j write", slog.String("cypher", cypher), slog.Any("error", err))
		return nil, xerrors.WrapInternal(err, "neo4j write transaction failed")
	}

	return result, nil
}

// Close 关闭驱动程序.
func (a *Neo4jAdapter) Close(ctx context.Context) error {
	return a.driver.Close(ctx)
}
