/*
 * Copyright (c) 2021 Xingwang Liao
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package internal

import (
	"sync"
	"time"
)

type FunctionCallback func()

type ClearCallback func(id int32)

type Item struct {
	ClearCB    ClearCallback
	FunctionCB FunctionCallback

	done      chan struct{}
	doneOnce  sync.Once
	clearOnce sync.Once
	ID        int32
	Delay     int32

	Interval bool
}

func (t *Item) Clear() {
	t.clearOnce.Do(func() {
		close(t.doneChannel())
		if t.ClearCB != nil {
			t.ClearCB(t.ID)
		}
	})
}

func (t *Item) Start() {
	done := t.doneChannel()
	go func() {
		defer t.Clear() // self clear

		delay := time.Duration(t.Delay) * time.Millisecond
		if !t.Interval {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				if t.FunctionCB != nil {
					t.FunctionCB()
				}
			case <-done:
			}
			return
		}

		ticker := time.NewTicker(delay)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if t.FunctionCB != nil {
					t.FunctionCB()
				}
			case <-done:
				return
			}
		}
	}()
}

func (t *Item) doneChannel() chan struct{} {
	t.doneOnce.Do(func() {
		t.done = make(chan struct{})
	})
	return t.done
}
