package inventory

import (
	"errors"
	"fmt"
	"github.com/dragonfly-tech/dragonfly/dragonfly/item"
	"strings"
	"sync"
)

// Inventory represents an inventory containing items. These inventories may be carried by entities or may be
// held by blocks such as chests.
// The size of an inventory may be specified upon construction, but cannot be changed after. The zero value of
// an Inventory is invalid. Use New() to obtain a new inventory.
// Inventory is safe for concurrent usage: Its values are protected by a mutex.
type Inventory struct {
	mu    sync.RWMutex
	slots []item.Stack
	f     func(slot int, item item.Stack)
}

// ErrSlotOutOfRange is returned by any methods on Inventory when a slot is passed which is not within the
// range of valid values for the inventory.
var ErrSlotOutOfRange = errors.New("slot is out of range: must be in range 0 <= slot < Inventory.Size()")

// New creates a new inventory with the size passed. The inventory size cannot be changed after it has been
// constructed.
// A function may be passed which is called every time a slot is changed. The function may also be nil, if
// nothing needs to be done.
func New(size int, f func(slot int, item item.Stack)) *Inventory {
	if size <= 0 {
		panic("inventory size must be at least 1")
	}
	if f == nil {
		f = func(slot int, item item.Stack) {}
	}
	return &Inventory{slots: make([]item.Stack, size), f: f}
}

// Item attempts to obtain an item from a specific slot in the Inventory. If an item was present in that slot,
// the item is returned and the error is nil. If no item was present in the slot, a Stack with air as its item
// and a count of 0 is returned. Stack.Empty() may be called to check if this is the case.
// Item only returns an error if the slot passed is out of range. (0 <= slot < Inventory.Size())
func (inv *Inventory) Item(slot int) (item.Stack, error) {
	inv.check()
	if !inv.validSlot(slot) {
		return item.Stack{}, ErrSlotOutOfRange
	}

	inv.mu.RLock()
	i := inv.slots[slot]
	inv.mu.RUnlock()
	return i, nil
}

// SetItem sets a stack of items to a specific slot in the Inventory. If an item is already present in the
// slot, that item will be overwritten.
// SetItem will return an error if the slot passed is out of range. (0 <= slot < Inventory.Size())
func (inv *Inventory) SetItem(slot int, item item.Stack) error {
	inv.check()
	if !inv.validSlot(slot) {
		return ErrSlotOutOfRange
	}

	inv.mu.Lock()
	inv.setItem(slot, item)
	inv.mu.Unlock()
	return nil
}

// AddItem attempts to add an item to the inventory. It does so in a couple of steps: It first iterates over
// the inventory to make sure no existing stacks of the same type exist. If these stacks do exist, the item
// added is first added on top of those stacks to make sure they are fully filled.
// If no existing stacks with leftover space are left, empty slots will be filled up with the remainder of the
// item added.
// If the item could not be fully added to the inventory, an error is returned.
func (inv *Inventory) AddItem(it item.Stack) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	for slot, invIt := range inv.slots {
		if invIt.Empty() {
			// This slot was empty, and we should first try to add the item stack to existing stacks.
			continue
		}

		a, b := invIt.AddStack(it)
		inv.setItem(slot, a)
		it = b
		if it.Empty() {
			// We were able to add the entire stack to existing stacks in the inventory.
			return nil
		}
	}
	for slot, invIt := range inv.slots {
		if !invIt.Empty() {
			// We can only use empty slots now: All existing stacks have already been filled up.
			continue
		}
		invIt = item.NewStack(it.Item(), 0)
		a, b := invIt.AddStack(it)

		inv.setItem(slot, a)
		it = b
		if it.Empty() {
			// We were able to add the entire stack to empty slots.
			return nil
		}
	}
	// We were unable to clear out the entire stack to be added to the inventory: There wasn't enough space.
	return fmt.Errorf("could not add full item stack to inventory")
}

// RemoveItem attempts to remove an item from the inventory. It will visit all slots in the inventory and
// empties them until it.Count() items have been removed from the inventory.
// If less than it.Count() items could be found in the inventory, an error is returned.
func (inv *Inventory) RemoveItem(it item.Stack) error {
	toRemove := it.Count()

	inv.mu.Lock()
	defer inv.mu.Unlock()

	for slot, slotIt := range inv.slots {
		if slotIt.Empty() {
			continue
		}
		if !slotIt.Comparable(it) {
			// The items were not comparable: Continue with the next slot.
			continue
		}
		inv.setItem(slot, slotIt.Grow(-toRemove))
		toRemove -= slotIt.Count()

		if toRemove <= 0 {
			// No more items left to remove: We can exit the loop.
			return nil
		}
	}
	return fmt.Errorf("could not remove all items from the inventory")
}

// Empty checks if the inventory is fully empty: It iterates over the inventory and makes sure every stack in
// it is empty.
func (inv *Inventory) Empty() bool {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	for _, it := range inv.slots {
		if !it.Empty() {
			return false
		}
	}
	return true
}

// setItem sets an item to a specific slot and overwrites the existing item. It calls the function which is
// called for every item change and does so without locking the inventory.
func (inv *Inventory) setItem(slot int, item item.Stack) {
	inv.slots[slot] = item
	inv.f(slot, item)
}

// Size returns the size of the inventory. It is always the same value as that passed in the call to New() and
// is always at least 1.
func (inv *Inventory) Size() int {
	inv.mu.RLock()
	l := len(inv.slots)
	inv.mu.RUnlock()
	return l
}

// Close closes the inventory, freeing the function called for every slot change.
// The returned error is always nil.
func (inv *Inventory) Close() error {
	inv.mu.Lock()
	inv.f = func(int, item.Stack) {}
	inv.mu.Unlock()
	return nil
}

// String implements the fmt.Stringer interface.
func (inv *Inventory) String() string {
	s := make([]string, 0, inv.Size())
	for _, it := range inv.slots {
		s = append(s, it.String())
	}
	return strings.Join(s, ", ")
}

// validSlot checks if the slot passed is valid for the inventory. It returns false if the slot is either
// smaller than 0 or bigger/equal to the size of the inventory's size.
func (inv *Inventory) validSlot(slot int) bool {
	return slot >= 0 && slot < inv.Size()
}

// check panics if the Inventory is valid, and panics if it is not. This typically happens if the inventory
// was not created using New().
func (inv *Inventory) check() {
	if inv.Size() == 0 {
		panic("uninitialised inventory: inventory must be constructed using inventory.New()")
	}
}