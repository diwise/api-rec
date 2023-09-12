package application

import (
	"context"
	"time"

	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
)

type Application interface {
	AddEntity(ctx context.Context, e database.Entity) error
	GetEntity(ctx context.Context, entityID, entityType string) (database.Entity, error)
	GetEntities(ctx context.Context, entityType string, page, size int) (int64, []database.Entity, error)
	GetChildEntities(ctx context.Context, root database.Entity, entityType string) ([]database.Entity, error)
	AddObservation(ctx context.Context, so database.SensorObservation) error
	GetObservations(ctx context.Context, sensorId string, starting, ending time.Time, page, size int) (int64, []database.Observation, error)
}

type app struct {
	db database.Database
}

func (a *app) AddEntity(ctx context.Context, e database.Entity) error {
	return a.db.AddEntity(ctx, e)
}

func (a *app) GetEntity(ctx context.Context, entityID string, entityType string) (database.Entity, error) {
	return a.db.GetEntity(ctx, entityID, entityType)
}

func (a *app) GetEntities(ctx context.Context, entityType string, page int, size int) (int64, []database.Entity, error) {
	return a.db.GetEntities(ctx, entityType, page, size)
}

func (a *app) GetChildEntities(ctx context.Context, root database.Entity, entityType string) ([]database.Entity, error) {
	return a.db.GetChildEntities(ctx, root, entityType)
}

func (a *app) AddObservation(ctx context.Context, so database.SensorObservation) error {
	return a.db.AddObservation(ctx, so)
}

func (a *app) GetObservations(ctx context.Context, sensorId string, starting time.Time, ending time.Time, page int, size int) (int64, []database.Observation, error) {
	return a.db.GetObservations(ctx, sensorId, starting, ending, page, size)
}

func New(db database.Database) Application {
	return &app{
		db: db,
	}
}
