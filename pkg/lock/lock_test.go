// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// GOMAXPROCS=10 go test

package lock

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

func reader(rwm ErrorRWLocker, iterations int, activity *int32, cdone chan bool) {
	for i := 0; i < iterations; i++ {
		err := rwm.RLock()
		if err != nil {
			panic(err)
		}
		n := atomic.AddInt32(activity, 1)
		if n < 1 || n >= 10000 {
			panic(fmt.Sprintf("wlock(%d)\n", n))
		}
		for i := 0; i < 100; i++ {
		}
		atomic.AddInt32(activity, -1)
		rwm.RUnlock()
	}
	cdone <- true
}

func writer(rwm ErrorRWLocker, iterations int, activity *int32, cdone chan bool) {
	for i := 0; i < iterations; i++ {
		err := rwm.Lock()
		if err != nil {
			panic(err)
		}
		n := atomic.AddInt32(activity, 10000)
		if n != 10000 {
			panic(fmt.Sprintf("wlock(%d)\n", n))
		}
		for i := 0; i < 100; i++ {
		}
		atomic.AddInt32(activity, -10000)
		rwm.Unlock()
	}
	cdone <- true
}

func HammerRWMutex(locker ErrorRWLocker, gomaxprocs, numReaders, iterations int) {
	runtime.GOMAXPROCS(gomaxprocs)
	// Number of active readers + 10000 * number of active writers.
	var activity int32
	cdone := make(chan bool)
	go writer(locker, iterations, &activity, cdone)
	var i int
	for i = 0; i < numReaders/2; i++ {
		go reader(locker, iterations, &activity, cdone)
	}
	go writer(locker, iterations, &activity, cdone)
	for ; i < numReaders; i++ {
		go reader(locker, iterations, &activity, cdone)
	}
	// Wait for the 2 writers and all readers to finish.
	for i := 0; i < 2+numReaders; i++ {
		<-cdone
	}
}

func HammerMutex(m ErrorLocker, loops int, cdone chan bool) {
	for i := 0; i < loops; i++ {
		err := m.Lock()
		if err != nil {
			panic(err)
		}
		m.Unlock()
	}
	cdone <- true
}

var n = 1000

func TestMemLock(t *testing.T) {
	backconf := config.GetConfig().Lock
	var err error
	config.GetConfig().Lock, err = config.NewRedisConfig("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { config.GetConfig().Lock = backconf }()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(-1))
	db := prefixer.NewPrefixer("cozy.local", "cozy.local")
	l := ReadWrite(db, "test-mem")
	HammerRWMutex(l, 1, 1, n)
	HammerRWMutex(l, 1, 3, n)
	HammerRWMutex(l, 1, 10, n)
	HammerRWMutex(l, 4, 1, n)
	HammerRWMutex(l, 4, 3, n)
	HammerRWMutex(l, 4, 10, n)
	HammerRWMutex(l, 10, 1, n)
	HammerRWMutex(l, 10, 3, n)
	HammerRWMutex(l, 10, 10, n)
	HammerRWMutex(l, 10, 5, n)
}

func TestRedisLock(t *testing.T) {
	backconf := config.GetConfig().Lock
	var err error
	config.GetConfig().Lock, err = config.NewRedisConfig("redis://localhost:6379/0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { config.GetConfig().Lock = backconf }()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(-1))
	db := prefixer.NewPrefixer("cozy.local", "cozy.local")
	l := ReadWrite(db, "test-redis")
	// @TODO use HammerRWMutex when redisLock is RW lock
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go HammerMutex(l, n, done)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	if testing.Short() {
		n = 5
	}
	os.Exit(m.Run())
}
