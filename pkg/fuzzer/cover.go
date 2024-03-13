// Copyright 2024 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package fuzzer

import (
	"sync"

	"github.com/google/syzkaller/pkg/signal"
)

// Cover keeps track of the signal known to the fuzzer.
type Cover struct {
	mu        sync.RWMutex
	maxSignal signal.Signal // max signal ever observed (including flakes)
	newSignal signal.Signal // newly identified max signal
}

// Signal that should no longer be chased after.
func (cover *Cover) AddMaxSignal(sign signal.Signal) {
	cover.mu.Lock()
	defer cover.mu.Unlock()
	cover.maxSignal.Merge(sign)
}

func (cover *Cover) addRawMaxSignal(signal []uint32, prio uint8) signal.Signal {
	cover.mu.Lock()
	defer cover.mu.Unlock()
	diff := cover.maxSignal.DiffRaw(signal, prio)
	if diff.Empty() {
		return diff
	}
	cover.maxSignal.Merge(diff)
	cover.newSignal.Merge(diff)
	return diff
}

func (cover *Cover) GrabNewSignal() signal.Signal {
	cover.mu.Lock()
	defer cover.mu.Unlock()
	sign := cover.newSignal
	cover.newSignal = nil
	return sign
}

type CoverStat struct {
	MaxSignal int
}

func (cover *Cover) Stat() CoverStat {
	cover.mu.RLock()
	defer cover.mu.RUnlock()
	return CoverStat{
		MaxSignal: len(cover.maxSignal),
	}
}
