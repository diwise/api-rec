package application

import (
	"testing"
	"time"

	"github.com/farshidtz/senml/v2"
	"github.com/google/uuid"
	"github.com/matryer/is"
)

func TestMessageAcceptedMapping(t *testing.T) {
	is := is.New(t)
	sensorID := uuid.NewString()
	now := time.Now()
	v := 12.345678
	ma := MessageAccepted{
		SensorID:  sensorID,
		Timestamp: now,
		Pack: senml.Pack{
			senml.Record{
				StringValue: sensorID,
				BaseTime:    float64(now.Unix()),
				BaseName:    Temperature,
			},
			senml.Record{
				Value: &v,
			},
		},
	}

	so, ok := ma.MapToObservation()

	is.True(ok)
	is.Equal(12.3, *so.Observations[0].Value)
}

func TestFunctionUpdated(t *testing.T) {
	is := is.New(t)

	fu := FunctionUpdated{
		Id:      uuid.NewString(),
		Type:    "waterquality",
		SubType: "beach",
		WaterQuality: &struct {
			Temperature float64   `json:"temperature"`
			Timestamp   time.Time `json:"timestamp"`
		}{
			Temperature: 12.345678,
			Timestamp:   time.Now(),
		},
	}

	so, ok := fu.MapToObservation()

	is.True(ok)
	is.Equal(12.3, *so.Observations[0].Value)
}
