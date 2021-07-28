package entity

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity/action"
	"github.com/df-mc/dragonfly/server/entity/physics"
	"github.com/df-mc/dragonfly/server/internal/nbtconv"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"math"
	"time"
)

// Item represents an item entity which may be added to the world. Players and several humanoid entities such
// as zombies are able to pick up these entities so that the items are added to their inventory.
type Item struct {
	transform
	age, pickupDelay int
	i                item.Stack

	c *MovementComputer
}

// NewItem creates a new item entity using the item stack passed. The item entity will be positioned at the
// position passed.
// If the stack's count exceeds its max count, the count of the stack will be changed to the maximum.
func NewItem(i item.Stack, pos mgl64.Vec3) *Item {
	if i.Count() > i.MaxCount() {
		i = i.Grow(i.Count() - i.MaxCount())
	}
	i = nbtconv.ItemFromNBT(nbtconv.ItemToNBT(i, false), nil)

	it := &Item{i: i, pickupDelay: 40, c: &MovementComputer{
		Gravity:           0.04,
		DragBeforeGravity: true,
		Drag:              0.02,
	}}
	it.transform = newTransform(it, pos)
	return it
}

// Item returns the item stack that the item entity holds.
func (it *Item) Item() item.Stack {
	return it.i
}

// SetPickupDelay sets a delay passed until the item can be picked up. If d is negative or d.Seconds()*20
// higher than math.MaxInt16, the item will never be able to be picked up.
func (it *Item) SetPickupDelay(d time.Duration) {
	ticks := int(d.Seconds() * 20)
	if ticks < 0 || ticks >= math.MaxInt16 {
		ticks = math.MaxInt16
	}
	it.pickupDelay = ticks
}

// Name ...
func (it *Item) Name() string {
	return fmt.Sprintf("%T", it.i.Item())
}

// Tick ticks the entity, performing movement.
func (it *Item) Tick(current int64) {
	it.mu.Lock()
	defer it.mu.Unlock()

	if it.pos[1] < cube.MinY && current%10 == 0 {
		_ = it.Close()
		return
	}
	if it.age++; it.age > 6000 {
		_ = it.Close()
		return
	}
	it.pos, it.vel = it.c.TickMovement(it, it.pos, it.vel)

	if it.pickupDelay == 0 {
		it.checkNearby()
	} else if it.pickupDelay != math.MaxInt16 {
		it.pickupDelay--
	}
}

// checkNearby checks the entities of the chunks around for item collectors and other item stacks. If a
// collector is found in range, the item will be picked up. If another item stack with the same item type is
// found in range, the item stacks will merge.
func (it *Item) checkNearby() {
	grown := it.AABB().GrowVec3(mgl64.Vec3{1, 0.5, 1}).Translate(it.pos)
	for _, e := range it.World().EntitiesWithin(it.AABB().Translate(it.pos).Grow(2)) {
		if e == it {
			// Skip the item entity itself.
			continue
		}
		if e.AABB().Translate(e.Position()).IntersectsWith(grown) {
			if collector, ok := e.(Collector); ok {
				// A collector was within range to pick up the entity.
				it.collect(collector)
				return
			} else if other, ok := e.(*Item); ok {
				// Another item entity was in range to merge with.
				if it.merge(other) {
					return
				}
			}
		}
	}
}

// merge merges the item entity with another item entity.
func (it *Item) merge(other *Item) bool {
	if other.i.Count() == other.i.MaxCount() || it.i.Count() == it.i.MaxCount() {
		// Either stack is already filled up to the maximum, meaning we can't change anything any way.
		return false
	}
	if !it.i.Comparable(other.i) {
		return false
	}

	a, b := other.i.AddStack(it.i)

	newA := NewItem(a, other.Position())
	newA.SetVelocity(other.Velocity())
	it.World().AddEntity(newA)

	if !b.Empty() {
		newB := NewItem(b, it.pos)
		newB.SetVelocity(it.vel)
		it.World().AddEntity(newB)
	}
	_ = it.Close()
	_ = other.Close()
	return true
}

// collect makes a collector collect the item (or at least part of it).
func (it *Item) collect(collector Collector) {
	n := collector.Collect(it.i)
	if n == 0 {
		return
	}
	for _, viewer := range it.World().Viewers(it.pos) {
		viewer.ViewEntityAction(it, action.PickedUp{Collector: collector})
	}

	if n == it.i.Count() {
		// The collector picked up the entire stack.
		_ = it.Close()
		return
	}
	// Create a new item entity and shrink it by the amount of items that the collector collected.
	it.World().AddEntity(NewItem(it.i.Grow(-n), it.pos))

	_ = it.Close()
}

// AABB ...
func (it *Item) AABB() physics.AABB {
	return physics.NewAABB(mgl64.Vec3{-0.125, 0, -0.125}, mgl64.Vec3{0.125, 0.25, 0.125})
}

// EncodeEntity ...
func (it *Item) EncodeEntity() string {
	return "minecraft:item"
}

// Collector represents an entity in the world that is able to collect an item, typically an entity such as
// a player or a zombie.
type Collector interface {
	world.Entity
	// Collect collects the stack passed. It is called if the Collector is standing near an item entity that
	// may be picked up.
	// The count of items collected from the stack n is returned.
	Collect(stack item.Stack) (n int)
}
