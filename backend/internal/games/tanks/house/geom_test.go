package house

import (
	"math"
	"testing"

	"tolerance/internal/games/tanks"
)

// wallTestRect is a simple 5x5 wall used across the segmentClearsRect/steerAround tests below.
var wallTestRect = tanks.Rect{X: 10, Y: 10, W: 5, H: 5}

// wallTestPad matches what hunterStrategy actually uses: tank radius (1.0, tanks.DefaultRules'
// TankRadius) plus steerMargin.
const wallTestPad = 1.0 + steerMargin

// wallTouchPoint is where a tank sits once it has collided with and rested against wallTestRect's
// left face: exactly its own radius away from it, which is inside wallTestPad.
var wallTouchPoint = tanks.Point{X: wallTestRect.X - 1.0, Y: wallTestRect.Y + wallTestRect.H/2}

func TestSegmentClearsRectTouchingWallHeadingAway(t *testing.T) {
	// Heading straight away from the wall (decreasing x, same y) must be reported clear even though
	// the segment starts inside the padded margin -- this is the case the engine's own collision
	// push-out puts every tank resting against a wall in.
	if !segmentClearsRect(wallTouchPoint.X, wallTouchPoint.Y, wallTouchPoint.X-10, wallTouchPoint.Y, wallTestRect, wallTestPad) {
		t.Errorf("segmentClearsRect: heading away from a touched wall reported blocked")
	}
}

func TestSegmentClearsRectTouchingWallHeadingIn(t *testing.T) {
	// Heading straight into the wall (increasing x, same y, crossing the solid rect) must still be
	// blocked: the touching-wall relaxation only lets through headings that don't get closer.
	if segmentClearsRect(wallTouchPoint.X, wallTouchPoint.Y, wallTouchPoint.X+15, wallTouchPoint.Y, wallTestRect, wallTestPad) {
		t.Errorf("segmentClearsRect: heading into a touched wall reported clear")
	}
}

func TestSegmentClearsRectTouchingWallHeadingAlongFaceStillBlockedIfCloser(t *testing.T) {
	// A diagonal heading that dips into the solid rect (not just the padded margin) must be blocked
	// even though it starts touching the wall: the relaxation never lets a heading cross the wall
	// itself, only headings that stay outside it and get no closer.
	into := tanks.Point{X: wallTestRect.X + 2, Y: wallTestRect.Y + 2} // inside the solid rect
	if segmentClearsRect(wallTouchPoint.X, wallTouchPoint.Y, into.X, into.Y, wallTestRect, wallTestPad) {
		t.Errorf("segmentClearsRect: a heading that dips into the solid wall reported clear")
	}
}

func TestSegmentClearsRectFarFromWallUnaffected(t *testing.T) {
	// Far from the wall (start outside the padded margin), the ordinary clipping behaviour applies
	// unchanged: a segment that crosses the rect is blocked, one that doesn't is clear.
	if segmentClearsRect(0, 12.5, 20, 12.5, wallTestRect, wallTestPad) {
		t.Errorf("segmentClearsRect: a segment crossing the wall from far away reported clear")
	}
	if !segmentClearsRect(0, 0, 5, 0, wallTestRect, wallTestPad) {
		t.Errorf("segmentClearsRect: a segment nowhere near the wall reported blocked")
	}
}

func TestSteerAroundTouchingWallPicksAnAwayHeading(t *testing.T) {
	walls := []tanks.Rect{wallTestRect}
	me := wallTouchPoint
	target := tanks.Point{X: wallTestRect.X + wallTestRect.W + 5, Y: me.Y} // straight through the wall

	direct := angleTo(me.X, me.Y, target.X, target.Y)
	directClear := pathClear(walls, me, target, wallTestPad)
	if directClear {
		t.Fatalf("test setup: expected the direct heading through the wall to be blocked")
	}

	heading := steerAround(walls, wallTestPad, me, target, directClear)
	if math.Abs(angleDiff(heading, direct)) < 1e-6 {
		t.Errorf("steerAround: still returned the blocked direct heading %v while touching the wall", heading)
	}
}

func TestSteerAroundReturnsDirectWhenCallerSaysClear(t *testing.T) {
	// steerAround takes the caller's own directClear verdict instead of recomputing pathClear itself
	// (the caller, e.g. hunterStrategy, already needed that answer for gating fire) -- passing true
	// short-circuits straight to the direct heading.
	walls := []tanks.Rect{wallTestRect}
	me := tanks.Point{X: 0, Y: 0}
	target := tanks.Point{X: 0, Y: 20}

	direct := angleTo(me.X, me.Y, target.X, target.Y)
	heading := steerAround(walls, wallTestPad, me, target, true)
	if heading != direct {
		t.Errorf("steerAround: directClear=true returned %v, want the direct heading %v unchanged", heading, direct)
	}
}
