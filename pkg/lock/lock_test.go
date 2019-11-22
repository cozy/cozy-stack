// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// GOMAXPROCS=10 go test

package lock

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/assert"
)

var nb = 100

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

func HammerMutex(m ErrorLocker, cdone chan bool) {
	for i := 0; i < nb; i++ {
		err := m.Lock()
		if err != nil {
			panic(err)
		}
		m.Unlock()
	}
	cdone <- true
}

func hammerRW(t *testing.T, l ErrorRWLocker) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(-1))
	HammerRWMutex(l, 1, 1, nb)
	HammerRWMutex(l, 1, 3, nb)
	HammerRWMutex(l, 1, 10, nb)
	HammerRWMutex(l, 4, 1, nb)
	HammerRWMutex(l, 4, 3, nb)
	HammerRWMutex(l, 4, 10, nb)
	HammerRWMutex(l, 10, 1, nb)
	HammerRWMutex(l, 10, 3, nb)
	HammerRWMutex(l, 10, 10, nb)
	HammerRWMutex(l, 10, 5, nb)
}

func TestMemLock(t *testing.T) {
	var err error
	config.GetConfig().Lock, err = config.NewRedisConfig("")
	if err != nil {
		t.Fatal(err)
	}
	db := prefixer.NewPrefixer("cozy.local", "cozy.local")
	l := ReadWrite(db, "test-mem")
	hammerRW(t, l)
}

func TestRedisLock(t *testing.T) {
	var err error
	config.GetConfig().Lock, err = config.NewRedisConfig("redis://localhost:6379/0")
	if err != nil {
		t.Fatal(err)
	}
	db := prefixer.NewPrefixer("cozy.local", "cozy.local")
	l := ReadWrite(db, "test-redis")
	hammerRW(t, l)
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go HammerMutex(l, done)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	other := ReadWrite(db, "test-redis").(*redisLock)
	assert.NoError(t, l.Lock())
	assert.Error(t, other.LockWithTimeout(1*time.Second))
	l.Unlock()
}

func TestLongLock(t *testing.T) {
	var err error
	config.GetConfig().Lock, err = config.NewRedisConfig("redis://localhost:6379/0")
	if err != nil {
		t.Fatal(err)
	}
	db := prefixer.NewPrefixer("cozy.local", "cozy.local")
	long := LongOperation(db, "test-long")
	l := ReadWrite(db, "test-long").(*redisLock)
	assert.NoError(t, long.Lock())
	err = l.Lock()
	assert.Error(t, err)
	assert.Equal(t, ErrTooManyRetries, err)
	l.Unlock()
}

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		nb = 3
	}

	config.UseTestFile()
	backconf := config.GetConfig().Lock
	defer func() { config.GetConfig().Lock = backconf }()

	os.Exit(m.Run())
}
