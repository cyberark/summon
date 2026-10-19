//go:build !solaris

package summon

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type countingTree struct {
	graceful, kill, probes, sig, closed int
}

func (c *countingTree) gracefulStop() error { c.graceful++; return nil }
func (c *countingTree) forceKill() error    { c.kill++; return nil }
func (c *countingTree) alive() bool         { c.probes++; return true }
func (c *countingTree) signal(os.Signal)    { c.sig++ }
func (c *countingTree) close()              { c.closed++ }

func TestTreeGuardIgnoresCallsAfterClose(t *testing.T) {
	inner := &countingTree{}
	tree := &treeGuard{}
	tree.attach(inner)

	assert.NoError(t, tree.gracefulStop())
	assert.NoError(t, tree.forceKill())
	assert.True(t, tree.alive())
	tree.signal(os.Interrupt)
	assert.Equal(t, countingTree{graceful: 1, kill: 1, probes: 1, sig: 1}, *inner)

	tree.close()
	tree.close() // idempotent

	assert.NoError(t, tree.gracefulStop())
	assert.NoError(t, tree.forceKill())
	assert.False(t, tree.alive())
	tree.signal(os.Interrupt)
	assert.Equal(t, countingTree{graceful: 1, kill: 1, probes: 1, sig: 1, closed: 1}, *inner)
}

func TestTreeGuardReattachAfterClose(t *testing.T) {
	first, second := &countingTree{}, &countingTree{}
	tree := &treeGuard{}

	tree.signal(os.Interrupt) // nothing attached: no-op
	tree.attach(first)
	tree.close()
	tree.attach(second)
	tree.signal(os.Interrupt)

	assert.Equal(t, countingTree{closed: 1}, *first)
	assert.Equal(t, countingTree{sig: 1}, *second)
}
