package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAllow_BlocksAfterMax(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := New(rc, 2, time.Minute)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		ok, err := lim.Allow(ctx, "login:a@b.co")
		if err != nil || !ok {
			t.Fatalf("attempt %d should be allowed: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := lim.Allow(ctx, "login:a@b.co")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("third attempt within window must be blocked")
	}
}
