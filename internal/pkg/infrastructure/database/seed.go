package database

import (
	"context"
	"encoding/csv"
	"io"
)

type rec struct {
	Space    string
	Building string
	Sensor   string
}

func (db *databaseImpl) Seed(ctx context.Context, reader io.Reader) error {

	recs := getRecFromReader(reader)

	// if input data is sorted then this will make it a tiny bit faster.
	// AddEntity will DO NOTHING ON CONFLICT
	lastSpaceId := ""
	lastBuildingId := ""

	for _, r := range recs {
		if lastSpaceId != r.Space {
			space := Entity{
				Context: SpaceContext,
				Id:      r.Space,
				Type:    SpaceType,
			}
			err := db.AddEntity(ctx, space)
			if err != nil {
				return err
			}
			lastSpaceId = r.Space
		}

		if lastBuildingId != r.Building {
			building := Entity{
				Context: BuildingContext,
				Id:      r.Building,
				Type:    BuildingType,
				IsPartOf: &Property{
					Id:   r.Space,
					Type: SpaceType,
				},
			}
			err := db.AddEntity(ctx, building)
			if err != nil {
				return err
			}
			lastBuildingId = r.Building
		}

		sensor := Entity{
			Context: SensorContext,
			Id:      r.Sensor,
			Type:    SensorType,
			IsPartOf: &Property{
				Id:   r.Building,
				Type: BuildingType,
			},
		}
		err := db.AddEntity(ctx, sensor)
		if err != nil {
			return err
		}
	}

	return nil
}

func getRecFromReader(reader io.Reader) []rec {
	r := csv.NewReader(reader)
	r.Comma = ';'

	rows, err := r.ReadAll()
	if err != nil {
		return []rec{}
	}

	return getRecFromRows(rows)
}

func getRecFromRows(rows [][]string) []rec {
	recs := make([]rec, 0)

	if len(rows) == 0 {
		return recs
	}

	for _, row := range rows[1:] {
		recs = append(recs, rec{
			Space:    row[0],
			Building: row[1],
			Sensor:   row[2],
		})
	}
	return recs
}
