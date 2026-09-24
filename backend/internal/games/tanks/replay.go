package tanks

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"math"
)

// ReplayPlayer describes one slot's participant for a replay's header.
type ReplayPlayer struct {
	Slot    int    `json:"slot"`
	Name    string `json:"name"`
	BotID   string `json:"bot_id,omitempty"`
	Version int    `json:"version,omitempty"`
	House   bool   `json:"house"`
	Source  string `json:"source,omitempty"`
}

// Frame is one tick's recorded state, coordinates rounded to 0.01 and
// angles to 0.001 to keep replays small.
type Frame struct {
	T int         `json:"t"`
	K [][]float64 `json:"k"` // per slot: x, y, hull, turret, hp, reload, alive(0|1)
	S [][]float64 `json:"s"` // id, owner, x, y
	B [][]float64 `json:"b"` // active bonuses: x, y
	Z float64     `json:"z"` // zone radius
}

// PlayerResult is one slot's final outcome.
type PlayerResult struct {
	Slot      int    `json:"slot"`
	Place     int    `json:"place"`
	Kills     int    `json:"kills"`
	Damage    int    `json:"damage"`
	DeathTick *int   `json:"death_tick"`
	Status    string `json:"status"` // ok | crashed | timeout | invalid
}

// Replay is the full recorded match: everything needed to play it back
// without re-simulating it.
type Replay struct {
	Version  int            `json:"version"`
	Engine   string         `json:"engine"`
	Seed     int64          `json:"seed"`
	Map      string         `json:"map"`
	TickRate int            `json:"tick_rate"`
	Rules    Rules          `json:"rules"`
	Walls    []Rect         `json:"walls"`
	Players  []ReplayPlayer `json:"players"`
	Frames   []Frame        `json:"frames"`
	Events   []Event        `json:"events"`
	Result   []PlayerResult `json:"result"`
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// Frame captures the game's current state as a replay frame.
func (g *Game) Frame() Frame {
	f := Frame{T: g.Tick, Z: round2(g.Zone.R)}

	f.K = make([][]float64, len(g.Tanks))
	for i, t := range g.Tanks {
		alive := 0.0
		if t.Alive {
			alive = 1
		}
		f.K[i] = []float64{
			round2(t.X), round2(t.Y), round3(t.Hull), round3(t.Turret),
			float64(t.HP), float64(t.Reload), alive,
		}
	}

	f.S = make([][]float64, len(g.Shells))
	for i, s := range g.Shells {
		f.S[i] = []float64{float64(s.ID), float64(s.Owner), round2(s.X), round2(s.Y)}
	}

	f.B = [][]float64{}
	for _, b := range g.Bonuses {
		if b.Active {
			f.B = append(f.B, []float64{round2(b.X), round2(b.Y)})
		}
	}

	return f
}

// EncodeReplay serializes a replay as gzip-compressed JSON.
func EncodeReplay(r Replay) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeReplay parses a gzip-compressed JSON replay.
func DecodeReplay(gz []byte) (Replay, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return Replay{}, err
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		return Replay{}, err
	}
	var r Replay
	if err := json.Unmarshal(data, &r); err != nil {
		return Replay{}, err
	}
	return r, nil
}
