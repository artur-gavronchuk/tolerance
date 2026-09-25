package tanks

import (
	"reflect"
	"testing"
)

func TestReplayRoundTrip(t *testing.T) {
	dt := 5
	r := Replay{
		Version:  1,
		Engine:   EngineVersion,
		Seed:     42,
		Map:      "crossroads",
		TickRate: 10,
		Rules:    DefaultRules(),
		Walls:    Maps()[1].Walls,
		Players: []ReplayPlayer{
			{Slot: 0, Name: "alpha", House: false, Source: "agent"},
			{Slot: 1, Name: "beta", House: true},
		},
		Frames: []Frame{
			{T: 0, K: [][]float64{{1, 2, 0, 0, 100, 0, 1}}, S: [][]float64{}, B: [][]float64{{3, 4}}, Z: 37},
		},
		Events: []Event{
			{T: 1, E: "shot", A: 0},
			{T: 2, E: "hit", A: 0, B: intPtr(1), D: intPtr(25)},
		},
		Result: []PlayerResult{
			{Slot: 0, Place: 1, Kills: 1, Damage: 100, DeathTick: nil, Status: "ok"},
			{Slot: 1, Place: 2, Kills: 0, Damage: 0, DeathTick: &dt, Status: "ok"},
		},
	}

	gz, err := EncodeReplay(r)
	if err != nil {
		t.Fatalf("EncodeReplay: %v", err)
	}
	got, err := DecodeReplay(gz)
	if err != nil {
		t.Fatalf("DecodeReplay: %v", err)
	}
	if !reflect.DeepEqual(r, got) {
		t.Errorf("round trip mismatch:\n got: %+v\nwant: %+v", got, r)
	}
}

func TestFrameRounding(t *testing.T) {
	g := &Game{
		Tanks: []Tank{{ID: 0, X: 1.23456, Y: 0, Hull: 0.123456, Turret: 0, HP: 100, Alive: true}},
		Zone:  Zone{R: 10},
	}
	f := g.Frame()
	if f.K[0][0] != 1.23 {
		t.Errorf("X = %v, want 1.23", f.K[0][0])
	}
	if f.K[0][2] != 0.123 {
		t.Errorf("Hull = %v, want 0.123", f.K[0][2])
	}
}
