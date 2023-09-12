package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/diwise/api-rec/internal/pkg/application"
	"github.com/diwise/api-rec/internal/pkg/infrastructure/database"
	"github.com/farshidtz/senml/v2"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

func TestMapToObservation(t *testing.T) {
	v := 1.23456789
	m := application.MessageAccepted{
		SensorID:  uuid.New().String(),
		Timestamp: time.Now(),
		Pack: senml.Pack{
			senml.Record{
				BaseName:    "test",
				StringValue: uuid.New().String(),
			},
			senml.Record{
				Value: &v,
			},
		},
	}

	o, _ := m.MapToObservation()

	if *o.Observations[0].Value != 1.23 {
		t.FailNow()
	}
}

func connect() (context.Context, context.CancelFunc, database.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	db, err := database.Connect(ctx, database.NewConfig("localhost", "postgres", "password", "5432", "postgres", "disable"))
	if err != nil {
		return ctx, cancel, nil, err
	}
	err = db.Init(ctx)
	if err != nil {
		return ctx, cancel, nil, err
	}

	return ctx, cancel, db, nil
}

func cloudEventSenderFunc(ctx context.Context, evt eventInfo) error {
	c, err := cloudevents.NewClientHTTP()
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%s:%d", evt.id, evt.timestamp.Unix())

	event := cloudevents.NewEvent()
	event.SetID(id)
	event.SetTime(evt.timestamp)
	event.SetSource(evt.source)
	event.SetType(evt.eventType)
	err = event.SetData(cloudevents.ApplicationJSON, evt.data)
	if err != nil {
		return err
	}

	ctx = cloudevents.ContextWithTarget(ctx, evt.endpoint)
	result := c.Send(ctx, event)
	if cloudevents.IsUndelivered(result) || errors.Is(result, unix.ECONNREFUSED) {
		return err
	}

	return nil
}

type eventInfo struct {
	id        string
	timestamp time.Time
	source    string
	eventType string
	endpoint  string
	data      []byte
}

func TestCloudevents(t *testing.T) {
	ctx, cancel, db, err := connect()
	defer cancel()

	if err != nil {
		t.Log("could not connect to database or create tables, will skip test")
		t.SkipNow()
	}

	app := application.New(db)

	srv := httptest.NewServer(handleCloudevents(ctx, app))

	sensorID := uuid.New().String()
	now := time.Now()

	v := 1.232
	m := application.MessageAccepted{
		SensorID:  sensorID,
		Timestamp: time.Now(),
		Pack: senml.Pack{
			senml.Record{
				BaseName:    "test",
				StringValue: sensorID,
				BaseTime:    float64(now.Unix()),
			},
			senml.Record{
				Value: &v,
			},
		},
	}

	b, _ := json.Marshal(m)

	err = cloudEventSenderFunc(ctx, eventInfo{
		endpoint:  srv.URL,
		eventType: application.MessageAcceptedName,
		data:      b,
		id:        uuid.New().String(),
		timestamp: time.Now(),
		source:    "test",
	})
	if err != nil {
		t.FailNow()
	}

	v2 := 1.233
	m.Pack[1].Value = &v2
	b, _ = json.Marshal(m)

	err = cloudEventSenderFunc(ctx, eventInfo{
		endpoint:  srv.URL,
		eventType: application.MessageAcceptedName,
		data:      b,
		id:        uuid.New().String(),
		timestamp: time.Now(),
		source:    "test",
	})
	if err != nil {
		t.Log("could not send cloudevent")
		t.FailNow()
	}

	count, observations, err := db.GetObservations(ctx, sensorID, now.Add(-1*time.Second), now.Add(1*time.Minute), 0, 10)
	if err != nil {
		t.Log("could not fetch observations")
		t.FailNow()
	}

	if count != 1 {
		t.Logf("number of observations is not expected, 1 != %d", count)
		t.FailNow()
	}

	if *observations[0].Value != 1.23 {
		t.Logf("value should be 1.23, is = %f", *observations[0].Value)
		t.FailNow()
	}
}
